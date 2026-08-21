package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// artifactDir is where a failed suite leaves its evidence. One predictable
// place, cleared at the start of a run, so "read the artifact" is an
// instruction rather than a hunt.
const artifactDir = ".artifacts/e2e"

// cmdE2E runs one named end-to-end suite.
//
// The suites differ in what they own. The Portal ones attach to a deployment
// someone else brought up — kind or Compose locally, the smoke jobs in CI —
// because what they prove is that the published bundle works against a real
// server, and a stack this command started privately would be a different
// claim. `local` is the exception: it owns a throwaway Compose stack for one
// run, which is the shape a contributor wants when there is nothing running
// yet. `cli` owns nothing but a temporary directory.
//
// Whichever it is, the command says so before it starts. A suite that silently
// chose a mode leaves the reader guessing what its result covered.
//
// The two deployments differ in a way the tests can see. kind puts one ingress
// in front of Portal and server, so the bundle's API base is same-origin;
// Compose publishes them on separate ports, so it is absolute. Neither is wrong
// and the browser cannot guess which it is looking at, so the target decides and
// passes the answer in.
func cmdE2E(args []string) error {
	suite := "kind"
	if len(args) > 0 {
		suite = args[0]
	}
	if len(args) > 1 {
		return e2eUsage()
	}
	switch suite {
	case "cli":
		return e2eCLI()
	case "local":
		return e2eOwningCompose()
	case "kind", "compose":
		target, err := e2eTarget(suite)
		if err != nil {
			return err
		}
		fmt.Printf("[e2e] attaching to the %s deployment at %s (this command did not start it)\n", suite, target.portalURL)
		return e2ePortal(target, suite)
	default:
		return e2eUsage()
	}
}

func e2eUsage() error {
	return fmt.Errorf("usage: %s e2e [kind|compose|local|cli]\n"+
		"  kind     Portal browser tests against a running kind deployment (the default)\n"+
		"  compose  Portal browser tests against a running Compose stack\n"+
		"  local    the same tests against a Compose stack this command starts and stops\n"+
		"  cli      the CLI and TUI suite: the built binary, a temporary home, no deployment", mk())
}

// e2eCLI runs the suite that needs no deployment at all. It is listed here so
// an agent choosing a suite finds every suite in one place, rather than
// learning that this one is spelled as a Go test.
func e2eCLI() error {
	fmt.Println("[e2e] CLI and TUI suite: the built binary, a temporary home, and a scripted model")
	// -count=1 because a cached pass is not evidence that the binary built from
	// the current tree still works.
	return runCmd("go", "test", "-count=1", "./internal/e2e/cli/...")
}

// e2eOwningCompose brings up a Compose stack, runs the browser tests, and takes
// it down again. Failure still tears down: a stack left running after a failed
// run is a trap for the next one, which would attach to it and report on the
// wrong deployment.
func e2eOwningCompose() error {
	// Owning a stack means taking it down afterwards, so refuse to adopt one
	// that is already up: it may be someone's running deployment, and this
	// command would end it. Attaching is what that case wants.
	probe := &http.Client{Timeout: 2 * time.Second}
	if err := waitForHTTP(context.Background(), probe, composePortalURL(), 2*time.Second); err == nil {
		return fmt.Errorf("a Compose stack is already answering at %s, and `%s e2e local` would take it down when it finished\nTest the stack you have with `%s e2e compose`, or stop it first with `%s compose down`",
			composePortalURL(), mk(), mk(), mk())
	}
	fmt.Println("[e2e] owning a Compose stack for this run: starting it, testing it, and taking it down")
	// The smoke overlay, not a plain `up`: the browser tests drive a run to
	// completion, which needs the deterministic model in front of the server
	// rather than a provider key this machine may not have.
	if err := composeUpSmokeStack(false); err != nil {
		return fmt.Errorf("start the Compose stack: %w", err)
	}
	testErr := e2ePortal(composeSmokeTarget(false), "local")
	if downErr := cmdCompose([]string{"down"}); downErr != nil && testErr == nil {
		return fmt.Errorf("take the Compose stack down: %w", downErr)
	}
	return testErr
}

