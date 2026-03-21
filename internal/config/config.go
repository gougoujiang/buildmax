// Package config provides configuration loading and defaults.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// LLM holds LLM provider settings (OpenRouter/OpenAI-compatible).
type LLM struct {
	APIKey  string
	BaseURL string
	Model   string
}

// Settings is the root structure for settings.json (e.g. under DataDir).
type Settings struct {
	Models []ModelEntry `json:"models"`
}

// ModelEntry is one LLM model entry in settings (snake_case on disk).
type ModelEntry struct {
	Model  string `json:"model"`
	Name   string `json:"name"`
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
}

// DefaultOpenRouterBaseURL is the OpenRouter OpenAI-compatible API base URL.
const DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// Model names on OpenRouter (switch DefaultModel to use one).
const (
	ModelGPT35Turbo      = "openai/gpt-3.5-turbo" // NOT FREE
	ModelGemma327bItFree = "google/gemma-3-27b-it:free"
	ModelGLM45AirFree    = "z-ai/glm-4.5-air:free"
	ModelGPT4oMini       = "openai/gpt-4o-mini"
)

// DefaultModel is the model used when BUILDMAX_MODEL is not set.
const DefaultModel = ModelGPT4oMini

// DefaultServerPort is the port used when BUILDMAX_SERVER_PORT is not set and --port is 0.
const DefaultServerPort = 5678

// LoadLLM loads LLM config from environment.
// BUILDMAX_API_KEY for API key; BUILDMAX_BASE_URL for base URL (defaults to OpenRouter);
// BUILDMAX_MODEL for model (defaults to openai/gpt-3.5-turbo).
func LoadLLM() LLM {
	apiKey := os.Getenv(EnvKeyBuildmaxAPIKey)
	baseURL := os.Getenv(EnvKeyBuildmaxBaseURL)
	if baseURL == "" {
		baseURL = DefaultOpenRouterBaseURL
	}
	model := os.Getenv(EnvKeyBuildmaxModel)
	if model == "" {
		model = DefaultModel
	}
	return LLM{APIKey: apiKey, BaseURL: baseURL, Model: model}
}

// DataDir returns the application data folder path.
// If BUILDMAX_HOME is set (non-empty), returns filepath.Clean(os.Getenv(EnvKeyBuildmaxHome)).
// Otherwise returns filepath.Join(os.UserHomeDir(), ".buildmax").
// Does not create the directory; callers must create it if needed.
func DataDir() string {
	if dir := os.Getenv(EnvKeyBuildmaxHome); dir != "" {
		return filepath.Clean(dir)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".buildmax")
}

// SessionsDir returns the path to the sessions directory under DataDir.
// Does not create the directory; callers must create it if needed.
func SessionsDir() string {
	return filepath.Join(DataDir(), "sessions")
}

// LogsDir returns the path to the logs directory under DataDir.
// Does not create the directory; callers must create it if needed.
func LogsDir() string {
	return filepath.Join(DataDir(), "logs")
}

// SettingsPath returns the path to the settings file under DataDir.
func SettingsPath() string {
	return filepath.Join(DataDir(), "settings.json")
}

// AuthPath returns the path to the auth credentials file under DataDir.
func AuthPath() string {
	return filepath.Join(DataDir(), "auth.json")
}

// ServerEnv holds server-only env (JWT, CORS). Port is resolved by cmd from flag or BUILDMAX_SERVER_PORT.
type ServerEnv struct {
	JWTSecret  string
	CORSOrigin string
}

// LoadServerEnv loads JWT and CORS from env. Returns an error if JWT_SECRET is empty.
func LoadServerEnv() (ServerEnv, error) {
	jwt := os.Getenv(EnvKeyBuildmaxJWTSecret)
	if jwt == "" {
		return ServerEnv{}, fmt.Errorf("%s is required for server mode", EnvKeyBuildmaxJWTSecret)
	}
	cors := os.Getenv(EnvKeyBuildmaxCorsOrigin)
	if cors == "" {
		cors = "http://localhost:5173"
	}
	return ServerEnv{JWTSecret: jwt, CORSOrigin: cors}, nil
}

