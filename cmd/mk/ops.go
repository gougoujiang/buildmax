package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// cmdInstall copies the built binaries into ~/.local/bin. The CLI is required;
// the others are copied when present, so `build cli` followed by `install` is a
// valid shortcut.
func cmdInstall() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	localBin := filepath.Join(home, ".local", "bin")

	cliSrc := filepath.Join(binDir, exe(cliBinary))
	if !exists(cliSrc) {
		return fmt.Errorf("%s not found. Run %s build first", cliSrc, mk())
	}
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		return err
	}
	fmt.Printf("Installing BuildMax binaries to %s...\n", localBin)
	for _, binary := range []string{cliBinary, serverBinary, workerBinary, desktopBinary} {
		src := filepath.Join(binDir, exe(binary))
		if !exists(src) {
			fmt.Printf("Note: %s not found, skip. Run %s build to build it.\n", exe(binary), mk())
			continue
		}
		dst := filepath.Join(localBin, exe(binary))
		fmt.Printf("Copying %s to %s\n", exe(binary), dst)
		if err := copyFile(src, dst, 0o755); err != nil {
			return err
		}
	}

	fmt.Println()
	if onPath(localBin) {
		fmt.Printf("%s is already in your PATH.\n", localBin)
	} else {
		printPathHint(localBin)
	}
	fmt.Println()
	fmt.Println("Installation complete!")
	return nil
}

func onPath(dir string) bool {
	want := filepath.Clean(dir)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == "" {
			continue
		}
		got := filepath.Clean(entry)
		if runtime.GOOS == "windows" {
			if strings.EqualFold(got, want) {
				return true
			}
			continue
		}
		if got == want {
			return true
		}
	}
	return false
}

func printPathHint(dir string) {
	fmt.Printf("%s is not in your PATH.\n", dir)
	fmt.Println("To run the binaries from any directory, add it:")
	if runtime.GOOS == "windows" {
		fmt.Printf("  setx PATH \"%%PATH%%;%s\"\n", dir)
		fmt.Println()
		fmt.Println("Then open a new terminal for the change to take effect.")
		return
	}
	fmt.Printf("  export PATH=\"%s:$PATH\"\n", dir)
	fmt.Println()
	fmt.Println("To make it permanent, add that line to your shell config:")
	fmt.Printf("  echo 'export PATH=\"%s:$PATH\"' >> ~/.zshrc   # or ~/.bashrc\n", dir)
	fmt.Println()
	fmt.Println("Then start a new terminal session, or source the file you edited.")
}

// cmdSetupScript runs one of the setup/ shell scripts. These drive kind, helm,
// kubectl, and awscli through bash, so they stay Unix-only; Windows users run
// them from WSL2.
func cmdSetupScript(name string) error {
	script := filepath.Join("setup", name)
	if !exists(script) {
		return fmt.Errorf("%s not found", script)
	}
	if runtime.GOOS == "windows" {
		return fmt.Errorf("%s is a bash script; run it from WSL2, macOS, or Linux", script)
	}
	abs, err := filepath.Abs(script)
	if err != nil {
		return err
	}
	return runCmd(abs)
}

// cmdPubImages builds the server and portal images and loads them into the
// local kind cluster, which has no registry to pull from.
func cmdPubImages() error {
	cluster := os.Getenv("BUILDMAX_KIND_CLUSTER")
	if cluster == "" {
		cluster = "buildmaxdev"
	}
	var platform []string
	if p := os.Getenv("BUILDMAX_IMAGE_PLATFORM"); p != "" {
		platform = []string{"--platform", p}
	}

	images := []struct {
		tag        string
		dockerfile string
	}{
		{"buildmax:local", filepath.Join("deployment", "docker", "Dockerfile.buildmax")},
		{"buildmax-portal:local", filepath.Join("deployment", "docker", "Dockerfile.portal")},
	}
	for _, image := range images {
		fmt.Printf("Building image %s...\n", image.tag)
		args := append([]string{"build", "-f", image.dockerfile}, platform...)
		args = append(args, "-t", image.tag, ".")
		if err := runCmd("docker", args...); err != nil {
			return fmt.Errorf("docker build of %s failed: %w", image.tag, err)
		}
	}
	fmt.Printf("Loading images into kind cluster %q...\n", cluster)
	for _, image := range images {
		if err := runCmd("kind", "load", "docker-image", image.tag, "--name", cluster); err != nil {
			return fmt.Errorf("kind load of %s failed: %w", image.tag, err)
		}
	}
	fmt.Println("Done. buildmax:local and buildmax-portal:local are available in kind.")
	return nil
}

func cmdDeploy() error {
	fmt.Println("Deploying buildmax server to kind cluster...")
	if err := cmdBuild(nil); err != nil {
		return err
	}
	if err := cmdPubImages(); err != nil {
		return err
	}
	fmt.Println("Ensuring namespace buildmax...")
	if err := ensureNamespace("buildmax"); err != nil {
		return err
	}
	if err := applySecret(); err != nil {
		return err
	}
	fmt.Println("Applying buildmax-deploy.yaml...")
	if err := runCmd("kubectl", "apply", "-f", filepath.Join("deployment", "buildmax-deploy.yaml")); err != nil {
		return err
	}
	fmt.Println("Restarting deployments...")
	for _, deployment := range []string{"buildmax-server", "buildmax-portal"} {
		if err := runCmd("kubectl", "rollout", "restart", "deployment", deployment, "-n", "buildmax"); err != nil {
			return err
		}
	}
	fmt.Println("Deployed. Add to /etc/hosts: 127.0.0.1 buildmax-api.kind.local buildmax.kind.local")
	fmt.Println("Then open the portal: http://buildmax.kind.local")
	return nil
}

// ensureNamespace is the idempotent `kubectl create --dry-run | kubectl apply`
// pattern. It is written as an explicit pipe rather than tolerating an
// "already exists" error, which would also hide real failures.
func ensureNamespace(namespace string) error {
	manifest, err := capture("kubectl", "create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	if err != nil {
		return fmt.Errorf("could not render namespace %s: %w", namespace, err)
	}
	return runStdin(manifest, "kubectl", "apply", "-f", "-")
}

func applySecret() error {
	local := filepath.Join("deployment", "buildmax-secret.local.yaml")
	if exists(local) {
		fmt.Println("Applying buildmax-secret.local.yaml...")
		return runCmd("kubectl", "apply", "-f", local)
	}
	if succeeds("kubectl", "get", "secret", "buildmax-secret", "-n", "buildmax") {
		fmt.Println("Using the existing buildmax-secret in the cluster (no local secret file).")
		return nil
	}
	return fmt.Errorf("no credentials for the deployment.\n"+
		"  cp deployment/buildmax-secret.example.yaml %s\n"+
		"  # fill in real values, then re-run %s deploy", local, mk())
}