// e2ePortal runs the browser tests against an already-running deployment.
// invocation is the suite name this run was asked for, which is what the run
// note has to repeat: the artifact directory is named for the surface, and
// telling a reader to run `e2e portal` would name no command at all.
func e2ePortal(target smokeTarget, invocation string) error {
	if err := e2ePreflight(); err != nil {
		return err
	}
	baseURL := envOr("BUILDMAX_E2E_BASE_URL", target.portalURL)

	ctx := context.Background()
	client := &http.Client{Timeout: 5 * time.Second}
	if err := waitForHTTP(ctx, client, baseURL, 15*time.Second); err != nil {
		return fmt.Errorf("no deployment is answering at %s: %w\nStart one with `%s kind up` or `%s compose smoke`, run `%s e2e local` to have one started for you, or set BUILDMAX_E2E_BASE_URL", baseURL, err, mk(), mk(), mk())
	}
	if err := confirmDeployment(ctx, client, baseURL, target); err != nil {
		return err
	}

	// A login code arrives out of band by design, so the browser cannot fetch
	// one. Issuing it here is what lets the tests sign in.
	if output, err := target.admin("user", "create", smokeEmail); err != nil && !strings.Contains(output, "already has an account") {
		return fmt.Errorf("create the end-to-end account: %w", err)
	}
	// The same account holds the deployment-scoped grant, so the browser tests
	// can reach /admin. Granting an existing grant is refused rather than being
	// an error worth stopping for — this command runs against a deployment that
	// may already have been set up.
	if output, err := target.admin("admin", "grant", smokeEmail); err != nil && !strings.Contains(output, "already holds") {
		return fmt.Errorf("grant system_admin to the end-to-end account: %w", err)
	}
	code, err := issueLoginCode(target, smokeEmail)
	if err != nil {
		return err
	}
	// A second account with no grant of any kind. A role-specific view can only
	// be proved by someone who does not hold the role, and the account above
	// holds every one of them.
	if output, err := target.admin("user", "create", smokeOutsiderEmail); err != nil && !strings.Contains(output, "already has an account") {
		return fmt.Errorf("create the ungranted end-to-end account: %w", err)
	}
	memberCode, err := issueLoginCode(target, smokeOutsiderEmail)
	if err != nil {
		return err
	}

	artifacts, runID, err := prepareArtifacts("portal", invocation, baseURL)
	if err != nil {
		return err
	}

	fmt.Printf("[e2e] running Portal browser tests against %s\n", baseURL)
	fmt.Printf("[e2e] artifacts from this run: %s\n", artifacts)
	// npm run rather than npx: npx will happily resolve Playwright from a
	// global cache, where it cannot see the config's own @playwright/test
	// import. The local bin is the only one that works.
	return runWith("portal", []string{
		"BUILDMAX_E2E_BASE_URL=" + baseURL,
		"BUILDMAX_E2E_API_BASE=" + target.portalRuntimeAPIBase,
		"BUILDMAX_E2E_EMAIL=" + smokeEmail,
		"BUILDMAX_E2E_LOGIN_CODE=" + code,
		"BUILDMAX_E2E_MEMBER_EMAIL=" + smokeOutsiderEmail,
		"BUILDMAX_E2E_MEMBER_LOGIN_CODE=" + memberCode,
		// Resources the specs create carry this, so a deployment several runs
		// old can still say which run left what behind.
		"BUILDMAX_E2E_RUN_ID=" + runID,
		// A subdirectory, because Playwright empties its output directory when a
		// run starts: the note beside it would not survive being told to write
		// there.
		"BUILDMAX_E2E_ARTIFACTS=" + filepath.Join(artifacts, "results"),
	}, "npm", "run", "e2e")
}

// issueLoginCode mints a single-use code for one account. The browser cannot
// fetch one, which is the point of an out-of-band credential.
func issueLoginCode(target smokeTarget, email string) (string, error) {
	output, err := target.admin("user", "login-code", email)
	if err != nil {
		return "", fmt.Errorf("issue a login code for %s: %w", email, err)
	}
	code := loginCodePattern.FindString(output)
	if code == "" {
		return "", fmt.Errorf("the login-code command for %s returned no bmxlogin_ code", email)
	}
	return code, nil
}

