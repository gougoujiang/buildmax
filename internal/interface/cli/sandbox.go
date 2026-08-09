package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/infra/sandbox"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func yamlUnmarshal(data []byte, out *map[string]any) error { return yaml.Unmarshal(data, out) }
func yamlMarshal(v any) ([]byte, error)                    { return yaml.Marshal(v) }

// newSandboxCommand builds `buildmax sandbox` with its subcommands. Phase A
// ships `status` only; later phases add `deps`, `mode`, `enable`, `disable`,
// and `overrides`. See docs/design/032-sandbox-and-execution-boundaries.md §8.
func newSandboxCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sandbox",
		Short: "Inspect and manage the BuildMax sandbox",
		Long: "Bash subprocess sandbox configuration. Mirrors the layout of Claude Code's /sandbox panel.\n" +
			"See docs/design/032-sandbox-and-execution-boundaries.md for the full design.",
	}
	cmd.AddCommand(newSandboxStatusCommand())
	cmd.AddCommand(newSandboxDepsCommand())
	cmd.AddCommand(newSandboxEnableCommand())
	cmd.AddCommand(newSandboxDisableCommand())
	cmd.AddCommand(newSandboxModeCommand())
	return cmd
}

func newSandboxEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Set sandbox.enabled: true in settings.yaml",
		RunE: func(_ *cobra.Command, _ []string) error {
			return mutateSandboxSetting(func(m map[string]any) { m["enabled"] = true })
		},
	}
}

func newSandboxDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Set sandbox.enabled: false in settings.yaml",
		RunE: func(_ *cobra.Command, _ []string) error {
			return mutateSandboxSetting(func(m map[string]any) { m["enabled"] = false })
		},
	}
}

func newSandboxModeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "mode <auto_allow|regular>",
		Short: "Set sandbox.auto_allow_bash_if_sandboxed in settings.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			switch args[0] {
			case "auto_allow":
				return mutateSandboxSetting(func(m map[string]any) { m["auto_allow_bash_if_sandboxed"] = true })
			case "regular":
				return mutateSandboxSetting(func(m map[string]any) { m["auto_allow_bash_if_sandboxed"] = false })
			default:
				return fmt.Errorf("mode must be auto_allow or regular, got %q", args[0])
			}
		},
	}
}

// mutateSandboxSetting reads <BUILDMAX_HOME>/settings.yaml, runs mutate on
// its `sandbox:` block (creating one if absent), and writes the file back.
// Uses raw YAML round-trip so unrelated keys / comments survive.
func mutateSandboxSetting(mutate func(sandbox map[string]any)) error {
	path := config.SettingsPath()
	var root map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if e := yamlUnmarshal(data, &root); e != nil {
			return fmt.Errorf("parse %s: %w", path, e)
		}
	}
	if root == nil {
		root = map[string]any{}
	}
	sb, _ := root["sandbox"].(map[string]any)
	if sb == nil {
		sb = map[string]any{}
		root["sandbox"] = sb
	}
	mutate(sb)
	out, err := yamlMarshal(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "wrote sandbox settings to %s\n", path)
	return nil
}

func newSandboxStatusCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: "Print the resolved sandbox config and source layers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			workspace, _ := cmd.Flags().GetString("workspace")
			app, err := agentapp.NewAgentApp(agentapp.AppConfig{
				WorkspaceDir:   workspace,
				EnableMCP:      false,
				SandboxSurface: config.SandboxSurfaceCLI,
			})
			if err != nil {
				return err
			}
			defer app.Close()
			return writeSandboxStatus(os.Stdout, app.SandboxStatus())
		},
	}
	c.Flags().String("workspace", "", "workspace directory (default: current directory)")
	return c
}

func newSandboxDepsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deps",
		Short: "Check host-side sandbox dependencies (bwrap/sandbox-exec/socat)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return writeSandboxDeps(os.Stdout, sandbox.CheckDeps())
		},
	}
}

