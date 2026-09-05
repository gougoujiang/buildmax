package workerclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewHTTPClientVerifiesServerCertificate is M2's core property: the worker
// verifies the server against a configured CA and never falls back to trusting
// an unverified certificate. See docs/design/worker-api-network-boundary.md §6.
func TestNewHTTPClientVerifiesServerCertificate(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	caFile := writeCertPEM(t, ts.Certificate().Raw)
	// A genuine, unrelated CA the server never used, so verification must fail.
	wrongCAFile := writeCertPEM(t, selfSignedDER(t))

	t.Run("trusting the server CA succeeds", func(t *testing.T) {
		c, err := NewHTTPClient(TLSClientOptions{CAFile: caFile})
		if err != nil {
			t.Fatalf("build client: %v", err)
		}
		resp, err := c.Get(ts.URL)
		if err != nil {
			t.Fatalf("request with the right CA failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d; want 204", resp.StatusCode)
		}
	})

	t.Run("a certificate from a different CA is rejected", func(t *testing.T) {
		c, err := NewHTTPClient(TLSClientOptions{CAFile: wrongCAFile})
		if err != nil {
			t.Fatalf("build client: %v", err)
		}
		if _, err := c.Get(ts.URL); err == nil {
			t.Fatal("request succeeded against a server signed by an untrusted CA; the worker must reject it")
		}
	})

	t.Run("system roots do not trust a private server certificate", func(t *testing.T) {
		c, err := NewHTTPClient(TLSClientOptions{}) // no CA -> system roots
		if err != nil {
			t.Fatalf("build client: %v", err)
		}
		if _, err := c.Get(ts.URL); err == nil {
			t.Fatal("request succeeded with system roots against a private cert; there must be no insecure fallback")
		}
	})
}

func TestNewHTTPClientConfigErrors(t *testing.T) {
	t.Run("an unreadable CA file is an error, not a silent system-root fallback", func(t *testing.T) {
		if _, err := NewHTTPClient(TLSClientOptions{CAFile: filepath.Join(t.TempDir(), "missing.pem")}); err == nil {
			t.Fatal("a missing CA file was accepted")
		}
	})

	t.Run("a CA file with no certificates is an error", func(t *testing.T) {
		empty := filepath.Join(t.TempDir(), "empty.pem")
		if err := os.WriteFile(empty, []byte("not a certificate\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewHTTPClient(TLSClientOptions{CAFile: empty}); err == nil {
			t.Fatal("a CA file with no certificates was accepted")
		}
	})

	t.Run("half an mTLS client pair is an error", func(t *testing.T) {
		if _, err := NewHTTPClient(TLSClientOptions{ClientCertFile: "cert.pem"}); err == nil {
			t.Fatal("a client certificate with no key was accepted")
		}
		if _, err := NewHTTPClient(TLSClientOptions{ClientKeyFile: "key.pem"}); err == nil {
			t.Fatal("a client key with no certificate was accepted")
		}
	})
}

// selfSignedDER returns the DER of a throwaway self-signed certificate, used as
// a CA the test server never signed with.
func selfSignedDER(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "unrelated-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// writeCertPEM writes a DER certificate to a temp PEM file and returns its path.
func writeCertPEM(t *testing.T, der []byte) string {
	t.Helper()
	if _, err := x509.ParseCertificate(der); err != nil {
		t.Fatalf("test certificate is not parseable: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	body := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
