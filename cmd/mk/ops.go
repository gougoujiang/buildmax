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

// cmdPubImages builds the server and portal images and loads them into the
// local kind cluster, which has no registry to pull from.
func cmdPubImages() error {
	cluster := os.Getenv("BUILDMAX_KIND_CLUSTER")
	if cluster == "" {
		cluster = "buildmaxdev"
	}
	return buildAndLoadKindImages(cluster, false)
}

func buildAndLoadKindImages(cluster string, includeSmoke bool) error {
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
	if includeSmoke {
		images = append(images, struct {
			tag        string
			dockerfile string
		}{"buildmax-smoke-llm:local", "deployment/smoke/mock-llm/Dockerfile"})
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
	fmt.Println("Kind images are loaded.")
	return nil
}
