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
	EnvKeyBuildmaxWorkspacesDir = "BUILDMAX_WORKSPACES_DIR"
	// Logging
	EnvKeyBuildmaxLogLevel = "BUILDMAX_LOG_LEVEL"
	// HTTP server
	EnvKeyBuildmaxServerPort = "BUILDMAX_SERVER_PORT"
	EnvKeyBuildmaxJWTSecret  = "BUILDMAX_JWT_SECRET"
	EnvKeyBuildmaxCorsOrigin = "BUILDMAX_CORS_ORIGIN"
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
	{EnvKeyBuildmaxWorkspacesDir, "", "Parent of workspace roots (default: DataDir()/workspaces)"},
	// Logging
	{EnvKeyBuildmaxLogLevel, "info", "Log level: debug, info, warn, error, off"},
	// HTTP server
	{EnvKeyBuildmaxServerPort, "5678", "Port for buildmax server"},
	{EnvKeyBuildmaxJWTSecret, "", "JWT signing secret (required for server)"},
	{EnvKeyBuildmaxCorsOrigin, "http://localhost:5173", "CORS allowed origin for portal"},
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
	// Optional / scripts
	{EnvKeyBuildmaxKindCluster, "buildmaxdev", "Kind cluster name in setup scripts"},
	// Test only
	{EnvKeyBuildmaxTestDSN, "", "MySQL DSN for store integration tests; unset skips"},
}
