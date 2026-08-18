package config

// The operator-facing view of server.yaml.
//
// It lives here, next to the struct it describes, rather than in the handler
// that serves it. When someone adds a configuration field, the decision about
// whether it may be shown is then in the file they are already editing, which
// is the best chance of that decision being made at all.
//
// The shape is a whitelist: every field below is named on purpose, and a field
// added to ServerConfig appears here only when someone puts it here. A
// blacklist — "hide anything called password" — fails open on every field added
// afterwards, which is the wrong direction for the one endpoint whose job is to
// be safe to look at.
//
// See docs/design/system-administration.md section 7.

// SecretStatus reports that a credential is configured, never anything about
// its value. Not a prefix, not a length, not a hash: each of those narrows a
// search for someone who has the response and wants the secret.
type SecretStatus struct {
	Set bool `json:"set"`
}

func secretStatus(v string) SecretStatus { return SecretStatus{Set: v != ""} }

// RedactedServerConfig is the effective server configuration, safe to show a
// System Administrator.
type RedactedServerConfig struct {
	LogLevel             string `json:"log_level,omitempty"`
	Port                 int    `json:"port"`
	AllowSignup          bool   `json:"allow_signup"`
	CORSOrigin           string `json:"cors_origin,omitempty"`
	WorkspacesDir        string `json:"workspaces_dir,omitempty"`
	DefaultQuotaTier     string `json:"default_quota_tier,omitempty"`
	AccessTokenTTL       string `json:"access_token_ttl,omitempty"`
	RefreshTokenTTL      string `json:"refresh_token_ttl,omitempty"`
	RefreshRotationGrace string `json:"refresh_rotation_grace,omitempty"`

	JWTSecret SecretStatus `json:"jwt_secret"`

	Database RedactedDBConfig      `json:"database"`
	Storage  RedactedStorageConfig `json:"storage"`
	Worker   RedactedWorkerConfig  `json:"worker"`
	LLM      RedactedLLMConfig     `json:"llm"`

	// Warnings are configuration states worth an operator's attention. They are
	// not errors — the server is running — and they are computed rather than
	// stored, so the list reflects the process's own view of itself.
	Warnings []string `json:"warnings"`
}

// RedactedDBConfig shows where the database is, never how to open it.
type RedactedDBConfig struct {
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
	User string `json:"user,omitempty"`
	Name string `json:"name,omitempty"`
	// TLS is the effective mode, not the raw setting, so an unset value reads
	// as what the connection actually does.
	TLS      string       `json:"tls"`
	Password SecretStatus `json:"password"`
}

// RedactedStorageConfig shows which backends are in use.
type RedactedStorageConfig struct {
	PersistBackend  string       `json:"persist_backend,omitempty"`
	ArtifactBackend string       `json:"artifact_backend,omitempty"`
	MinIOEndpoint   string       `json:"minio_endpoint,omitempty"`
	MinIOBucket     string       `json:"minio_bucket,omitempty"`
	MinIORegion     string       `json:"minio_region,omitempty"`
	MinIOAccessKey  SecretStatus `json:"minio_access_key"`
	MinIOSecretKey  SecretStatus `json:"minio_secret_key"`
}

// RedactedWorkerConfig shows how runs are launched.
type RedactedWorkerConfig struct {
	RunMode      string       `json:"run_mode,omitempty"`
	ServerURL    string       `json:"server_url,omitempty"`
	RunTokenTTL  string       `json:"run_token_ttl,omitempty"`
	RunTimeout   string       `json:"run_timeout,omitempty"`
	LLMTransport string       `json:"llm_transport,omitempty"`
	LLMAlias     string       `json:"llm_alias,omitempty"`
	K8sNamespace string       `json:"k8s_namespace,omitempty"`
	K8sImage     string       `json:"k8s_image,omitempty"`
	SharedToken  SecretStatus `json:"shared_token"`
}

// RedactedLLMConfig shows the deployment's model policy. Aliases name catalog
// ids, which are identifiers rather than credentials; the credentials are in
// the llm_model table and are never served anywhere.
type RedactedLLMConfig struct {
	DefaultAlias  string            `json:"default_alias,omitempty"`
	Aliases       map[string]string `json:"aliases,omitempty"`
	ConversationM RedactedModel     `json:"conversation_model"`
}

// RedactedModel describes the Tier 1 model without its credential.
type RedactedModel struct {
	Name        string       `json:"name,omitempty"`
	Model       string       `json:"model,omitempty"`
	APIURL      string       `json:"api_url,omitempty"`
	ModelTarget string       `json:"model_target,omitempty"`
	APIKey      SecretStatus `json:"api_key"`
}

