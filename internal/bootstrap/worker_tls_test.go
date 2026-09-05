package bootstrap

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
)

func TestBuildWorkerListenerTLS(t *testing.T) {
	certFile, keyFile, _ := writeServerCert(t)

	t.Run("no certificate means plain HTTP", func(t *testing.T) {
		cfg, err := buildWorkerListenerTLS(config.ServerWorkerAPITLSConfig{})
		if err != nil {
			t.Fatalf("empty config errored: %v", err)
		}
		if cfg != nil {
			t.Error("a listener with no certificate must serve plain HTTP (nil TLS)")
		}
	})

	t.Run("a keypair loads and requires no client certificate", func(t *testing.T) {
		cfg, err := buildWorkerListenerTLS(config.ServerWorkerAPITLSConfig{CertFile: certFile, KeyFile: keyFile})
		if err != nil {
			t.Fatalf("valid keypair errored: %v", err)
		}
		if cfg == nil || len(cfg.Certificates) != 1 {
			t.Fatalf("certificate not loaded: %+v", cfg)
		}
		if cfg.ClientAuth != tls.NoClientCert {
			t.Errorf("ClientAuth = %v; want NoClientCert without a client CA", cfg.ClientAuth)
		}
		if cfg.MinVersion != tls.VersionTLS12 {
			t.Errorf("MinVersion = %x; want TLS 1.2", cfg.MinVersion)
		}
	})

	t.Run("a client CA turns on required mTLS", func(t *testing.T) {
		caFile, _, _ := writeServerCert(t)
		cfg, err := buildWorkerListenerTLS(config.ServerWorkerAPITLSConfig{
			CertFile: certFile, KeyFile: keyFile, ClientCAFile: caFile,
		})
		if err != nil {
			t.Fatalf("mTLS config errored: %v", err)
		}
		if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
			t.Errorf("ClientAuth = %v; want RequireAndVerifyClientCert", cfg.ClientAuth)
		}
		if cfg.ClientCAs == nil {
			t.Error("ClientCAs not set despite a client CA file")
		}
	})

	t.Run("a missing certificate file is a startup error", func(t *testing.T) {
		_, err := buildWorkerListenerTLS(config.ServerWorkerAPITLSConfig{
			CertFile: filepath.Join(t.TempDir(), "nope.crt"), KeyFile: keyFile,
		})
		if err == nil {
			t.Fatal("a missing certificate file was accepted")
		}
	})
}

// TestWorkerListenerServesTLS proves the built config actually completes a
// handshake a client that trusts the certificate can verify.
func TestWorkerListenerServesTLS(t *testing.T) {
	certFile, keyFile, certDER := writeServerCert(t)
	tlsCfg, err := buildWorkerListenerTLS(config.ServerWorkerAPITLSConfig{CertFile: certFile, KeyFile: keyFile})
	if err != nil {
		t.Fatalf("build listener TLS: %v", err)
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ts.TLS = tlsCfg
	ts.StartTLS()
	defer ts.Close()

	pool := x509.NewCertPool()
	cert, _ := x509.ParseCertificate(certDER)
	pool.AddCert(cert)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("a trusting client could not complete the handshake: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d; want 204", resp.StatusCode)
	}
}

// writeServerCert writes a self-signed ECDSA cert (valid for 127.0.0.1) and its
// key to temp PEM files and returns their paths plus the cert DER.
func writeServerCert(t *testing.T) (certPath, keyPath string, der []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "worker-api-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, certPath, "CERTIFICATE", der)
	writePEM(t, keyPath, "PRIVATE KEY", keyDER)
	return certPath, keyPath, der
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	body := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
