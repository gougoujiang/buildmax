package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	defaultKindCluster = "buildmaxdev"
	kindPortalURL      = "http://localhost:8080"
)

func cmdKind(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ./make kind <up|smoke|logs|down>")
	}
	switch args[0] {
	case "up":
		return kindUp()
	case "smoke":
		return kindSmoke()
	case "logs":
		return kindLogs()
	case "down":
		return kindDown()
	default:
		return fmt.Errorf("unknown kind command %q (want up, smoke, logs, or down)", args[0])
	}
}

func kindClusterName() string {
	return envOr("BUILDMAX_KIND_CLUSTER", defaultKindCluster)
}

func kindContext() string {
	return "kind-" + kindClusterName()
}

func kindKubectl(args ...string) error {
	return runCmd("kubectl", append([]string{"--context", kindContext()}, args...)...)
}

func captureKindKubectl(args ...string) (string, error) {
	return capture("kubectl", append([]string{"--context", kindContext()}, args...)...)
}

func kindUp() error {
	if err := requireCommands("docker", "kind", "kubectl"); err != nil {
		return err
	}
	if !succeeds("docker", "info") {
		return errors.New("Docker is installed but the engine is not ready")
	}

	cluster := kindClusterName()
	previousContext, _ := capture("kubectl", "config", "current-context")
	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		fmt.Printf("Creating kind cluster %q...\n", cluster)
		if err := runCmd("kind", "create", "cluster", "--name", cluster, "--config", "setup/kind-config.yaml"); err != nil {
			return err
		}
		// kind makes the new cluster globally current. Every command below uses
		// an explicit context, so restore the contributor's previous selection.
		if previousContext != "" && previousContext != kindContext() {
			if err := runCmd("kubectl", "config", "use-context", previousContext); err != nil {
				return fmt.Errorf("restore kubectl context %q: %w", previousContext, err)
			}
		}
	} else {
		fmt.Printf("Using existing kind cluster %q.\n", cluster)
		if err := validateKindPortMapping(cluster); err != nil {
			return err
		}
	}
	if err := kindKubectl("wait", "--for=condition=Ready", "nodes", "--all", "--timeout=120s"); err != nil {
		return err
	}

	fmt.Println("Installing ingress and backing services...")
	if err := kindKubectl("apply", "-f", "setup/kind-ingress-nginx.yaml"); err != nil {
		return err
	}
	if err := kindKubectl("wait", "--for=condition=Available", "deployment/ingress-nginx-controller", "-n", "ingress-nginx", "--timeout=180s"); err != nil {
		return err
	}
	if err := ensureKindNamespace("storage"); err != nil {
		return err
	}
	for _, manifest := range []string{"setup/mysql.yaml", "setup/minio.yaml"} {
		if err := kindKubectl("apply", "-f", manifest); err != nil {
			return err
		}
	}
	if err := kindKubectl("wait", "--for=condition=Available", "deployment/mysql", "-n", "db", "--timeout=180s"); err != nil {
		return err
	}
	if err := kindKubectl("wait", "--for=condition=Available", "deployment/minio", "-n", "storage", "--timeout=180s"); err != nil {
		return err
	}
	if err := initializeKindBucket(); err != nil {
		return err
	}

	if err := buildAndLoadKindImages(cluster, true); err != nil {
		return err
	}
	if err := ensureKindNamespace("buildmax"); err != nil {
		return err
	}
	if err := applyKindSecret(); err != nil {
		return err
	}
	if err := kindKubectl("apply", "-f", "deployment/buildmax-deploy.yaml"); err != nil {
		return err
	}
	if err := kindKubectl("apply", "-f", "deployment/smoke/mock-llm.kind.yaml"); err != nil {
		return err
	}
	if err := applyKindSmokeConfig(); err != nil {
		return err
	}
	for _, deployment := range []string{"buildmax-smoke-llm", "buildmax-server", "buildmax-portal"} {
		if err := kindKubectl("rollout", "restart", "deployment/"+deployment, "-n", "buildmax"); err != nil {
			return err
		}
	}
	for _, deployment := range []string{"buildmax-smoke-llm", "buildmax-server", "buildmax-portal"} {
		if err := kindKubectl("rollout", "status", "deployment/"+deployment, "-n", "buildmax", "--timeout=180s"); err != nil {
			return err
		}
	}

	if err := kindSmoke(); err != nil {
		fmt.Printf("Kind smoke failed. Run %s kind logs for diagnostics.\n", mk())
		return err
	}
	fmt.Printf("Kind stack is ready at %s (cluster %s).\n", kindPortalURL, cluster)
	return nil
}

func kindSmoke() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	target := kindSmokeTarget()
	if err := runDeploymentSmoke(context.Background(), target); err != nil {
		return err
	}
	printSmokeLogin(target)
	return nil
}

