package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
	"github.com/gougoujiang/buildmax/internal/interface/client"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const defaultServerURL = "http://localhost:5678"

func newLoginCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to a BuildMax server",
		RunE:  runLogin,
	}
}

func newLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out and clear stored credentials",
		RunE:  runLogout,
	}
}

func newWhoamiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current login status",
		RunE:  runWhoami,
	}
}

func runLogin(_ *cobra.Command, _ []string) error {
	return interactiveLogin()
}

// interactiveLogin prompts for server URL, email, and OTP, then saves
// credentials on success. Used by both the login subcommand and the TUI
// startup gate.
func interactiveLogin() error {
	reader := bufio.NewReader(os.Stdin)
	s, _ := config.LoadSettings()
	serverDefault := s.ServerURL
	if serverDefault == "" {
		serverDefault = defaultServerURL
	}
	fmt.Fprintf(os.Stdout, "Server URL [%s]: ", serverDefault)
	serverURL := readLine(reader)
	if serverURL == "" {
		serverURL = serverDefault
	}

	fmt.Fprint(os.Stdout, "Email: ")
	email := readLine(reader)
	if email == "" {
		return fmt.Errorf("email is required")
	}

	ctx := context.Background()
	c := client.NewClient(serverURL)

	// Password first, since that is the everyday way in. An empty one falls
	// through to a login code, which is how someone claims a new account or
	// recovers a forgotten password — there is no mail channel, so an operator
	// issues that code by hand.
	password, err := readPassword("Password (leave blank to use a login code): ")
	if err != nil {
		return err
	}

	var lr *client.LoginResponse
	if password != "" {
		lr, err = c.LoginWithPassword(ctx, email, password, "cli")
		if err != nil {
			return fmt.Errorf("login: %w", err)
		}
	} else {
		fmt.Fprintln(os.Stdout, "Ask an administrator to run: buildmax-server user login-code "+email)
		fmt.Fprint(os.Stdout, "Login code: ")
		otp := readLine(reader)
		if otp == "" {
			return fmt.Errorf("a password or a login code is required")
		}
		lr, err = c.Login(ctx, email, otp, "cli")
		if err != nil {
			return fmt.Errorf("login: %w", err)
		}
	}

	creds := &auth.Credentials{
		ServerURL:    serverURL,
		Token:        lr.Access(),
		RefreshToken: lr.RefreshToken,
		UserID:       lr.User.ID,
		Email:        lr.User.Email,
		Name:         lr.User.Name,
	}
	if err := auth.SaveCredentials(creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Logged in as %s on %s\n", lr.User.Email, serverURL)
	return nil
}

func runLogout(_ *cobra.Command, _ []string) error {
	err := auth.LogoutAndRevoke()
	fmt.Fprintln(os.Stdout, "Logged out.")
	if err != nil {
		// The credentials are gone either way. Say what did not happen rather
		// than reporting a failure for something that succeeded locally.
		fmt.Fprintf(os.Stderr, "warning: the session may still be active on the server: %v\n", err)
	}
	return nil
}

func runWhoami(_ *cobra.Command, _ []string) error {
	info, err := auth.Info()
	if err != nil {
		return fmt.Errorf("load auth: %w", err)
	}
	if !info.LoggedIn {
		fmt.Fprintln(os.Stdout, "Not logged in.")
		return nil
	}
	fmt.Fprintf(os.Stdout, "Logged in as %s on %s\n", info.Email, info.ServerURL)
	return nil
}

func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

// readPassword prompts and reads without echoing.
//
// When stdin is not a terminal — a pipe, a script — it reads a line normally.
// There is nothing to hide from in that case, and refusing would make the
// command unusable from anything but an interactive shell.
func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt)
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", nil
		}
		return strings.TrimSpace(line), nil
	}
	raw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}
