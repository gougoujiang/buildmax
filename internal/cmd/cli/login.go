package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"buildmax/internal/auth"
	"buildmax/internal/config"

	"github.com/spf13/cobra"
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

	serverDefault := config.WorkerServerURL()
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
	client := auth.NewClient(serverURL)

	if err := client.RequestOTP(ctx, email, "login"); err != nil {
		return fmt.Errorf("request OTP: %w", err)
	}
	fmt.Fprintln(os.Stdout, "OTP sent.")

	fmt.Fprint(os.Stdout, "OTP: ")
	otp := readLine(reader)
	if otp == "" {
		return fmt.Errorf("OTP is required")
	}

	lr, err := client.Login(ctx, email, otp, "cli")
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	creds := &auth.Credentials{
		ServerURL: serverURL,
		Token:     lr.Token,
		UserID:    lr.User.ID,
		Email:     lr.User.Email,
		Name:      lr.User.Name,
	}
	if err := auth.Save(creds, config.AuthPath()); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Logged in as %s on %s\n", lr.User.Email, serverURL)
	return nil
}

func runLogout(_ *cobra.Command, _ []string) error {
	if err := auth.Clear(config.AuthPath()); err != nil {
		return fmt.Errorf("clear credentials: %w", err)
	}
	fmt.Fprintln(os.Stdout, "Logged out.")
	return nil
}

func runWhoami(_ *cobra.Command, _ []string) error {
	creds, err := auth.Load(config.AuthPath())
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	if !creds.IsValid() {
		fmt.Fprintln(os.Stdout, "Not logged in.")
		return nil
	}
	fmt.Fprintf(os.Stdout, "Logged in as %s on %s\n", creds.Email, creds.ServerURL)
	return nil
}

func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}
