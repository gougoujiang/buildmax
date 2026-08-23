package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// ServerConfig and nested types
// ---------------------------------------------------------------------------

// ServerConfig is the root of BUILDMAX_HOME/server.yaml.
type ServerConfig struct {
	LogLevel  string `mapstructure:"log_level"`
	Port      int    `mapstructure:"port"`
	JWTSecret string `mapstructure:"jwt_secret"`
	// AccessTokenTTL is how long a signed access token stays valid. It is not
	// stored anywhere, so this is also the window in which a leaked one still
	// works — shortening it costs nothing but refresh traffic.
	AccessTokenTTL time.Duration `mapstructure:"access_token_ttl"`
	// RefreshTokenTTL is how long a login can be renewed without a new login
	// code. Every rotation restarts it, so an active session lives on and an
	// abandoned one expires.
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`
	// RefreshRotationGrace is how long a just-rotated refresh token may be
	// exchanged again before the server treats it as reuse and revokes the
	// session. It absorbs concurrent refreshes from processes sharing one
	// credentials file; it is not a security setting to raise casually.
	RefreshRotationGrace time.Duration `mapstructure:"refresh_rotation_grace"`
	// AllowSignup opens POST /api/otp/request to self-registration. It defaults
	// to false, and the zero value is the safe one on purpose: a server that
	// forgets to configure this is closed, not open.
	AllowSignup      bool                `mapstructure:"allow_signup"`
	CORSOrigin       string              `mapstructure:"cors_origin"`
	WorkspacesDir    string              `mapstructure:"workspaces_dir"`
	DefaultQuotaTier string              `mapstructure:"default_quota_tier"`
	Conversation     ServerConvConfig    `mapstructure:"conversation"`
	LLM              ServerLLMConfig     `mapstructure:"llm"`
	Database         ServerDBConfig      `mapstructure:"database"`
	Webhook          ServerWebhookConfig `mapstructure:"webhook"`
	Worker           ServerWorkerConfig  `mapstructure:"worker"`
	Storage          ServerStorageConfig `mapstructure:"storage"`
	Audit            ServerAuditConfig   `mapstructure:"audit"`
}

// ServerAuditConfig decides how long the governance trail is kept.
type ServerAuditConfig struct {
	// RetentionDays expires audit events older than the window. Zero, the
	// default, keeps them forever.
	//
	// Keeping is the default because the trail is evidence, and a deployment
	// that never chose a retention policy has not decided to discard anything.
	// Setting this is a deliberate act with a cost, which is why the sweep
	// records what it removed: a trail that begins partway through then says
	// that policy shortened it, rather than leaving a reader to guess between
	// policy and loss.
	RetentionDays int `mapstructure:"retention_days"`
}

// ServerConvConfig holds Tier 1 conversation LLM settings.
type ServerConvConfig struct {
	Model ServerModelEntry `mapstructure:"model"`
	// ModelTarget names a catalog model (an llm_model row) to use for Tier 1
	// inference instead of the model above. It is a catalog ID, not a team
	// alias: the server picks its own model rather than being granted one.
	ModelTarget string `mapstructure:"model_target"`
}

// ServerModelEntry is the LLM model used for Tier 1 conversation.
type ServerModelEntry struct {
	Model         string `mapstructure:"model"`
	Name          string `mapstructure:"name"`
	APIURL        string `mapstructure:"api_url"`
	APIKey        string `mapstructure:"api_key"`
	ContextWindow int    `mapstructure:"context_window"`
	CallTimeout   int    `mapstructure:"call_timeout"` // seconds; 0 = uses DefaultCallTimeoutSecs
	MaxTokens     int    `mapstructure:"max_tokens"`   // 0 = the adapter's own default
	// Provider is the wire protocol this model speaks. Empty means
	// LLMProviderOpenAICompatible.
	Provider string `mapstructure:"provider"`
	// Reasoning is the effort level: off (the default), low, medium, or high.
	Reasoning string `mapstructure:"reasoning"`
	// PromptCache is the deprecated shorthand for CacheControl; absent and
	// false ask for different things, hence the pointer.
	PromptCache *bool `mapstructure:"prompt_cache"`
	// CacheControl is this model's prompt-cache policy.
	CacheControl *CacheControl `mapstructure:"cache_control"`
	// Pricing is what this model charges; without it cost is unavailable.
	Pricing *ModelPricing `mapstructure:"pricing"`
	// Vision says this model accepts image input.
	Vision bool `mapstructure:"vision"`
}

// RuntimeModelEntry converts the server's resolved model configuration into
// the model shape used by the shared agent runtime. Environment overrides have
// already been applied before this conversion, so credentials stay in memory.
//
// The result is always a direct entry: a worker runs the server's own model
// with the server's own credential, and does not call the gateway. Worker
// adoption of managed inference is separate work — see
// docs/design/llm-gateway.md section 15.
func (m ServerModelEntry) RuntimeModelEntry() ModelEntry {
	return ModelEntry{
		Model:         m.Model,
		Name:          m.Name,
		APIURL:        m.APIURL,
		APIKey:        m.APIKey,
		ContextWindow: m.ContextWindow,
		CallTimeout:   m.CallTimeout,
		MaxTokens:     m.MaxTokens,
		Provider:      m.Provider,
		Reasoning:     m.Reasoning,
		PromptCache:   m.PromptCache,
		CacheControl:  m.CacheControl,
		Pricing:       m.Pricing,
		Vision:        m.Vision,
		Transport:     TransportDirect,
	}
}

// ServerLLMConfig is the deployment-wide team model policy for the managed LLM
// gateway. The catalog itself lives in the llm_model table, edited with
// `buildmax-server model`, because it changes while the server runs.
//
// It is optional. A server with no llm section still serves Tier 1
// conversations from conversation.model, and grants no team any managed model.
type ServerLLMConfig struct {
	// DefaultAlias is the alias used when a managed caller names none.
	DefaultAlias string `mapstructure:"default_alias"`
	// Aliases maps a stable alias such as "fast" to a target id in Targets.
	// Leaving it empty means no team may call the gateway.
	Aliases map[string]string `mapstructure:"aliases"`
}

// ServerDBConfig holds MySQL connection settings.
type ServerDBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	// TLS is the go-sql-driver tls parameter. Empty means DefaultDBTLSMode.
	//
	// A deployment pointing at a database it already runs — RDS, Aurora, Cloud
	// SQL — is the case this exists for: most managed MySQL either requires TLS
	// or is reached over a network where the credentials should not travel in
	// the clear.
	//
	// Values: "preferred" (TLS when the server offers it, certificate not
	// verified), "true" (require TLS and verify the certificate against the
	// system roots), "skip-verify" (require TLS, accept any certificate), or
	// "false" (never).
	TLS string `mapstructure:"tls"`
}

// DefaultDBTLSMode is used when database.tls is unset.
//
// "preferred" upgrades a connection whenever the server advertises TLS and
// behaves exactly as before against one that does not, so it has no failure
// mode a plaintext connection did not already have. It does not verify the
// certificate; a deployment that needs that sets "true".
const DefaultDBTLSMode = "preferred"

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
	tlsMode := d.TLS
	if tlsMode == "" {
		tlsMode = DefaultDBTLSMode
	}
	return (&serverDBFormatter{host: host, port: port, user: user, password: password, name: name, tls: tlsMode}).dsn()
}

type serverDBFormatter struct {
	host, user, password, name, tls string
	port                            int
}

func (f *serverDBFormatter) dsn() string {
	dsn := f.user + ":" + f.password + "@tcp(" + f.host + ":" + itoa(f.port) + ")/" + f.name + "?charset=utf8mb4&parseTime=True"
	if f.tls != "" {
		dsn += "&tls=" + f.tls
	}
	return dsn
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
	Binary    string                `mapstructure:"binary"`
	RunMode   string                `mapstructure:"run_mode"`
	Token     string                `mapstructure:"token"`
	ServerURL string                `mapstructure:"server_url"`
	LLM       ServerWorkerLLMConfig `mapstructure:"llm"`
	K8s       ServerK8sConfig       `mapstructure:"k8s"`
	// RunTokenTTL bounds a run token. Zero uses authtoken's default. It has to
	// outlast the longest run: the token is not renewable, so a run that outlives
	// it can no longer report anything, including its own result.
	RunTokenTTL time.Duration `mapstructure:"run_token_ttl"`
	// RunTimeout is how long a run may stay SCHEDULED or RUNNING before the
	// server records it as abandoned. Zero uses the scheduler's default.
	//
	// Only the worker moves a run out of those states, so without this a run
	// whose worker died stays there forever. Keep it at or below RunTokenTTL: a
	// run that outlived its credential cannot report an outcome, so nothing else
	// will ever close it.
	RunTimeout time.Duration `mapstructure:"run_timeout"`
}

// ServerWorkerLLMConfig decides how a task run reaches a model.
//
// The choice is the operator's and lives on the server, not in the worker's
// hands: a worker executes model-chosen code, so it is told which transport and
// alias to use rather than selecting them.
type ServerWorkerLLMConfig struct {
	// Transport is TransportDirect or TransportBuildMax. Empty means direct,
	// which is what every existing deployment gets.
	Transport string `mapstructure:"transport"`
	// Alias is the team model alias a managed run calls. Empty uses the team's
	// default alias.
	Alias string `mapstructure:"alias"`
	// ContextWindow and CallTimeout describe the alias to the run. The protocol
	// does not report them per call, so they come from configuration or stay
	// unset.
	ContextWindow int `mapstructure:"context_window"`
	CallTimeout   int `mapstructure:"call_timeout"`
}

// Managed reports whether task runs call the gateway instead of a provider.
func (c ServerWorkerLLMConfig) Managed() bool { return c.Transport == TransportBuildMax }

// ServerK8sConfig holds Kubernetes worker job settings.
type ServerK8sConfig struct {
	Namespace string `mapstructure:"namespace"`
	Image     string `mapstructure:"image"`
	// ConfigMap names a ConfigMap holding a server.yaml key. It is mounted into
	// every worker pod so the worker reads the same configuration the server does.
	// Empty means no config file is mounted and the worker relies on inherited
	// environment variables alone, which is rarely enough.
	ConfigMap string `mapstructure:"config_map"`
	// HomeDir is BUILDMAX_HOME inside a worker pod; server.yaml is mounted there.
	HomeDir string `mapstructure:"home_dir"`
	// RunAsUser is the uid a worker pod runs as. Zero uses BuildMax's default.
	// Set it on clusters that assign their own uid ranges, OpenShift most
	// commonly. The worker never needs a uid the image knows about — it writes
	// only into mounted volumes.
	RunAsUser int64 `mapstructure:"run_as_user"`
	// Resources bounds a worker pod. Empty leaves the matching request or limit
	// unset, so an existing deployment keeps running unbounded until an
	// operator chooses values.
	Resources ServerK8sResources `mapstructure:"resources"`
}

// ServerK8sResources holds Kubernetes quantity strings for a worker pod.
type ServerK8sResources struct {
	CPURequest    string `mapstructure:"cpu_request"`
	CPULimit      string `mapstructure:"cpu_limit"`
	MemoryRequest string `mapstructure:"memory_request"`
	MemoryLimit   string `mapstructure:"memory_limit"`
}

// ServerStorageConfig holds blob storage backend selection and MinIO settings.
type ServerStorageConfig struct {
	PersistBackend  string            `mapstructure:"persist_backend"`
	ArtifactBackend string            `mapstructure:"artifact_backend"`
	MinIO           ServerMinIOConfig `mapstructure:"minio"`
	// MaxArtifactMB caps one artifact upload. Zero uses the built-in default.
	//
	// It is a per-file limit and deliberately not a team storage allowance: a
	// stock of bytes held is a different measurement from the rates the quota
	// model records, and it waits for its own decision. What this already
	// settles is that one request cannot cost the deployment unbounded disk.
	MaxArtifactMB int `mapstructure:"max_artifact_mb"`
}

// ServerMinIOConfig holds MinIO/S3 connection settings.
type ServerMinIOConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	Region    string `mapstructure:"region"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
	Prefix    string `mapstructure:"prefix"`
	// PathStyle forces bucket-in-path addressing. Unset derives it from
	// endpoint: set means an S3-compatible store such as MinIO, which needs
	// path style; empty means real AWS S3, which does not. Set it explicitly
	// for a compatible store that uses virtual-host addressing.
	PathStyle *bool `mapstructure:"path_style"`
}

