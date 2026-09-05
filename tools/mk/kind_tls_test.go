package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

// TestGenerateWorkerAPICert checks the certificate kind serves the worker
// listener with: it must load as a keypair, carry the Service DNS name the
// worker verifies, and — being self-signed — verify against itself as the CA the
// worker mounts. If any of these slip, the kind smoke's HTTPS worker path fails
// with a handshake error instead of proving the boundary.
func TestGenerateWorkerAPICert(t *testing.T) {
	certPEM, keyPEM, err := generateWorkerAPICert()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("the cert and key are not a usable pair: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("certificate PEM did not decode")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	if err := cert.VerifyHostname(workerAPIServiceDNS); err != nil {
		t.Errorf("certificate is not valid for %q: %v", workerAPIServiceDNS, err)
	}

	// The same cert is the CA the worker trusts. It must chain to itself, and it
	// must be valid for the Service name — the two facts the worker's HTTP client
	// checks on every call.
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	if _, err := cert.Verify(x509.VerifyOptions{DNSName: workerAPIServiceDNS, Roots: roots}); err != nil {
		t.Errorf("self-signed cert does not verify as its own CA: %v", err)
	}
}