// writeSandboxStatus renders the resolved sandbox config in a stable
// human-readable form. Extracted from the subcommand so it is unit-testable.
func writeSandboxStatus(w io.Writer, st agentapp.SandboxStatus) error {
	res := st.Resolution
	c := res.Config
	fmt.Fprintln(w, "Sandbox status")
	fmt.Fprintln(w, "==============")
	fmt.Fprintf(w, "  runtime_enabled:               %v\n", st.Enabled)
	fmt.Fprintf(w, "  runtime_backend:               %s\n", st.Backend)
	if st.ProxyAddress != "" {
		fmt.Fprintf(w, "  runtime_proxy:                 http://%s  (allows=%d denies=%d)\n",
			st.ProxyAddress, st.ProxyAllows, st.ProxyDenies)
	} else {
		fmt.Fprintln(w, "  runtime_proxy:                 (not running)")
	}
	if c.Enabled && !st.Enabled {
		fmt.Fprintln(w, "  ⚠ sandbox is enabled in settings but the OS backend is unavailable.")
		fmt.Fprintln(w, "    run `buildmax sandbox deps` to see what is missing.")
	}
	fmt.Fprintf(w, "  enabled:                       %v\n", c.Enabled)
	fmt.Fprintf(w, "  mode:                          %s\n", displayMode(c))
	fmt.Fprintf(w, "  fail_if_unavailable:           %v\n", c.FailIfUnavailable)
	fmt.Fprintf(w, "  auto_allow_bash_if_sandboxed:  %v\n", c.EffectiveAutoAllowBash())
	strict := ""
	if !c.EffectiveAllowUnsandboxed() {
		strict = "  (strict sandbox mode — dangerously_disable_sandbox ignored)"
	}
	fmt.Fprintf(w, "  allow_unsandboxed_commands:    %v%s\n", c.EffectiveAllowUnsandboxed(), strict)
	fmt.Fprintf(w, "  enable_weaker_nested_sandbox:  %v\n", c.EnableWeakerNestedSandbox)
	fmt.Fprintf(w, "  enable_weaker_network_isolation: %v\n", c.EnableWeakerNetworkIsolation)

	fmt.Fprintln(w, "  excluded_commands:")
	writeList(w, "    - ", c.ExcludedCommands)

	fmt.Fprintln(w, "  filesystem:")
	fmt.Fprintf(w, "    allow_managed_read_paths_only: %v\n", c.Filesystem.AllowManagedReadPathsOnly)
	fmt.Fprintln(w, "    allow_write:")
	writeList(w, "      - ", c.Filesystem.AllowWrite)
	fmt.Fprintln(w, "    deny_write:")
	writeList(w, "      - ", c.Filesystem.DenyWrite)
	fmt.Fprintln(w, "    allow_read:")
	writeList(w, "      - ", c.Filesystem.AllowRead)
	fmt.Fprintln(w, "    deny_read:")
	writeList(w, "      - ", c.Filesystem.DenyRead)

	fmt.Fprintln(w, "  network:")
	fmt.Fprintf(w, "    allow_managed_domains_only:  %v\n", c.Network.AllowManagedDomainsOnly)
	fmt.Fprintln(w, "    allowed_domains:")
	writeList(w, "      - ", c.Network.AllowedDomains)
	fmt.Fprintln(w, "    denied_domains:")
	writeList(w, "      - ", c.Network.DeniedDomains)
	fmt.Fprintln(w, "    allow_unix_sockets:")
	writeList(w, "      - ", c.Network.AllowUnixSockets)
	fmt.Fprintf(w, "    allow_all_unix_sockets:      %v\n", c.Network.AllowAllUnixSockets)
	fmt.Fprintf(w, "    allow_local_binding:         %v\n", c.Network.AllowLocalBinding)
	fmt.Fprintf(w, "    http_proxy_port:             %d\n", c.Network.HTTPProxyPort)
	fmt.Fprintf(w, "    socks_proxy_port:            %d\n", c.Network.SOCKSProxyPort)

	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "Sources: %s\n", strings.Join(res.Sources, " → "))
	writeRecentViolations(w, st.Recent)
	return nil
}

// writeRecentViolations prints up to 10 recent allow/deny events, hiding
// entries flagged Suppressed (via ignore_violations).
func writeRecentViolations(w io.Writer, recent []sandbox.Violation) {
	visible := make([]sandbox.Violation, 0, len(recent))
	for _, v := range recent {
		if v.Suppressed {
			continue
		}
		visible = append(visible, v)
	}
	if len(visible) == 0 {
		return
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Recent decisions:")
	for _, v := range visible {
		host := v.Host
		if host == "" {
			host = "(no host)"
		}
		fmt.Fprintf(w, "  %s  %-10s  %s  %s\n",
			v.Time.Format("15:04:05"), v.Kind, host, v.Reason)
	}
}

// writeSandboxDeps renders the host-side dependency report. Extracted so
// it is unit-testable independently of the live sandbox.CheckDeps().
func writeSandboxDeps(w io.Writer, r sandbox.DepsReport) error {
	fmt.Fprintln(w, "Sandbox dependencies")
	fmt.Fprintln(w, "====================")
	fmt.Fprintf(w, "  platform: %s\n", r.Platform)
	fmt.Fprintf(w, "  backend:  %s\n", r.Backend)
	if len(r.Checks) == 0 {
		fmt.Fprintln(w, "  (no checks defined for this platform)")
		return nil
	}
	for _, c := range r.Checks {
		marker := "✓"
		if !c.OK {
			marker = "✗"
		}
		req := "required"
		if !c.Required {
			req = "optional"
		}
		fmt.Fprintf(w, "  %s %s (%s)", marker, c.Name, req)
		if c.OK {
			fmt.Fprintf(w, " — %s\n", c.Path)
		} else {
			fmt.Fprintln(w)
			if c.Hint != "" {
				fmt.Fprintf(w, "      hint: %s\n", c.Hint)
			}
		}
	}
	if r.AllRequiredOK() {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  All required dependencies present.")
	} else {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  Sandbox cannot run on this host without the required deps.")
	}
	return nil
}

func writeList(w io.Writer, prefix string, items []string) {
	if len(items) == 0 {
		fmt.Fprintf(w, "%s(none)\n", prefix)
		return
	}
	for _, it := range items {
		fmt.Fprintf(w, "%s%s\n", prefix, it)
	}
}

// sandboxFooterTag returns the short footer chip shown in the TUI:
//   - "sandbox: off"        — disabled in settings
//   - "sandbox: missing"    — enabled but backend unavailable
//   - "sandbox: on(auto)"   — enabled, auto_allow mode
//   - "sandbox: on(regular)" — enabled, regular permissions mode
func sandboxFooterTag(app *agentapp.AgentApp) string {
	if app == nil {
		return ""
	}
	st := app.SandboxStatus()
	c := st.Resolution.Config
	if !c.Enabled {
		return "sandbox: off"
	}
	if !st.Enabled { // enabled in settings but backend missing
		return "sandbox: missing"
	}
	if c.EffectiveAutoAllowBash() {
		return "sandbox: on(auto)"
	}
	return "sandbox: on(regular)"
}

func displayMode(c config.SandboxConfig) string {
	m := c.EffectiveMode()
	if m == "" {
		return "(disabled)"
	}
	return m
}