// e2ePreflight names what is missing before a browser starts. Playwright's own
// failure for an absent browser arrives minutes later and reads like a test
// failure, which sends the reader to the wrong problem.
func e2ePreflight() error {
	if err := requireCommands("node", "npm"); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join("portal", "node_modules", "@playwright", "test")); err != nil {
		return fmt.Errorf("the Portal test dependencies are not installed: run `npm --prefix portal ci`")
	}
	if dir := playwrightBrowserDir(); dir != "" {
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("no Playwright browsers in %s: run `npm --prefix portal exec -- playwright install chromium`", dir)
		}
	}
	return nil
}

// playwrightBrowserDir is where Playwright keeps its browsers by default. An
// empty string means this platform's location is not known here, in which case
// the check is skipped rather than guessed wrong.
func playwrightBrowserDir() string {
	if custom := os.Getenv("PLAYWRIGHT_BROWSERS_PATH"); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Caches", "ms-playwright")
	case "linux":
		return filepath.Join(home, ".cache", "ms-playwright")
	default:
		return ""
	}
}

// prepareArtifacts clears and recreates this suite's artifact directory and
// leaves a note in it saying what ran and how to run it again. It returns the
// absolute directory and the run id the suite tags its resources with.
//
// Clearing matters: evidence from an earlier run sitting beside this one's is
// worse than none, because it is read as this one's.
func prepareArtifacts(surface, invocation, baseURL string) (string, string, error) {
	dir, err := filepath.Abs(filepath.Join(artifactDir, surface))
	if err != nil {
		return "", "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", "", fmt.Errorf("clear %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("create %s: %w", dir, err)
	}
	runID := time.Now().UTC().Format("20060102-150405")
	note := fmt.Sprintf("surface: %s\ndeployment: %s\nrun id: %s\nreproduce: %s e2e %s\n",
		surface, baseURL, runID, mk(), invocation)
	if err := os.WriteFile(filepath.Join(dir, "run.txt"), []byte(note), 0o644); err != nil {
		return "", "", fmt.Errorf("write %s: %w", filepath.Join(dir, "run.txt"), err)
	}
	return dir, runID, nil
}

// confirmDeployment checks that the stack answering is the one that was asked
// for.
//
// Both deployments publish the Portal on port 8080 by default, so something
// answering there proves nothing about which it is. Running the kind target
// against a Compose stack otherwise fails several steps later, inside a
// `kubectl exec`, with an error about the wrong thing entirely.
//
// The runtime config is the cheapest distinguishing fact: it is written at
// container start from the deployment's own API base, which is same-origin
// behind an ingress and absolute behind published ports.
func confirmDeployment(ctx context.Context, client *http.Client, portalURL string, target smokeTarget) error {
	config, err := requestText(ctx, client, http.MethodGet, strings.TrimRight(portalURL, "/")+"/config.js", "", nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("read the Portal runtime config at %s: %w", portalURL, err)
	}
	if want := fmt.Sprintf("apiBase: %q", target.portalRuntimeAPIBase); !strings.Contains(config, want) {
		return fmt.Errorf("the deployment at %s does not look like the one requested: its runtime config is %s, which does not contain %s\nCheck which stack is running, or pass the other target to `%s e2e`",
			portalURL, strings.TrimSpace(config), want, mk())
	}
	return nil
}

// e2eTarget resolves which running deployment to test. kind stays the default
// because it is the reference deployment and what the browser job in CI brings
// up.
func e2eTarget(name string) (smokeTarget, error) {
	switch name {
	case "kind":
		return kindSmokeTarget(), nil
	case "compose":
		// The browser tests read a deployment; they do not care which transport
		// its task runs use, and the file list only affects which server.yaml a
		// fresh stack would mount.
		return composeSmokeTarget(false), nil
	}
	return smokeTarget{}, fmt.Errorf("unknown e2e target %q (want kind or compose)", name)
}
