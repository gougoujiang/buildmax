package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	defaultKindCluster = "buildmaxdev"
	kindPortalURL      = "http://localhost:8080"

	// kind is a plain Go program, so it runs from a pin here rather than from a
	// contributor's PATH — the same treatment golangci-lint, actionlint, and the
	// rest already get. Docker and kubectl cannot be handled this way, which is
	// why they stay in requireCommands. Pinning also means one version creates,
	// inspects, and deletes a cluster; a PATH install cannot promise that.
	// The first run builds it in a few seconds, then the Go build cache serves it.
	kindPkg = "sigs.k8s.io/kind@v0.31.0"

	kindConfigPath = "deployment/dev-kind/kind-config.yaml"
)

func runKind(args ...string) error {
	return runCmd("go", append([]string{"run", kindPkg}, args...)...)
}

func captureKind(args ...string) (string, error) {
	return captureErr("go", append([]string{"run", kindPkg}, args...)...)
}

func cmdKind(args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return usageErrorf("kind", "kind needs an action")
	}
	switch args[0] {
	case "up":
		return kindUp()
	case "images":
		return cmdPubImages()
	case "smoke":
		managed, err := composeSmokeMode(args[1:])
		if err != nil {
			return err
		}
		if managed {
			return kindManagedSmoke()
		}
		return kindSmoke()
	case "status":
		return kindStatus()
	case "logs":
		return kindLogs()
	case "down":
		return kindDown()
	default:
		return usageErrorf("kind", "unknown kind action: %s", args[0])
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
	if err := requireCommands("docker", "kubectl"); err != nil {
		return err
	}
	if !succeeds("docker", "info") {
		return errors.New("docker is installed but the engine is not ready")
	}

	cluster := kindClusterName()
	previousContext, _ := capture("kubectl", "config", "current-context")
	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		// Before the cluster, not after: creating one is the step that publishes
		// the ports, and everything expensive comes later. A conflict found here
		// costs a second; found by the ingress, it costs an image build first.
		if err := checkKindHostPorts(cluster); err != nil {
			return err
		}
		fmt.Printf("Creating kind cluster %q...\n", cluster)
		if err := runKind("create", "cluster", "--name", cluster, "--config", kindConfigPath); err != nil {
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
	if err := kindKubectl("apply", "-f", "deployment/dev-kind/kind-ingress-nginx.yaml"); err != nil {
		return err
	}
	if err := waitForKindDeployment("ingress-nginx", "ingress-nginx-controller", "180s"); err != nil {
		return err
	}
	if err := ensureKindNamespace("storage"); err != nil {
		return err
	}
	for _, manifest := range []string{"deployment/dev-kind/mysql.yaml", "deployment/dev-kind/minio.yaml"} {
		if err := kindKubectl("apply", "-f", manifest); err != nil {
			return err
		}
	}
	if err := waitForKindDeployment("db", "mysql", "360s"); err != nil {
		return err
	}
	if err := waitForKindDeployment("storage", "minio", "360s"); err != nil {
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

// kindStatus reports what the selected cluster is running without changing it,
// so a contributor can tell "nothing deployed" from "deployed but unhealthy"
// before reaching for the much noisier kind logs.
func kindStatus() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	cluster := kindClusterName()
	fmt.Printf("Cluster: %s (context %s)\n", cluster, kindContext())

	exists, err := kindClusterExists(cluster)
	if err != nil {
		return err
	}
	if !exists {
		fmt.Printf("The cluster does not exist. Run %s kind up.\n", mk())
		return nil
	}
	if err := validateKindPortMapping(cluster); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}
	fmt.Printf("Portal:  %s (%s)\n", kindPortalURL, httpHealth(kindPortalURL+"/healthz"))

	printKindSection("Nodes", "get", "nodes", "-o", "wide")
	for _, namespace := range []string{"ingress-nginx", "db", "storage", "buildmax"} {
		printKindSection("Namespace "+namespace, "get", "deployments,jobs,pods", "-n", namespace)
	}
	fmt.Printf("\nRun %s kind logs for events and container logs.\n", mk())
	return nil
}

func printKindSection(title string, args ...string) {
	fmt.Printf("\n%s\n", title)
	if err := kindKubectl(args...); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}
}

func kindLogs() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	for _, namespace := range []string{"ingress-nginx", "db", "storage", "buildmax"} {
		dumpKindNamespace(namespace)
	}
	return nil
}

func waitForKindDeployment(namespace, name, timeout string) error {
	err := kindKubectl("wait", "--for=condition=Available", "deployment/"+name, "-n", namespace, "--timeout="+timeout)
	if err != nil {
		dumpKindNamespace(namespace)
	}
	return err
}

