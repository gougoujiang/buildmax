package workerclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
)

// TLSClientOptions configures the trust a worker uses to reach the server's
// worker listener. See docs/design/worker-api-network-boundary.md §6.
type TLSClientOptions struct {
	// CAFile is a PEM bundle the client verifies the server certificate
	// against. Empty uses the system trust roots, which is correct only when the
	// server certificate chains to a public root.
	CAFile string
	// ClientCertFile and ClientKeyFile are an optional client certificate for
	// native mTLS, presented in addition to the run token. Both or neither.
	ClientCertFile string
	ClientKeyFile  string
}

// NewHTTPClient builds the one reusable *http.Client a worker uses for every
// call back to the server, rather than http.DefaultClient. Server identity is
// always verified against the request URL's host and the configured (or system)
// roots — there is no insecure-skip mode, because a leaked run token plus an
// unverified server is exactly the disclosure this boundary exists to prevent.
//
// A plain-HTTP server_url never exercises the TLS config, so a development
// deployment can pass the zero value.
func NewHTTPClient(opts TLSClientOptions) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if opts.CAFile != "" {
		pemBytes, err := os.ReadFile(opts.CAFile)
		if err != nil {
			return nil, fmt.Errorf("worker server CA %q: %w", opts.CAFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("worker server CA %q contains no certificates", opts.CAFile)
		}
		tlsCfg.RootCAs = pool
	}

	switch {
	case opts.ClientCertFile != "" && opts.ClientKeyFile != "":
		cert, err := tls.LoadX509KeyPair(opts.ClientCertFile, opts.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("worker client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	case opts.ClientCertFile != "" || opts.ClientKeyFile != "":
		return nil, fmt.Errorf("worker client certificate needs both client_cert_file and client_key_file")
	}

	// Clone the default transport so timeouts, proxy handling, and HTTP/2 stay
	// as they were; only the trust changes.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tlsCfg
	return &http.Client{Transport: tr}, nil
}
