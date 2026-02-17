package config

import (
	"context"
	"fmt"
	"os"
	"strings"

	"buildmax/internal/workspacestorage"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

// BuildS3Client creates an S3-compatible client (MinIO or AWS S3). Use when either provider is minio.
func BuildS3Client(ctx context.Context, cfg WorkspaceStorageConfig) (workspacestorage.S3Client, error) {
	// Custom endpoint (MinIO): use static credentials and endpoint resolver
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               cfg.Endpoint,
			SigningRegion:     cfg.Region,
			HostnameImmutable: true,
		}, nil
	})

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithEndpointResolverWithOptions(resolver),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	return workspacestorage.NewS3ClientAdapter(client), nil
}

// BuildPersistStorage returns the configured persist storage implementation.
func BuildPersistStorage(cfg WorkspaceStorageConfig, persistRoot func(workspaceID string) string, s3Client workspacestorage.S3Client) (workspacestorage.PersistStorage, error) {
	switch cfg.PersistProvider {
	case ProviderMinIO:
		if s3Client == nil {
			return nil, fmt.Errorf("persist storage is minio but S3 client is nil")
		}
		return workspacestorage.NewS3PersistStorage(s3Client, cfg.Bucket, cfg.Prefix), nil
	default:
		return workspacestorage.NewLocalFSPersistStorage(persistRoot), nil
	}
}

// BuildArtifactStorage returns the configured artifact storage implementation.
func BuildArtifactStorage(cfg WorkspaceStorageConfig, artifactDir func(workspaceID, taskID, artifactID string) string, s3Client workspacestorage.S3Client) (workspacestorage.ArtifactStorage, error) {
	switch cfg.ArtifactProvider {
	case ProviderMinIO:
		if s3Client == nil {
			return nil, fmt.Errorf("artifact storage is minio but S3 client is nil")
		}
		return workspacestorage.NewS3ArtifactStorage(s3Client, cfg.Bucket, cfg.Prefix), nil
	default:
		return workspacestorage.NewLocalFSArtifactStorage(artifactDir), nil
	}
}