// WorkerBinaryPath returns the worker binary name for the scheduler to spawn (BUILDMAX_WORKER_BINARY, default "buildmax-worker").
func WorkerBinaryPath() string {
	if s := os.Getenv(EnvKeyBuildmaxWorkerBinary); s != "" {
		return s
	}
	return "buildmax-worker"
}

// WorkerServerURL returns the server base URL for the worker (BUILDMAX_SERVER_URL). No default; required when running buildmax-worker.
func WorkerServerURL() string {
	return os.Getenv(EnvKeyBuildmaxServerURL)
}

// WorkerToken returns the token for worker-to-server auth (BUILDMAX_WORKER_TOKEN). Required for /api/worker/*.
func WorkerToken() string {
	return os.Getenv(EnvKeyBuildmaxWorkerToken)
}

// WorkerRunMode returns how the scheduler runs workers: "local_process" or "k8s_job" (BUILDMAX_WORKER_RUN_MODE, default local_process).
func WorkerRunMode() string {
	switch os.Getenv(EnvKeyBuildmaxWorkerRunMode) {
	case "k8s_job":
		return "k8s_job"
	default:
		return "local_process"
	}
}

// WorkerJobNamespace returns the Kubernetes namespace for worker Jobs (BUILDMAX_WORKER_JOB_NAMESPACE, default buildmax).
func WorkerJobNamespace() string {
	if s := os.Getenv(EnvKeyBuildmaxWorkerJobNs); s != "" {
		return s
	}
	return "buildmax"
}

// WorkerImage returns the container image for worker Job pods (BUILDMAX_WORKER_IMAGE, default buildmax:local).
func WorkerImage() string {
	if s := os.Getenv(EnvKeyBuildmaxWorkerImage); s != "" {
		return s
	}
	return "buildmax:local"
}

// WebhookMessagePath returns the JSON path for the message in webhook body (BUILDMAX_WEBHOOK_MESSAGE_PATH, default "message").
func WebhookMessagePath() string {
	if s := os.Getenv(EnvKeyBuildmaxWebhookMessagePath); s != "" {
		return s
	}
	return "message"
}

// WebhookUserID returns the user ID used as CreatedBy for webhook-created runs (BUILDMAX_WEBHOOK_USER_ID, default "webhook").
func WebhookUserID() string {
	if s := os.Getenv(EnvKeyBuildmaxWebhookUserID); s != "" {
		return s
	}
	return "webhook"
}

// ResolveServerPort returns the port to use: portFromFlag if > 0, else BUILDMAX_SERVER_PORT, else DefaultServerPort.
// Returns an error if BUILDMAX_SERVER_PORT is set but not a valid positive number.
func ResolveServerPort(portFromFlag int) (int, error) {
	if portFromFlag > 0 {
		return portFromFlag, nil
	}
	env := os.Getenv(EnvKeyBuildmaxServerPort)
	if env == "" {
		return DefaultServerPort, nil
	}
	port, err := strconv.Atoi(env)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", EnvKeyBuildmaxServerPort, env, err)
	}
	if port <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %d", EnvKeyBuildmaxServerPort, port)
	}
	return port, nil
}

// LogLevel returns the value of BUILDMAX_LOG_LEVEL (may be empty; callers treat empty as default level).
func LogLevel() string {
	return os.Getenv(EnvKeyBuildmaxLogLevel)
}

