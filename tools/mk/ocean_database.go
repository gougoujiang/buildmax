package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const oceanDatabaseProxyName = "buildmax-database-proxy"

func oceanDatabase(cfg oceanConfig, args []string) error {
	if len(args) == 0 {
		return usageErrorf("ocean", "ocean database needs an action")
	}
	if len(args) > 1 {
		return usageErrorf("ocean", "ocean database takes exactly one action")
	}
	if args[0] != "forward" {
		return usageErrorf("ocean", "unknown ocean database action: %s", args[0])
	}
	return oceanDatabaseForward(cfg)
}

func oceanDatabaseForward(cfg oceanConfig) error {
	if err := requireCommands("tofu", "kubectl"); err != nil {
		return err
	}
	if !exists(oceanStatePath(cfg)) || !exists(oceanKubeconfigPath(cfg)) {
		return fmt.Errorf("ocean infrastructure is not ready; run `%s ocean up` first", mk())
	}
	localPort, err := oceanDatabaseLocalPort()
	if err != nil {
		return err
	}
	if err := oceanInit(cfg); err != nil {
		return err
	}
	host, err := oceanOutput(cfg, "database_private_host")
	if err != nil {
		return err
	}
	port, err := oceanOutput(cfg, "database_port")
	if err != nil {
		return err
	}
	user, err := oceanOutput(cfg, "database_user")
	if err != nil {
		return err
	}
	ca, err := oceanOutput(cfg, "database_ca")
	if err != nil {
		return err
	}
	if err := os.WriteFile(oceanDatabaseCAPath(cfg), []byte(ca+"\n"), 0o600); err != nil {
		return fmt.Errorf("write Ocean database CA: %w", err)
	}

	proxy := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      oceanDatabaseProxyName,
			"namespace": oceanNamespace,
		},
		"spec": map[string]any{
			"replicas": 1,
			"selector": map[string]any{"matchLabels": map[string]string{"app": oceanDatabaseProxyName}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]string{"app": oceanDatabaseProxyName}},
				"spec": map[string]any{
					"containers": []any{map[string]any{
						"name":    "proxy",
						"image":   defaultOceanBuildMaxImage,
						"command": []string{"/bin/sh", "-ec"},
						"args":    []string{`exec nc -lk -p 3306 -e nc "$DATABASE_HOST" "$DATABASE_PORT"`},
						"env": []any{
							map[string]any{"name": "DATABASE_HOST", "value": host},
							map[string]any{"name": "DATABASE_PORT", "value": port},
						},
						"ports": []any{map[string]any{"name": "mysql", "containerPort": 3306}},
						"securityContext": map[string]any{
							"allowPrivilegeEscalation": false,
							"readOnlyRootFilesystem":   true,
							"capabilities":             map[string]any{"drop": []string{"ALL"}},
						},
						"resources": map[string]any{
							"requests": map[string]string{"cpu": "5m", "memory": "8Mi"},
							"limits":   map[string]string{"cpu": "50m", "memory": "32Mi"},
						},
					}},
				},
			},
		},
	}
	if err := oceanApplyObject(cfg, proxy); err != nil {
		return fmt.Errorf("create Ocean database proxy: %w", err)
	}
	if err := oceanKubectl(cfg, "rollout", "status", "deployment/"+oceanDatabaseProxyName, "--namespace", oceanNamespace, "--timeout=120s"); err != nil {
		return fmt.Errorf("wait for Ocean database proxy: %w", err)
	}

	fmt.Printf("\nForwarding the private MySQL endpoint to 127.0.0.1:%d. Press Ctrl-C to stop.\n", localPort)
	fmt.Printf("CA certificate: %s\n", oceanDatabaseCAPath(cfg))
	fmt.Printf("MySQL CLI (password is prompted):\n  mysql --host 127.0.0.1 --port %d --user %s --password --ssl-mode=VERIFY_CA --ssl-ca=%s buildmax\n\n", localPort, user, oceanDatabaseCAPath(cfg))
	return oceanKubectl(cfg, "port-forward", "--namespace", oceanNamespace, "--address", "127.0.0.1", "deployment/"+oceanDatabaseProxyName, fmt.Sprintf("%d:3306", localPort))
}

func oceanDatabaseLocalPort() (int, error) {
	localPort, err := strconv.Atoi(envOr("BUILDMAX_OCEAN_DATABASE_LOCAL_PORT", "13306"))
	if err != nil || localPort < 1 || localPort > 65535 {
		return 0, errors.New("BUILDMAX_OCEAN_DATABASE_LOCAL_PORT must be a TCP port from 1 to 65535")
	}
	return localPort, nil
}

func oceanDatabaseCAPath(cfg oceanConfig) string {
	return filepath.Join(cfg.stateDir, "database-ca.pem")
}
