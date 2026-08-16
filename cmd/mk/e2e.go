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
// kind locally, the smoke job in CI — is the whole idea. Starting a private one
// here would test a different thing.
func cmdE2E(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: ./make e2e")
	}
	if err := requireCommands("node", "npm"); err != nil {
		return err
	}
	baseURL := envOr("BUILDMAX_E2E_BASE_URL", kindPortalURL)

	client := &http.Client{Timeout: 5 * time.Second}
	if err := waitForHTTP(context.Background(), client, baseURL, 15*time.Second); err != nil {
		return fmt.Errorf("no deployment is answering at %s: %w\nStart one with `%s kind up`, or set BUILDMAX_E2E_BASE_URL", baseURL, err, mk())
	}

	// A login code arrives out of band by design, so the browser cannot fetch
	// one. Issuing it here is what lets the tests sign in.
	target := kindSmokeTarget()
	if output, err := target.admin("user", "create", smokeEmail); err != nil && !strings.Contains(output, "already has an account") {
		return fmt.Errorf("create the end-to-end account: %w", err)
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
		"BUILDMAX_E2E_EMAIL=" + smokeEmail,
		"BUILDMAX_E2E_LOGIN_CODE=" + code,
	}, "npm", "run", "e2e")
}