// MySQLDSN returns the MySQL DSN from environment.
func MySQLDSN() string {
	host := os.Getenv(EnvKeyBuildmaxDBHost)
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv(EnvKeyBuildmaxDBPort)
	if port == "" {
		port = "3306"
	}
	user := os.Getenv(EnvKeyBuildmaxDBUser)
	if user == "" {
		user = "buildmax"
	}
	password := os.Getenv(EnvKeyBuildmaxDBPassword)
	if password == "" {
		password = "buildmax"
	}
	database := os.Getenv(EnvKeyBuildmaxDBDatabase)
	if database == "" {
		database = "buildmax"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True", user, password, host, port, database)
}

// WorkspacesDir returns the parent directory of all workspace roots.
// It is only used in server mode. If BUILDMAX_WORKSPACES_DIR is set, returns that path (cleaned).
// If unset, returns empty string (no default). Server startup must check and fail if empty.
// Workspace root for a workspace is filepath.Join(WorkspacesDir(), workspaceID).
func WorkspacesDir() string {
	if dir := os.Getenv(EnvKeyBuildmaxWorkspacesDir); dir != "" {
		return filepath.Clean(dir)
	}
	return ""
}

// PersistentWorkspaceDir returns the directory for a workspace's persistent files (uploads, result files).
// It is WorkspacesDir()/<workspaceID>/home.
func PersistentWorkspaceDir(workspaceID string) string {
	return filepath.Join(WorkspacesDir(), workspaceID, "home")
}

// RuntimeWorkspaceDir returns the ephemeral directory for a task.
// It is WorkspacesDir()/<workspaceID>/tasks/<taskID>. Runtime dirs are left on disk; cleanup is a separate task.
func RuntimeWorkspaceDir(workspaceID, chatID string) string {
	return filepath.Join(WorkspacesDir(), workspaceID, "tasks", chatID)
}

// RuntimeChatBuildmaxDir returns the buildmax subdir for a task (agent data: sessions, logs, settings).
// It is RuntimeWorkspaceDir(workspaceID, chatID)/buildmax. For run-scoped dir use RuntimeTaskRunBuildmaxDir.
func RuntimeChatBuildmaxDir(workspaceID, chatID string) string {
	return filepath.Join(RuntimeWorkspaceDir(workspaceID, chatID), "buildmax")
}

// RuntimeTaskRunBuildmaxDir returns the buildmax subdir for a specific run: .../tasks/<taskID>/<taskRunID>/buildmax.
func RuntimeTaskRunBuildmaxDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(WorkspacesDir(), workspaceID, "tasks", chatID, chatRunID, "buildmax")
}

// RuntimeTaskRunDir returns the run directory: .../tasks/<taskID>/<taskRunID>.
func RuntimeTaskRunDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(WorkspacesDir(), workspaceID, "tasks", chatID, chatRunID)
}

// RuntimeTaskRunHomeDir returns the run's home dir (materialized workspace home): .../tasks/<taskID>/<taskRunID>/home.
func RuntimeTaskRunHomeDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(RuntimeTaskRunDir(workspaceID, chatID, chatRunID), "home")
}

// RuntimeTaskRunArtifactsDir returns the run's artifacts dir (run outputs): .../tasks/<taskID>/<taskRunID>/artifacts.
func RuntimeTaskRunArtifactsDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(RuntimeTaskRunDir(workspaceID, chatID, chatRunID), "artifacts")
}

// RuntimeTaskRunGlobalDir returns the run's global dir (BUILDMAX_HOME): .../tasks/<taskID>/<taskRunID>/global.
func RuntimeTaskRunGlobalDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(RuntimeTaskRunDir(workspaceID, chatID, chatRunID), "global")
}

// RuntimeChatWSDir returns the workspace subdir for a task run (materialized persist files).
// It is RuntimeWorkspaceDir(workspaceID, chatID)/ws. Deprecated: use RuntimeTaskRunHomeDir for run-scoped materialize.
func RuntimeChatWSDir(workspaceID, chatID string) string {
	return filepath.Join(RuntimeWorkspaceDir(workspaceID, chatID), "ws")
}

