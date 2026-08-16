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
	// PathStyle forces bucket-in-path addressing on or off. Nil derives it
	// from Endpoint, which is the right answer for both of the cases that
	// actually occur — see bootstrap.BuildS3Client.
	PathStyle *bool
}
