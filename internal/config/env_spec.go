package config

// This file is the single source of truth for env var names, defaults, and descriptions.
// Use the EnvKey* constants everywhere instead of string literals. Keep .env.example in sync.

// Environment variable key names (use these instead of string literals).
const (
	// LLM / API
	EnvKeyBuildmaxAPIKey  = "BUILDMAX_API_KEY"
	EnvKeyBuildmaxBaseURL = "BUILDMAX_BASE_URL"
	EnvKeyBuildmaxModel   = "BUILDMAX_MODEL"
	// App data
	EnvKeyBuildmaxHome          = "BUILDMAX_HOME"
	EnvKeyBuildmaxMCPConfig     = "BUILDMAX_MCP_CONFIG"
	// MCP mcp.json $VAR expansion: replaced by LoadMCPConfigForWorkspace(workspaceDir), not os.Getenv.
	EnvKeyBuildmaxWorkspaceRoot = "BUILDMAX_WORKSPACE_ROOT"
	EnvKeyBuildmaxWorkspacesDir = "BUILDMAX_WORKSPACES_DIR"
	// Logging
	EnvKeyBuildmaxLogLevel = "BUILDMAX_LOG_LEVEL"
	// HTTP server
	EnvKeyBuildmaxServerPort = "BUILDMAX_SERVER_PORT"
	EnvKeyBuildmaxJWTSecret  = "BUILDMAX_JWT_SECRET"
	EnvKeyBuildmaxCorsOrigin = "BUILDMAX_CORS_ORIGIN"
	// Worker (buildmax-worker binary and worker-to-server auth)
	EnvKeyBuildmaxWorkerBinary  = "BUILDMAX_WORKER_BINARY"
	EnvKeyBuildmaxServerURL     = "BUILDMAX_SERVER_URL"
	EnvKeyBuildmaxWorkerToken   = "BUILDMAX_WORKER_TOKEN"
	EnvKeyBuildmaxWorkerRunMode = "BUILDMAX_WORKER_RUN_MODE"
	EnvKeyBuildmaxWorkerJobNs   = "BUILDMAX_WORKER_JOB_NAMESPACE"
	EnvKeyBuildmaxWorkerImage   = "BUILDMAX_WORKER_IMAGE"
	// Database (MySQL)
	EnvKeyBuildmaxDBHost     = "BUILDMAX_DB_HOST"
	EnvKeyBuildmaxDBPort     = "BUILDMAX_DB_PORT"
	EnvKeyBuildmaxDBUser     = "BUILDMAX_DB_USER"
	EnvKeyBuildmaxDBPassword = "BUILDMAX_DB_PASSWORD"
	EnvKeyBuildmaxDBDatabase = "BUILDMAX_DB_DATABASE"
	// Workspace storage
	EnvKeyBuildmaxPersistStorage  = "BUILDMAX_PERSIST_STORAGE"
	EnvKeyBuildmaxArtifactStorage = "BUILDMAX_ARTIFACT_STORAGE"
	EnvKeyBuildmaxMinioEndpoint   = "BUILDMAX_MINIO_ENDPOINT"
	EnvKeyBuildmaxMinioRegion     = "BUILDMAX_MINIO_REGION"
	EnvKeyBuildmaxMinioAccessKey  = "BUILDMAX_MINIO_ACCESS_KEY"
	EnvKeyBuildmaxMinioSecretKey  = "BUILDMAX_MINIO_SECRET_KEY"
	EnvKeyBuildmaxMinioBucket     = "BUILDMAX_MINIO_BUCKET"
	EnvKeyBuildmaxMinioPrefix     = "BUILDMAX_MINIO_PREFIX"
	// Quota (per-user tier and limits; limits in DB, default tier name from env)
	EnvKeyBuildmaxDefaultQuotaTier = "BUILDMAX_DEFAULT_QUOTA_TIER"
	// Webhook (per-workspace API keys; message path and optional user ID for runs)
	EnvKeyBuildmaxWebhookMessagePath = "BUILDMAX_WEBHOOK_MESSAGE_PATH"
	EnvKeyBuildmaxWebhookUserID      = "BUILDMAX_WEBHOOK_USER_ID"
	// Optional / scripts
	EnvKeyBuildmaxKindCluster = "BUILDMAX_KIND_CLUSTER"
	// Test only
	EnvKeyBuildmaxTestDSN = "BUILDMAX_TEST_DSN"
)

// EnvVar describes one environment variable used by BuildMax.
type EnvVar struct {
	Name        string // use EnvKey* const
	Default     string // empty = no default or required
	Description string
}