// RunOutputDir returns the directory for a task run's output files (one artifact per run).
// It is WorkspacesDir()/<workspaceID>/artifacts/<chatID>/<chatRunID>.
func RunOutputDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(WorkspacesDir(), workspaceID, "artifacts", chatID, chatRunID)
}

// LoadSettings reads the settings file at path. If path is empty, SettingsPath() is used.
// On missing file (default path only): creates DataDir and the file with content "{}", then returns (Settings{}, nil).
// Returns an error for invalid JSON, unreadable files, or non-default missing paths.
func LoadSettings(path string) (Settings, error) {
	isDefault := path == ""
	if isDefault {
		path = SettingsPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && isDefault {
			// Auto-create default settings file.
			_ = os.MkdirAll(DataDir(), 0755)
			_ = os.WriteFile(path, []byte("{}\n"), 0644)
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("parse settings: %w", err)
	}
	return s, nil
}

// EffectiveLLMWithSelector returns the LLM config and display name to use at startup,
// optionally selecting a model by id or name. settingsPath is passed to LoadSettings
// (empty means default SettingsPath()). When modelSelector is empty, behaviour equals
// EffectiveLLM: first model from settings or LoadLLM() fallback; no error. When
// modelSelector is non-empty, settings must have at least one model and an entry
// whose model or name equals modelSelector (first match wins); otherwise an error
// is returned.
func EffectiveLLMWithSelector(settingsPath string, modelSelector string) (LLM, string, error) {
	s, err := LoadSettings(settingsPath)
	if err != nil {
		return LLM{}, "", fmt.Errorf("load settings: %w", err)
	}
	if modelSelector == "" {
		if len(s.Models) == 0 {
			cfg := LoadLLM()
			return cfg, cfg.Model, nil
		}
		m := s.Models[0]
		displayName := m.Name
		if displayName == "" {
			displayName = m.Model
		}
		return LLM{
			APIKey:  m.APIKey,
			BaseURL: m.APIURL,
			Model:   m.Model,
		}, displayName, nil
	}
	if len(s.Models) == 0 {
		return LLM{}, "", errors.New("no models in settings; add settings or omit --model")
	}
	for _, m := range s.Models {
		if m.Model == modelSelector || m.Name == modelSelector {
			displayName := m.Name
			if displayName == "" {
				displayName = m.Model
			}
			return LLM{
				APIKey:  m.APIKey,
				BaseURL: m.APIURL,
				Model:   m.Model,
			}, displayName, nil
		}
	}
	return LLM{}, "", fmt.Errorf("model not found: %q", modelSelector)
}

// EffectiveLLM returns the LLM config and display name to use at startup.
// If path is empty, the default settings path is used. When the settings file
// has at least one model, the first entry is used; otherwise LoadLLM() is used.
// Display name is the entry's name if set, else the model id.
func EffectiveLLM(path string) (LLM, string) {
	cfg, name, _ := EffectiveLLMWithSelector(path, "")
	return cfg, name
}

// SkillSearchPaths returns the ordered list of directories to scan for skills.
// Priority (first wins on name conflict):
//  1. <workspace>/.buildmax/skills  (project-level)
//  2. <DataDir>/skills              (global-level, e.g. ~/.buildmax/skills)
//
// Missing directories are not created; callers handle absent dirs gracefully.
func SkillSearchPaths(workspace string) []string {
	return []string{
		filepath.Join(workspace, ".buildmax", "skills"),
		filepath.Join(DataDir(), "skills"),
	}
}

// AgentDefsSearchPaths returns the ordered list of directories to scan for agent definitions.
// Priority (first wins on name conflict):
//  1. <workspace>/.buildmax/agents  (project-level)
//  2. <DataDir>/agents              (global-level, e.g. ~/.buildmax/agents)
//
// Missing directories are not created; callers handle absent dirs gracefully.
func AgentDefsSearchPaths(workspace string) []string {
	return []string{
		filepath.Join(workspace, ".buildmax", "agents"),
		filepath.Join(DataDir(), "agents"),
	}
}
