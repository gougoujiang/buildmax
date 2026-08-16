// Package bootstrap wires process startup dependencies.
package bootstrap

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/config"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// BuildS3Client creates an S3-compatible client. Use when either provider is minio.
//
// Two shapes are supported, and the endpoint is what distinguishes them: a
// deployment naming an endpoint is talking to a store it runs or a vendor's
// S3-compatible service, while one that names none is talking to AWS S3 and
// wants the SDK's own regional endpoint resolution.
//
// Credentials follow the same principle. Static keys are used when configured;
// leaving them empty falls through to the SDK's default chain, which is how a
// cluster reaches a bucket through IRSA, workload identity, or an instance
// profile instead of a long-lived key the deployment has to store and rotate.
func BuildS3Client(ctx context.Context, cfg config.WorkspaceStorageConfig) (blob.S3Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}
	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		)))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = usePathStyle(cfg)
	})
	return blob.NewS3ClientAdapter(client), nil
}

// usePathStyle decides bucket addressing.
//
// An explicit setting wins. Otherwise a configured endpoint means an
// S3-compatible store, which needs bucket-in-path; no endpoint means AWS S3,
// where virtual-host addressing is the supported form for buckets created
// since 2020.
func usePathStyle(cfg config.WorkspaceStorageConfig) bool {
	if cfg.PathStyle != nil {
		return *cfg.PathStyle
	}
	return cfg.Endpoint != ""
}

// BuildPersistStorage returns the configured persist storage implementation.
func BuildPersistStorage(cfg config.WorkspaceStorageConfig, persistRoot func(teamID string) string, s3Client blob.S3Client) (blob.PersistStorage, error) {
	switch cfg.PersistProvider {
	case config.ProviderMinIO:
		if s3Client == nil {
			return nil, fmt.Errorf("persist storage is minio but S3 client is nil")
		}
		return blob.NewS3PersistStorage(s3Client, cfg.Bucket, cfg.Prefix), nil
	default:
		return blob.NewLocalFSPersistStorage(persistRoot), nil
	}
}

// BuildArtifactStorage returns the configured artifact storage implementation.
// runOutputDir is (userID, conversationID, taskID, taskRunID) -> path for run output files.
func BuildArtifactStorage(cfg config.WorkspaceStorageConfig, runOutputDir func(userID, conversationID, taskID, taskRunID string) string, s3Client blob.S3Client) (blob.ArtifactStorage, error) {
	switch cfg.ArtifactProvider {
	case config.ProviderMinIO:
		if s3Client == nil {
			return nil, fmt.Errorf("artifact storage is minio but S3 client is nil")
		}
		return blob.NewS3ArtifactStorage(s3Client, cfg.Bucket, cfg.Prefix), nil
	default:
		return blob.NewLocalFSArtifactStorage(runOutputDir), nil
	}
}
