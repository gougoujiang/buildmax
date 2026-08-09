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

	if err := c.RequestOTP(ctx, email, "login"); err != nil {
		return fmt.Errorf("request OTP: %w", err)
	}
	fmt.Fprintln(os.Stdout, "OTP sent.")

	fmt.Fprint(os.Stdout, "OTP: ")
	otp := readLine(reader)
	if otp == "" {
		return fmt.Errorf("OTP is required")
	}

	lr, err := c.Login(ctx, email, otp, "cli")
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
	if err := auth.SaveCredentials(creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Logged in as %s on %s\n", lr.User.Email, serverURL)
	return nil
}

func runLogout(_ *cobra.Command, _ []string) error {
	if err := auth.Logout(); err != nil {
		return fmt.Errorf("clear credentials: %w", err)
	}
	fmt.Fprintln(os.Stdout, "Logged out.")
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
