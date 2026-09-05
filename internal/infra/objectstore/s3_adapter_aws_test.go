package objectstore

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// readerOnly hides any Seeker/Len the underlying value has, so the body looks
// to the SDK exactly like a request's multipart part: a plain stream of unknown
// length. This is the shape that a bare PutObject rejects over plain HTTP.
type readerOnly struct{ r io.Reader }

func (r readerOnly) Read(p []byte) (int, error) { return r.r.Read(p) }

// TestPutObjectAcceptsUnseekableStreamOverHTTP guards the artifact-upload fix.
//
// Artifact content reaches storage as an unseekable stream over a plain-HTTP S3
// endpoint (the in-cluster MinIO). A bare s3.PutObject cannot sign such a body
// -- it has neither a length nor a way to rewind for the payload hash, and TLS
// is required for the streaming-checksum fallback -- so it failed the whole
// upload. Routing through the transfer manager fixes it; this test fails on a
// regression back to a bare PutObject.
func TestPutObjectAcceptsUnseekableStreamOverHTTP(t *testing.T) {
	want := []byte("artifact bytes streamed from a request, no length, no seek")

	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The transfer manager sends a small body as a single PutObject. Record
		// the object write; answer everything with the S3 success shape.
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			got = body
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("k", "s", "")),
	)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
		o.UsePathStyle = true
	})

	adapter := NewS3ClientAdapter(client)
	if err := adapter.PutObject(context.Background(), "bucket", "key", readerOnly{bytes.NewReader(want)}); err != nil {
		t.Fatalf("PutObject with an unseekable stream over HTTP: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored body = %q, want %q", got, want)
	}
}
