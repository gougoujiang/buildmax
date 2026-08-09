package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// ServerConfig and nested types
// ---------------------------------------------------------------------------

// ServerConfig is the root of BUILDMAX_HOME/server.yaml.
type ServerConfig struct {
	LogLevel         string              `mapstructure:"log_level"`
	Port             int                 `mapstructure:"port"`
	JWTSecret        string              `mapstructure:"jwt_secret"`
	DevLoginOTP      string              `mapstructure:"dev_login_otp"`
	CORSOrigin       string              `mapstructure:"cors_origin"`
	WorkspacesDir    string              `mapstructure:"workspaces_dir"`
	DefaultQuotaTier string              `mapstructure:"default_quota_tier"`
	Conversation     ServerConvConfig    `mapstructure:"conversation"`
	Database         ServerDBConfig      `mapstructure:"database"`
	Webhook          ServerWebhookConfig `mapstructure:"webhook"`
	Worker           ServerWorkerConfig  `mapstructure:"worker"`
	Storage          ServerStorageConfig `mapstructure:"storage"`
}

// ServerConvConfig holds Tier 1 conversation LLM settings.
type ServerConvConfig struct {
	Model ServerModelEntry `mapstructure:"model"`
}

// ServerModelEntry is the LLM model used for Tier 1 conversation.
type ServerModelEntry struct {
	Model         string `mapstructure:"model"`
	Name          string `mapstructure:"name"`
	APIURL        string `mapstructure:"api_url"`
	APIKey        string `mapstructure:"api_key"`
	ContextWindow int    `mapstructure:"context_window"`
	CallTimeout   int    `mapstructure:"call_timeout"` // seconds; 0 = uses DefaultCallTimeoutSecs
}

// ServerDBConfig holds MySQL connection settings.
type ServerDBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
}

// DSN builds a MySQL DSN from the database config.
func (d ServerDBConfig) DSN() string {
	host := d.Host
	if host == "" {
		host = "localhost"
	}
	port := d.Port
	if port == 0 {
		port = 3306
	}
	user := d.User
	if user == "" {
		user = "buildmax"
	}
	password := d.Password
	name := d.Name
	if name == "" {
		name = "buildmax"
	}
	return (&serverDBFormatter{host: host, port: port, user: user, password: password, name: name}).dsn()
}

type serverDBFormatter struct {
	host, user, password, name string
	port                       int
}

func (f *serverDBFormatter) dsn() string {
	return f.user + ":" + f.password + "@tcp(" + f.host + ":" + itoa(f.port) + ")/" + f.name + "?charset=utf8mb4&parseTime=True"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n >= 10 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	pos--
	buf[pos] = byte('0' + n)
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// ServerWebhookConfig holds webhook handler options.
type ServerWebhookConfig struct {
	MessagePath string `mapstructure:"message_path"`
	UserID      string `mapstructure:"user_id"`
}

// ServerWorkerConfig holds worker launch and connection options.
type ServerWorkerConfig struct {
	Binary    string          `mapstructure:"binary"`
	RunMode   string          `mapstructure:"run_mode"`
	Token     string          `mapstructure:"token"`
	ServerURL string          `mapstructure:"server_url"`
	K8s       ServerK8sConfig `mapstructure:"k8s"`
}

// ServerK8sConfig holds Kubernetes worker job settings.
type ServerK8sConfig struct {
	Namespace string `mapstructure:"namespace"`
	Image     string `mapstructure:"image"`
}

// ServerStorageConfig holds blob storage backend selection and MinIO settings.
type ServerStorageConfig struct {
	PersistBackend  string            `mapstructure:"persist_backend"`
	ArtifactBackend string            `mapstructure:"artifact_backend"`
	MinIO           ServerMinIOConfig `mapstructure:"minio"`
}

// ServerMinIOConfig holds MinIO/S3 connection settings.
type ServerMinIOConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	Region    string `mapstructure:"region"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
	Prefix    string `mapstructure:"prefix"`
}

// ---------------------------------------------------------------------------
// Loader
// ---------------------------------------------------------------------------

// ServerConfigPath returns the path to the server config file (BUILDMAX_HOME/server.yaml).
func ServerConfigPath() string {
	return filepath.Join(DataDir(), "server.yaml")
}

// LoadServerConfig reads BUILDMAX_HOME/server.yaml via Viper and applies defaults.
// BUILDMAX_JWT_SECRET env var overrides the jwt_secret field (for production deploy).
// A missing file is not an error — returns a config with all defaults applied.
func LoadServerConfig() (ServerConfig, error) {
	v := viper.New()
	v.SetConfigFile(ServerConfigPath())

	// Defaults
	v.SetDefault("log_level", "info")
	v.SetDefault("port", 5678)
	v.SetDefault("cors_origin", "http://localhost:5173")
	v.SetDefault("default_quota_tier", "free_trial")
	v.SetDefault("webhook.message_path", "message")
	v.SetDefault("webhook.user_id", "webhook")
	v.SetDefault("worker.binary", "buildmax-worker")
	v.SetDefault("worker.run_mode", "local_process")
	v.SetDefault("worker.k8s.namespace", "buildmax")
	v.SetDefault("worker.k8s.image", "buildmax:local")
	v.SetDefault("storage.persist_backend", ProviderLocalFS)
	v.SetDefault("storage.artifact_backend", ProviderLocalFS)
	v.SetDefault("storage.minio.endpoint", "http://localhost:9000")
	v.SetDefault("storage.minio.region", "us-east-1")
	v.SetDefault("storage.minio.access_key", "minio")
	v.SetDefault("storage.minio.secret_key", "minio123")
	v.SetDefault("storage.minio.bucket", "bmstore")
	v.SetDefault("storage.minio.prefix", "workspaces")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 3306)
	v.SetDefault("database.user", "buildmax")
	v.SetDefault("database.password", "buildmax")
	v.SetDefault("database.name", "buildmax")
	v.SetDefault("conversation.model.context_window", 0)

	// BUILDMAX_JWT_SECRET env var overrides jwt_secret in the file.
	v.SetEnvPrefix("BUILDMAX")
	_ = v.BindEnv("jwt_secret", EnvKeyBuildmaxJWTSecret)
	_ = v.BindEnv("dev_login_otp", EnvKeyBuildmaxDevLoginOTP)

	if err := v.ReadInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ServerConfig{}, err
	}

	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return ServerConfig{}, err
	}
	return cfg, nil
}
