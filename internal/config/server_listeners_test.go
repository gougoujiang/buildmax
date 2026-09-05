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

// TestValidateWorkerURL covers the worker-URL rules: a k8s_job must not use
// plaintext without an explicit opt-in, and no worker URL may point back at the
// public listener. See docs/design/worker-api-network-boundary.md §6 and §10.
func TestValidateWorkerURL(t *testing.T) {
	base := func() ServerConfig {
		return ServerConfig{WorkerAPI: ServerWorkerAPIConfig{Listen: "127.0.0.1:5679"}}
	}

	t.Run("k8s_job over http without the opt-in is refused", func(t *testing.T) {
		c := base()
		c.Worker.RunMode = "k8s_job"
		c.Worker.ServerURL = "http://buildmax-worker-api.buildmax.svc.cluster.local:5679"
		if err := c.ValidateListeners(":5678"); err == nil {
			t.Fatal("a k8s_job over plaintext http was accepted without allow_insecure_http")
		}
	})

	t.Run("k8s_job over http with the opt-in passes", func(t *testing.T) {
		c := base()
		c.Worker.RunMode = "k8s_job"
		c.Worker.ServerURL = "http://buildmax-worker-api.buildmax.svc.cluster.local:5679"
		c.Worker.AllowInsecureHTTP = true
		if err := c.ValidateListeners(":5678"); err != nil {
			t.Fatalf("a k8s_job with allow_insecure_http was rejected: %v", err)
		}
	})

	t.Run("k8s_job over https passes without the opt-in", func(t *testing.T) {
		c := base()
		c.Worker.RunMode = "k8s_job"
		c.Worker.ServerURL = "https://buildmax-worker-api.buildmax.svc.cluster.local:5679"
		if err := c.ValidateListeners(":5678"); err != nil {
			t.Fatalf("a k8s_job over https was rejected: %v", err)
		}
	})

	t.Run("local_process over http is fine", func(t *testing.T) {
		c := base()
		c.Worker.RunMode = "local_process"
		c.Worker.ServerURL = "http://127.0.0.1:5679"
		if err := c.ValidateListeners(":5678"); err != nil {
			t.Fatalf("local_process over loopback http was rejected: %v", err)
		}
	})

	t.Run("a loopback worker URL on the public port is refused", func(t *testing.T) {
		c := base()
		c.Worker.RunMode = "local_process"
		c.Worker.ServerURL = "http://127.0.0.1:5678"
		if err := c.ValidateListeners(":5678"); err == nil {
			t.Fatal("a worker URL pointing at the public listener was accepted")
		}
	})

	t.Run("an empty worker URL is left to the worker to reject", func(t *testing.T) {
		c := base()
		c.Worker.ServerURL = ""
		if err := c.ValidateListeners(":5678"); err != nil {
			t.Fatalf("an empty server_url was rejected at the server: %v", err)
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
