package config

const (
	ProviderLocalFS = "local_fs"
	ProviderMinIO   = "minio"
)

// WorkspaceStorageConfig holds resolved provider selection and S3/MinIO connection settings.
// Populated from ServerStorageConfig by bootstrap; not read from env directly.
type WorkspaceStorageConfig struct {
	PersistProvider  string
	ArtifactProvider string
	Endpoint         string
	Region           string
	AccessKey        string
	SecretKey        string
	Bucket           string
	Prefix           string
}