// ---------------------------------------------------------------------------
// Loader
// ---------------------------------------------------------------------------

// ServerConfigPath returns the path to the server config file (BUILDMAX_HOME/server.yaml).
func ServerConfigPath() string {
	return filepath.Join(DataDir(), "server.yaml")
}

// Environment overrides for secret-bearing server.yaml fields. The file stays the
// source of truth for shape and non-secret values; these exist so a deployment can
// inject credentials from a Kubernetes Secret, a Docker secret, or a CI variable
// without writing them to disk. Same pattern as jwt_secret.
const (
	// BUILDMAX_DATABASE_PASSWORD overrides database.password.
	EnvKeyBuildmaxDatabasePassword = "BUILDMAX_DATABASE_PASSWORD"
	// BUILDMAX_STORAGE_MINIO_ACCESS_KEY overrides storage.minio.access_key.
	EnvKeyBuildmaxMinIOAccessKey = "BUILDMAX_STORAGE_MINIO_ACCESS_KEY"
	// BUILDMAX_STORAGE_MINIO_SECRET_KEY overrides storage.minio.secret_key.
	EnvKeyBuildmaxMinIOSecretKey = "BUILDMAX_STORAGE_MINIO_SECRET_KEY"
	// BUILDMAX_WORKER_TOKEN overrides worker.token, the shared secret for /api/worker/*.
	EnvKeyBuildmaxWorkerToken = "BUILDMAX_WORKER_TOKEN"
	// BUILDMAX_CONVERSATION_MODEL_API_KEY overrides conversation.model.api_key.
	EnvKeyBuildmaxConversationAPIKey = "BUILDMAX_CONVERSATION_MODEL_API_KEY"
)

