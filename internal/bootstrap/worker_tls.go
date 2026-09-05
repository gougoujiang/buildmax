package bootstrap

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/gougoujiang/buildmax/internal/config"
)

// buildWorkerListenerTLS builds the worker listener's server TLS configuration,
// or nil when no certificate is configured — a development deployment that
// serves the worker API over plain HTTP. A half-configured keypair is already
// refused by ServerConfig.ValidateListeners before this runs; loading it here
// is what turns a missing or malformed certificate file into a startup failure
// rather than a listener that cannot complete a handshake.
//
// When client_ca_file is set the listener requires a client certificate that CA
// issued (native mTLS), in addition to the run token. See
// docs/design/worker-api-network-boundary.md §6.
func buildWorkerListenerTLS(t config.ServerWorkerAPITLSConfig) (*tls.Config, error) {
	if t.CertFile == "" && t.KeyFile == "" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("worker_api.tls certificate: %w", err)
	}
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}
	if t.ClientCAFile != "" {
		pemBytes, err := os.ReadFile(t.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("worker_api.tls client CA %q: %w", t.ClientCAFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("worker_api.tls client CA %q contains no certificates", t.ClientCAFile)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}