// EnvVars lists all environment variables in use. Grouped by domain for documentation.
var EnvVars = []EnvVar{
	// LLM / API
	{EnvKeyBuildmaxAPIKey, "", "API key for OpenAI-compatible / OpenRouter API"},
	{EnvKeyBuildmaxBaseURL, "https://openrouter.ai/api/v1", "LLM API base URL"},
	{EnvKeyBuildmaxModel, DefaultModel, "LLM model name"},
	// App data
	{EnvKeyBuildmaxHome, "~/.buildmax", "Application data directory"},
	{EnvKeyBuildmaxMCPConfig, "", "Optional path to mcp.json; if set and the file exists, only that file is used. Otherwise global DataDir/mcp.json and workspace .buildmax/mcp.json are merged (workspace wins on same server id); CLI MCP only"},
	{EnvKeyBuildmaxWorkspaceRoot, "", "Name only for mcp.json $ expansion: expands to the workspace directory passed to LoadMCPConfigForWorkspace (not read from the process environment for expansion); CLI MCP only"},
	{EnvKeyBuildmaxWorkspacesDir, "", "Parent of workspace roots (required for server mode; no default)"},
	// Logging
	{EnvKeyBuildmaxLogLevel, "info", "Log level: debug, info, warn, error, off"},
	// HTTP server
	{EnvKeyBuildmaxServerPort, "5678", "Port for buildmax server"},
	{EnvKeyBuildmaxJWTSecret, "", "JWT signing secret (required for server)"},
	{EnvKeyBuildmaxCorsOrigin, "http://localhost:5173", "CORS allowed origin for portal"},
	// Worker
	{EnvKeyBuildmaxWorkerBinary, "buildmax-worker", "Worker binary name for scheduler to spawn (default: buildmax-worker)"},
	{EnvKeyBuildmaxServerURL, "", "Server base URL for worker (required when running buildmax-worker; e.g. http://localhost:5678)"},
	{EnvKeyBuildmaxWorkerToken, "", "Token for worker-to-server auth (required for /api/worker/*)"},
	{EnvKeyBuildmaxWorkerRunMode, "local_process", "How to run worker: local_process (spawn binary) or k8s_job (create Kubernetes Job per task)"},
	{EnvKeyBuildmaxWorkerJobNs, "buildmax", "Kubernetes namespace for worker Jobs when BUILDMAX_WORKER_RUN_MODE=k8s_job"},
	{EnvKeyBuildmaxWorkerImage, "buildmax:local", "Container image for worker Job pod when BUILDMAX_WORKER_RUN_MODE=k8s_job"},
	// Database (MySQL)
	{EnvKeyBuildmaxDBHost, "localhost", "MySQL host"},
	{EnvKeyBuildmaxDBPort, "3306", "MySQL port"},
	{EnvKeyBuildmaxDBUser, "buildmax", "MySQL user"},
	{EnvKeyBuildmaxDBPassword, "buildmax", "MySQL password"},
	{EnvKeyBuildmaxDBDatabase, "buildmax", "MySQL database name"},
	// Workspace storage
	{EnvKeyBuildmaxPersistStorage, "local_fs", "Persist backend: local_fs or minio"},
	{EnvKeyBuildmaxArtifactStorage, "local_fs", "Artifact backend: local_fs or minio"},
	{EnvKeyBuildmaxMinioEndpoint, "http://localhost:9000", "MinIO/S3 endpoint"},
	{EnvKeyBuildmaxMinioRegion, "us-east-1", "MinIO/S3 region"},
	{EnvKeyBuildmaxMinioAccessKey, "minio", "MinIO/S3 access key"},
	{EnvKeyBuildmaxMinioSecretKey, "minio123", "MinIO/S3 secret key"},
	{EnvKeyBuildmaxMinioBucket, "bmstore", "MinIO/S3 bucket"},
	{EnvKeyBuildmaxMinioPrefix, "workspaces", "MinIO/S3 key prefix"},
	// Quota
	{EnvKeyBuildmaxDefaultQuotaTier, "free_trial", "Default quota tier name for new users (tier limits in quota_tier table)"},
	// Webhook
	{EnvKeyBuildmaxWebhookMessagePath, "message", "JSON path for message in webhook body (e.g. message, body.text)"},
	{EnvKeyBuildmaxWebhookUserID, "webhook", "User ID used as CreatedBy for webhook-created task runs"},
	// Optional / scripts
	{EnvKeyBuildmaxKindCluster, "buildmaxdev", "Kind cluster name in setup scripts"},
	// Test only
	{EnvKeyBuildmaxTestDSN, "", "MySQL DSN for store integration tests; unset skips"},
}