func dumpKindNamespace(namespace string) {
	commands := [][]string{
		{"get", "pods,jobs,deployments,services,ingresses", "-n", namespace, "-o", "wide"},
		{"get", "events", "-n", namespace, "--sort-by=.lastTimestamp"},
	}

	switch namespace {
	case "ingress-nginx":
		commands = append(commands,
			[]string{"describe", "deployment/ingress-nginx-controller", "-n", namespace},
			[]string{"logs", "-n", namespace, "deployment/ingress-nginx-controller", "--all-containers", "--tail=200"},
		)
	case "db":
		commands = append(commands,
			[]string{"describe", "deployment/mysql", "-n", namespace},
			[]string{"logs", "-n", namespace, "deployment/mysql", "--all-containers", "--tail=200"},
		)
	case "storage":
		commands = append(commands,
			[]string{"describe", "deployment/minio", "-n", namespace},
			[]string{"logs", "-n", namespace, "deployment/minio", "--all-containers", "--tail=200"},
			[]string{"logs", "-n", namespace, "job/minio-init", "--all-containers", "--tail=200"},
		)
	case "buildmax":
		commands = append(commands,
			[]string{"logs", "-n", namespace, "deployment/buildmax-server", "--all-containers", "--tail=200"},
			[]string{"logs", "-n", namespace, "deployment/buildmax-portal", "--all-containers", "--tail=100"},
			[]string{"logs", "-n", namespace, "-l", "job-name", "--all-containers", "--tail=200"},
		)
	}

	for _, args := range commands {
		if err := kindKubectl(args...); err != nil {
			fmt.Printf("Warning: %v\n", err)
		}
	}
}

func kindDown() error {
	// Deleting a cluster still talks to the container engine, so docker is the
	// prerequisite here even though kind itself no longer is.
	if err := requireCommands("docker"); err != nil {
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
	return runKind("delete", "cluster", "--name", cluster)
}

// checkKindHostPorts fails when something already holds a port the new cluster
// would publish.
//
// The ports are read from the cluster config rather than repeated here, so the
// preflight cannot drift from the mapping it is checking for.
func checkKindHostPorts(cluster string) error {
	config, err := os.ReadFile(kindConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", kindConfigPath, err)
	}
	var busy []string
	for _, match := range regexp.MustCompile(`(?m)^\s*hostPort:\s*(\d+)`).FindAllStringSubmatch(string(config), -1) {
		// Dial rather than bind. A probe that binds proves nothing here: every
		// listener that matters sets SO_REUSEADDR, and so does net.Listen, so on
		// macOS the probe succeeds against a port that is very much in use. What
		// this needs to know is whether something answers, which is a connection.
		conn, dialErr := net.DialTimeout("tcp", "127.0.0.1:"+match[1], 300*time.Millisecond)
		if dialErr != nil {
			continue
		}
		_ = conn.Close()
		busy = append(busy, match[1])
	}
	if len(busy) == 0 {
		return nil
	}
	return fmt.Errorf("host port %s already in use, and kind cluster %q publishes it\n  Stop what is listening, or run another cluster with BUILDMAX_KIND_CLUSTER=<name> after changing the hostPort in %s",
		strings.Join(busy, " and "), cluster, kindConfigPath)
}

func kindClusterExists(name string) (bool, error) {
	output, err := captureKind("get", "clusters")
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
	if err := kindKubectl("apply", "-f", "deployment/dev-kind/minio-init.yaml"); err != nil {
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
	return applyKindSmokeConfigFrom("deployment/smoke/server.kind.yaml")
}

func applyKindSmokeConfigFrom(path string) error {
	manifest, err := captureKindKubectl(
		"create", "configmap", "buildmax-config", "-n", "buildmax",
		"--from-file=server.yaml="+path,
		"--dry-run=client", "-o", "yaml",
	)
	if err != nil {
		return fmt.Errorf("render kind smoke config: %w", err)
	}
	return runStdin(manifest, "kubectl", "--context", kindContext(), "apply", "-f", "-")
}

// kindManagedSmoke reruns the smoke against a cluster switched to managed
// task-run inference.
//
// It swaps the ConfigMap and restarts the server rather than standing up a
// second cluster: transport is startup configuration, so the server has to
// reread it, but everything else about the deployment is the same. Worker pods
// pick up the new file because each Job mounts the ConfigMap when it starts.
//
// The cluster is left in managed mode afterwards. Rerun `./make kind up` to put
// it back, which is also what a contributor would do to get a clean stack.
func kindManagedSmoke() error {
	if err := requireCommands("kubectl"); err != nil {
		return err
	}
	if err := applyKindSmokeConfigFrom("deployment/smoke/server.kind.managed.yaml"); err != nil {
		return err
	}
	if err := kindKubectl("rollout", "restart", "deployment/buildmax-server", "-n", "buildmax"); err != nil {
		return err
	}
	if err := kindKubectl("rollout", "status", "deployment/buildmax-server", "-n", "buildmax", "--timeout=180s"); err != nil {
		return err
	}
	target := kindSmokeTarget()
	target.managedLLM = true
	if err := runDeploymentSmoke(context.Background(), target); err != nil {
		return err
	}
	printSmokeLogin(target)
	return nil
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
