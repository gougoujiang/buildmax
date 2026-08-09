package config

// This file is the single source of truth for environment variable names and the few
// functions that read them. Most configuration now lives in settings.yaml (local) or
// server.yaml (server/worker). Only bootstrap-level values that cannot be in a file
// remain as environment variables.

// Environment variable key names.
const (
	// BUILDMAX_HOME — path to the data directory; tells every binary where to find its
	// config files (settings.yaml, server.yaml). Must remain an env var because the
	// config files cannot be located until this path is known.
	EnvKeyBuildmaxHome = "BUILDMAX_HOME"

	// BUILDMAX_JWT_SECRET — optional override for jwt_secret in server.yaml.
	// In production, inject via env var (Kubernetes Secret, Docker secret) instead of
	// storing the value in the YAML file on disk.
	EnvKeyBuildmaxJWTSecret = "BUILDMAX_JWT_SECRET"

	// BUILDMAX_DEV_LOGIN_OTP — optional override for dev_login_otp in server.yaml.
	// When set, POST /api/login accepts this single fixed code as the one-time
	// password for every account. This is a development convenience that stands in
	// for a real OTP delivery channel; it is a full authentication bypass for anyone
	// who knows a registered email address. Leave it unset in any deployment that
	// is reachable by untrusted users.
	EnvKeyBuildmaxDevLoginOTP = "BUILDMAX_DEV_LOGIN_OTP"

	// Test only.
	EnvKeyBuildmaxTestDSN = "BUILDMAX_TEST_DSN"
)

// Note: EnvKeyBuildmaxSandboxEnabled lives in sandbox.go alongside the
// resolver that reads it (per-subsystem env constant).

// EnvVar describes one environment variable used by BuildMax.
type EnvVar struct {
	Name        string
	Default     string
	Description string
}

// EnvVars lists every environment variable read by BuildMax binaries.
var EnvVars = []EnvVar{
	{EnvKeyBuildmaxHome, "~/.buildmax", "Application data directory; locates settings.yaml and server.yaml"},
	{EnvKeyBuildmaxJWTSecret, "", "Override for jwt_secret in server.yaml; inject at deploy time in production"},
	{EnvKeyBuildmaxDevLoginOTP, "", "Development-only fixed login OTP; leave unset except on trusted local deployments"},
	{EnvKeyBuildmaxTestDSN, "", "MySQL DSN for store integration tests; unset skips those tests"},
	{EnvKeyBuildmaxSandboxEnabled, "", "Override sandbox.enabled in settings; values: 1/true/yes/on or 0/false/no/off"},
	{EnvKeyBuildmaxTraceDisabled, "", "Disable durable run traces when truthy (1/true/yes/on); traces are on by default"},
}
