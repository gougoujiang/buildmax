package bootstrap

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// TestUsePathStyle covers the two shapes a deployment actually has, and the
// escape hatch for the third. Getting this wrong is not a subtle bug: MinIO
// rejects virtual-host addressing, and AWS S3 has not supported path style for
// buckets created since 2020.
func TestUsePathStyle(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name string
		cfg  config.WorkspaceStorageConfig
		want bool
	}{
		{
			name: "an endpoint means a compatible store, which needs path style",
			cfg:  config.WorkspaceStorageConfig{Endpoint: "http://minio.storage.svc.cluster.local:9000"},
			want: true,
		},
		{
			name: "no endpoint means AWS S3, which does not",
			cfg:  config.WorkspaceStorageConfig{Region: "eu-west-1"},
			want: false,
		},
		{
			name: "an explicit setting wins over the endpoint",
			cfg:  config.WorkspaceStorageConfig{Endpoint: "https://s3.vendor.example", PathStyle: &no},
			want: false,
		},
		{
			name: "and wins in the other direction too",
			cfg:  config.WorkspaceStorageConfig{PathStyle: &yes},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usePathStyle(tt.cfg); got != tt.want {
				t.Errorf("usePathStyle = %v, want %v", got, tt.want)
			}
		})
	}
}
