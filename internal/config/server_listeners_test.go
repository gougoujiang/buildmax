package config

import "testing"

// TestValidateListeners covers the fail-closed rules that keep the two-listener
// boundary from collapsing into two names for one socket, or into a TLS
// configuration that half-exists. See
// docs/design/worker-api-network-boundary.md §10.
func TestValidateListeners(t *testing.T) {
	base := func() ServerConfig {
		return ServerConfig{WorkerAPI: ServerWorkerAPIConfig{Listen: "127.0.0.1:5679"}}
	}

	t.Run("the loopback default passes against the default public port", func(t *testing.T) {
		if err := base().ValidateListeners(":5678"); err != nil {
			t.Fatalf("default configuration rejected: %v", err)
		}
	})

	t.Run("a worker listener on the public port is refused", func(t *testing.T) {
		c := base()
		c.WorkerAPI.Listen = "127.0.0.1:5678"
		if err := c.ValidateListeners(":5678"); err == nil {
			t.Fatal("a shared port was accepted; the two listeners must differ")
		}
	})

	t.Run("a bare-colon worker port equal to the public port is refused", func(t *testing.T) {
		c := base()
		c.WorkerAPI.Listen = ":5678"
		if err := c.ValidateListeners(":5678"); err == nil {
			t.Fatal("a shared port was accepted despite differing hosts")
		}
	})

	t.Run("an empty worker listen address is refused", func(t *testing.T) {
		c := base()
		c.WorkerAPI.Listen = ""
		if err := c.ValidateListeners(":5678"); err == nil {
			t.Fatal("an empty worker_api.listen was accepted")
		}
	})

	t.Run("half a TLS keypair is refused", func(t *testing.T) {
		for _, tc := range []ServerWorkerAPITLSConfig{
			{CertFile: "cert.pem"},
			{KeyFile: "key.pem"},
		} {
			c := base()
			c.WorkerAPI.TLS = tc
			if err := c.ValidateListeners(":5678"); err == nil {
				t.Errorf("half a keypair was accepted: %+v", tc)
			}
		}
	})

	t.Run("a complete TLS keypair passes", func(t *testing.T) {
		c := base()
		c.WorkerAPI.TLS = ServerWorkerAPITLSConfig{CertFile: "cert.pem", KeyFile: "key.pem"}
		if err := c.ValidateListeners(":5678"); err != nil {
			t.Fatalf("a complete keypair was rejected: %v", err)
		}
	})

	t.Run("half an mTLS client identity is refused", func(t *testing.T) {
		for _, tc := range []struct{ cert, key string }{
			{cert: "client.pem"},
			{key: "client-key.pem"},
		} {
			c := base()
			c.Worker.ClientCertFile = tc.cert
			c.Worker.ClientKeyFile = tc.key
			if err := c.ValidateListeners(":5678"); err == nil {
				t.Errorf("half an mTLS identity was accepted: %+v", tc)
			}
		}
	})

	t.Run("a complete mTLS client identity passes", func(t *testing.T) {
		c := base()
		c.Worker.ClientCertFile = "client.pem"
		c.Worker.ClientKeyFile = "client-key.pem"
		if err := c.ValidateListeners(":5678"); err != nil {
			t.Fatalf("a complete mTLS identity was rejected: %v", err)
		}
	})
}

// TestLoadServerConfigDefaultsWorkerAPIListen pins the secure default: a
// deployment that configures nothing gets a loopback worker listener, not a
// zero value that would bind everything or fail to bind at all.
func TestLoadServerConfigDefaultsWorkerAPIListen(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, dir)
	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.WorkerAPI.Listen != "127.0.0.1:5679" {
		t.Errorf("worker_api.listen default = %q; want 127.0.0.1:5679", cfg.WorkerAPI.Listen)
	}
}
