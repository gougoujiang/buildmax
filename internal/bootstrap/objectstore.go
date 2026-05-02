// Package bootstrap wires process startup dependencies.
package bootstrap

import (
	"context"
	"fmt"

	"buildmax/internal/config"
	blob "buildmax/internal/infra/objectstore"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// BuildS3Client creates an S3-compatible client (MinIO or AWS S3). Use when either provider is minio.
func BuildS3Client(ctx context.Context, cfg config.WorkspaceStorageConfig) (blob.S3Client, error) {
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
	return blob.NewS3ClientAdapter(client), nil
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