// Redacted returns the operator-facing view of the configuration.
func (sc ServerConfig) Redacted() RedactedServerConfig {
	out := RedactedServerConfig{
		LogLevel:         sc.LogLevel,
		Port:             sc.Port,
		AllowSignup:      sc.AllowSignup,
		CORSOrigin:       sc.CORSOrigin,
		WorkspacesDir:    sc.WorkspacesDir,
		DefaultQuotaTier: sc.DefaultQuotaTier,
		JWTSecret:        secretStatus(sc.JWTSecret),
		Database: RedactedDBConfig{
			Host:     sc.Database.Host,
			Port:     sc.Database.Port,
			User:     sc.Database.User,
			Name:     sc.Database.Name,
			TLS:      effectiveDBTLS(sc.Database.TLS),
			Password: secretStatus(sc.Database.Password),
		},
		Storage: RedactedStorageConfig{
			PersistBackend:  sc.Storage.PersistBackend,
			ArtifactBackend: sc.Storage.ArtifactBackend,
			MinIOEndpoint:   sc.Storage.MinIO.Endpoint,
			MinIOBucket:     sc.Storage.MinIO.Bucket,
			MinIORegion:     sc.Storage.MinIO.Region,
			MinIOAccessKey:  secretStatus(sc.Storage.MinIO.AccessKey),
			MinIOSecretKey:  secretStatus(sc.Storage.MinIO.SecretKey),
		},
		Worker: RedactedWorkerConfig{
			RunMode:      sc.Worker.RunMode,
			ServerURL:    sc.Worker.ServerURL,
			LLMTransport: sc.Worker.LLM.Transport,
			LLMAlias:     sc.Worker.LLM.Alias,
			K8sNamespace: sc.Worker.K8s.Namespace,
			K8sImage:     sc.Worker.K8s.Image,
			SharedToken:  secretStatus(sc.Worker.Token),
		},
		LLM: RedactedLLMConfig{
			DefaultAlias: sc.LLM.DefaultAlias,
			Aliases:      sc.LLM.Aliases,
			ConversationM: RedactedModel{
				Name:        sc.Conversation.Model.Name,
				Model:       sc.Conversation.Model.Model,
				APIURL:      sc.Conversation.Model.APIURL,
				ModelTarget: sc.Conversation.ModelTarget,
				APIKey:      secretStatus(sc.Conversation.Model.APIKey),
			},
		},
	}
	if sc.AccessTokenTTL > 0 {
		out.AccessTokenTTL = sc.AccessTokenTTL.String()
	}
	if sc.RefreshTokenTTL > 0 {
		out.RefreshTokenTTL = sc.RefreshTokenTTL.String()
	}
	if sc.RefreshRotationGrace > 0 {
		out.RefreshRotationGrace = sc.RefreshRotationGrace.String()
	}
	if sc.Worker.RunTokenTTL > 0 {
		out.Worker.RunTokenTTL = sc.Worker.RunTokenTTL.String()
	}
	if sc.Worker.RunTimeout > 0 {
		out.Worker.RunTimeout = sc.Worker.RunTimeout.String()
	}
	out.Warnings = sc.configWarnings()
	return out
}

func effectiveDBTLS(mode string) string {
	if mode == "" {
		return DefaultDBTLSMode
	}
	return mode
}

// configWarnings lists configuration states an operator should know about.
//
// Every entry is a documented trade-off somewhere else in the project; this is
// where a deployment finds out which of them apply to it, without reading the
// documentation for all of them.
func (sc ServerConfig) configWarnings() []string {
	warnings := []string{}
	if sc.AllowSignup {
		warnings = append(warnings, "allow_signup is on: anyone who can reach the server can create an account")
	}
	if sc.Worker.Token != "" {
		warnings = append(warnings, "worker.token is set: the deployment-wide worker token is deprecated and names no run; remove it once every server has restarted")
	}
	if sc.Worker.LLM.Managed() && len(sc.LLM.Aliases) == 0 {
		warnings = append(warnings, "worker.llm.transport is buildmax but llm.aliases is empty: no team can call a managed model")
	}
	if sc.Worker.RunMode != "k8s_job" {
		warnings = append(warnings, "worker.run_mode is not k8s_job: runs execute as local processes on the server, which is a development path rather than a deployment topology")
	}
	if sc.Worker.RunTokenTTL > 0 && sc.Worker.RunTimeout > sc.Worker.RunTokenTTL {
		warnings = append(warnings, "worker.run_timeout is longer than worker.run_token_ttl: a run can outlive its credential and then cannot report an outcome")
	}
	if sc.Storage.PersistBackend == "" || sc.Storage.PersistBackend == "local_fs" {
		warnings = append(warnings, "storage.persist_backend is local_fs: run output lives on the server's disk and is lost with the pod")
	}
	return warnings
}