func kindSmokeTarget() smokeTarget {
	return smokeTarget{
		apiBase:              kindPortalURL,
		portalURL:            kindPortalURL,
		portalRuntimeAPIBase: "/",
		admin: func(args ...string) (string, error) {
			cmdArgs := append([]string{"--context", kindContext(), "exec", "-n", "buildmax", "deployment/buildmax-server", "--", "buildmax-server"}, args...)
			return captureCombined("kubectl", cmdArgs...)
		},
	}
}

func kindLogs() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	commands := [][]string{
		{"get", "pods,jobs,services,ingresses", "-n", "buildmax", "-o", "wide"},
		{"get", "events", "-n", "buildmax", "--sort-by=.lastTimestamp"},
		{"logs", "-n", "buildmax", "deployment/buildmax-server", "--all-containers", "--tail=200"},
		{"logs", "-n", "buildmax", "deployment/buildmax-portal", "--all-containers", "--tail=100"},
		{"logs", "-n", "buildmax", "-l", "job-name", "--all-containers", "--tail=200"},
	}
	for _, args := range commands {
		if err := kindKubectl(args...); err != nil {
			fmt.Printf("Warning: %v\n", err)
		}
	}
	return nil
}

func kindDown() error {
	if err := requireCommands("kind"); err != nil {
		return err
	}
	cluster := kindClusterName()
	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		fmt.Printf("Kind cluster %q does not exist.\n", cluster)
		return nil
	}
	return runCmd("kind", "delete", "cluster", "--name", cluster)
}

func kindClusterExists(name string) (bool, error) {
	output, err := capture("kind", "get", "clusters")
	if err != nil {
		return false, fmt.Errorf("list kind clusters: %w", err)
	}
	for _, cluster := range strings.Fields(output) {
		if cluster == name {
			return true, nil
		}
	}
	return false, nil
}

func validateKindPortMapping(cluster string) error {
	output, err := capture("docker", "port", cluster+"-control-plane", "80/tcp")
	if err != nil {
		return fmt.Errorf("inspect ingress port for kind cluster %q: %w", cluster, err)
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), ":8080") {
			return nil
		}
	}
	return fmt.Errorf("kind cluster %q uses an older ingress port mapping; run BUILDMAX_KIND_CLUSTER=%s %s kind down, then kind up", cluster, cluster, mk())
}

func ensureKindNamespace(namespace string) error {
	manifest, err := captureKindKubectl("create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	if err != nil {
		return fmt.Errorf("render namespace %s: %w", namespace, err)
	}
	return runStdin(manifest, "kubectl", "--context", kindContext(), "apply", "-f", "-")
}

func initializeKindBucket() error {
	_ = kindKubectl("delete", "job/minio-init", "-n", "storage", "--ignore-not-found")
	if err := kindKubectl("apply", "-f", "setup/minio-init.yaml"); err != nil {
		return err
	}
	if err := kindKubectl("wait", "--for=condition=complete", "job/minio-init", "-n", "storage", "--timeout=180s"); err != nil {
		_ = kindKubectl("logs", "job/minio-init", "-n", "storage")
		return err
	}
	return nil
}

func applyKindSecret() error {
	jwt, err := randomHex(32)
	if err != nil {
		return err
	}
	workerToken, err := randomHex(24)
	if err != nil {
		return err
	}
	manifest, err := captureKindKubectl(
		"create", "secret", "generic", "buildmax-secret", "-n", "buildmax",
		"--from-literal=BUILDMAX_JWT_SECRET="+jwt,
		"--from-literal=BUILDMAX_DATABASE_PASSWORD=buildmax",
		"--from-literal=BUILDMAX_STORAGE_MINIO_ACCESS_KEY=minio",
		"--from-literal=BUILDMAX_STORAGE_MINIO_SECRET_KEY=minio123",
		"--from-literal=BUILDMAX_WORKER_TOKEN="+workerToken,
		"--from-literal=BUILDMAX_CONVERSATION_MODEL_API_KEY=smoke-key",
		"--dry-run=client", "-o", "yaml",
	)
	if err != nil {
		return fmt.Errorf("render local buildmax secret: %w", err)
	}
	return runStdin(manifest, "kubectl", "--context", kindContext(), "apply", "-f", "-")
}

func applyKindSmokeConfig() error {
	manifest, err := captureKindKubectl(
		"create", "configmap", "buildmax-config", "-n", "buildmax",
		"--from-file=server.yaml=deployment/smoke/server.kind.yaml",
		"--dry-run=client", "-o", "yaml",
	)
	if err != nil {
		return fmt.Errorf("render kind smoke config: %w", err)
	}
	return runStdin(manifest, "kubectl", "--context", kindContext(), "apply", "-f", "-")
}

func randomHex(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate local secret: %w", err)
	}
	return hex.EncodeToString(data), nil
}

func requireCommands(names ...string) error {
	var missing []string
	for _, name := range names {
		if !have(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required commands: %s", strings.Join(missing, ", "))
	}
	return nil
}
