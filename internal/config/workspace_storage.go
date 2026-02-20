package config

import (
	"os"
	"strings"
)

const (
	ProviderLocalFS = "local_fs"
	ProviderMinIO   = "minio"
)

// WorkspaceStorageConfig holds provider selection and S3/MinIO connection settings.
type WorkspaceStorageConfig struct {
	PersistProvider  string // "local_fs" or "minio"
	ArtifactProvider string // "local_fs" or "minio"
	// S3/MinIO connection (used when either provider is minio)
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	Bucket    string
	Prefix    string
}

// LoadWorkspaceStorageConfig reads env vars and returns the workspace storage config with defaults.
func LoadWorkspaceStorageConfig() WorkspaceStorageConfig {
	persist := os.Getenv(EnvKeyBuildmaxPersistStorage)
	if persist == "" {
		persist = ProviderLocalFS
	}
	persist = strings.ToLower(strings.TrimSpace(persist))
	if persist != ProviderLocalFS && persist != ProviderMinIO {
		persist = ProviderLocalFS
	}

	artifact := os.Getenv(EnvKeyBuildmaxArtifactStorage)
	if artifact == "" {
		artifact = ProviderLocalFS
	}
	artifact = strings.ToLower(strings.TrimSpace(artifact))
	if artifact != ProviderLocalFS && artifact != ProviderMinIO {
		artifact = ProviderLocalFS
	}

	endpoint := os.Getenv(EnvKeyBuildmaxMinioEndpoint)
	if endpoint == "" {
		endpoint = "http://localhost:9000"
	}
	region := os.Getenv(EnvKeyBuildmaxMinioRegion)
	if region == "" {
		region = "us-east-1"
	}
	accessKey := os.Getenv(EnvKeyBuildmaxMinioAccessKey)
	if accessKey == "" {
		accessKey = "minio"
	}
	secretKey := os.Getenv(EnvKeyBuildmaxMinioSecretKey)
	if secretKey == "" {
		secretKey = "minio123"
	}
	bucket := os.Getenv(EnvKeyBuildmaxMinioBucket)
	if bucket == "" {
		bucket = "bmstore"
	}
	prefix := os.Getenv(EnvKeyBuildmaxMinioPrefix)
	if prefix == "" {
		prefix = "workspaces"
	}

	return WorkspaceStorageConfig{
		PersistProvider:  persist,
		ArtifactProvider: artifact,
		Endpoint:         endpoint,
		Region:           region,
		AccessKey:        accessKey,
		SecretKey:        secretKey,
		Bucket:           bucket,
		Prefix:           prefix,
	}
}