// LoadServerConfig reads BUILDMAX_HOME/server.yaml via Viper and applies defaults.
// Secret-bearing fields can be overridden by the environment variables above.
// A missing file is not an error — returns a config with all defaults applied.
func LoadServerConfig() (ServerConfig, error) {
	v := viper.New()
	v.SetConfigFile(ServerConfigPath())

	// Defaults
	v.SetDefault("log_level", "info")
	v.SetDefault("port", 5678)
	v.SetDefault("cors_origin", "http://localhost:5173")
	v.SetDefault("access_token_ttl", "168h")
	v.SetDefault("refresh_token_ttl", "720h")
	v.SetDefault("refresh_rotation_grace", "30s")
	v.SetDefault("default_quota_tier", "free_trial")
	v.SetDefault("webhook.message_path", "message")
	v.SetDefault("webhook.user_id", "webhook")
	v.SetDefault("worker.binary", "buildmax-worker")
	v.SetDefault("worker.run_mode", "local_process")
	v.SetDefault("worker.k8s.namespace", "buildmax")
	v.SetDefault("worker.k8s.image", "buildmax:local")
	v.SetDefault("worker.k8s.config_map", "buildmax-config")
	v.SetDefault("worker.k8s.home_dir", "/buildmax")
	v.SetDefault("storage.persist_backend", ProviderLocalFS)
	v.SetDefault("storage.artifact_backend", ProviderLocalFS)
	// endpoint, region, and the two keys have no defaults on purpose.
	//
	// A default endpoint sends a deployment to localhost; default credentials
	// send it as a MinIO development user. Both used to be set, which made
	// "leave it empty and let the SDK resolve it" impossible to express — the
	// value was never empty. Unset now means unset, which is what selects AWS
	// endpoint resolution and the default credential chain.
	v.SetDefault("storage.minio.bucket", "bmstore")
	v.SetDefault("storage.minio.prefix", "workspaces")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 3306)
	v.SetDefault("database.user", "buildmax")
	v.SetDefault("database.password", "buildmax")
	v.SetDefault("database.name", "buildmax")
	v.SetDefault("conversation.model.context_window", 0)

	// Environment overrides for values that should not live in the file on disk.
	// An explicit env name is passed, so SetEnvPrefix does not apply to these.
	v.SetEnvPrefix("BUILDMAX")
	_ = v.BindEnv("jwt_secret", EnvKeyBuildmaxJWTSecret)
	_ = v.BindEnv("database.password", EnvKeyBuildmaxDatabasePassword)
	_ = v.BindEnv("storage.minio.access_key", EnvKeyBuildmaxMinIOAccessKey)
	_ = v.BindEnv("storage.minio.secret_key", EnvKeyBuildmaxMinIOSecretKey)
	_ = v.BindEnv("worker.token", EnvKeyBuildmaxWorkerToken)
	_ = v.BindEnv("conversation.model.api_key", EnvKeyBuildmaxConversationAPIKey)

	if err := v.ReadInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ServerConfig{}, err
	}

	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return ServerConfig{}, err
	}
	return cfg, nil
}
