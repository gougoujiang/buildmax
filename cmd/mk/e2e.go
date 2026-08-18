package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// cmdE2E runs the Portal browser tests against a deployment that is already
// running.
//
// It does not start one. The tests are about whether the published bundle works
// against a real server, so pointing them at a stack someone else brought up —
// kind or Compose locally, the smoke jobs in CI — is the whole idea. Starting a
// private one here would test a different thing.
//
// The two deployments differ in a way the tests can see. kind puts one ingress
// in front of Portal and server, so the bundle's API base is same-origin;
// Compose publishes them on separate ports, so it is absolute. Neither is wrong
// and the browser cannot guess which it is looking at, so the target decides and
// passes the answer in.
func cmdE2E(args []string) error {
	target, err := e2eTarget(args)
	if err != nil {
		return err
	}
	if err := requireCommands("node", "npm"); err != nil {
		return err
	}
	baseURL := envOr("BUILDMAX_E2E_BASE_URL", target.portalURL)

	ctx := context.Background()
	client := &http.Client{Timeout: 5 * time.Second}
	if err := waitForHTTP(ctx, client, baseURL, 15*time.Second); err != nil {
		return fmt.Errorf("no deployment is answering at %s: %w\nStart one with `%s kind up` or `%s compose smoke`, or set BUILDMAX_E2E_BASE_URL", baseURL, err, mk(), mk())
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
	codeOutput, err := target.admin("user", "login-code", smokeEmail)
	if err != nil {
		return fmt.Errorf("issue a login code: %w", err)
	}
	code := loginCodePattern.FindString(codeOutput)
	if code == "" {
		return errors.New("the login-code command returned no bmxlogin_ code")
	}

	fmt.Printf("[e2e] running Portal browser tests against %s\n", baseURL)
	// npm run rather than npx: npx will happily resolve Playwright from a
	// global cache, where it cannot see the config's own @playwright/test
	// import. The local bin is the only one that works.
	return runWith("portal", []string{
		"BUILDMAX_E2E_BASE_URL=" + baseURL,
		"BUILDMAX_E2E_API_BASE=" + target.portalRuntimeAPIBase,
		"BUILDMAX_E2E_EMAIL=" + smokeEmail,
		"BUILDMAX_E2E_LOGIN_CODE=" + code,
	}, "npm", "run", "e2e")
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
func e2eTarget(args []string) (smokeTarget, error) {
	name := "kind"
	if len(args) > 0 {
		name = args[0]
	}
	if len(args) > 1 {
		return smokeTarget{}, fmt.Errorf("usage: %s e2e [kind|compose]", mk())
	}
	switch name {
	case "kind":
		return kindSmokeTarget(), nil
	case "compose":
		// The browser tests read a deployment; they do not care which transport
		// its task runs use, and the file list only affects which server.yaml a
		// fresh stack would mount.
		return composeSmokeTarget(false), nil
	default:
		return smokeTarget{}, fmt.Errorf("unknown e2e target %q (want kind or compose)", name)
	}
}
