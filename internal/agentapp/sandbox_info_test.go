package agentapp

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// TestSandboxInfoDowngraded_ConfigOnly asserts a configuration downgrade
// computed by config.ResolveSandboxForRun (carried on sandboxResolved.Downgraded)
// reaches the hook/trace payload even when the runtime matches what was asked.
func TestSandboxInfoDowngraded_ConfigOnly(t *testing.T) {
	app := &AgentApp{
		sandbox: agent.NoopSandbox{}, // Enabled() == false, matching Config.Enabled below
		sandboxResolved: config.SandboxResolution{
			Config:     config.SandboxConfig{Enabled: false},
			Downgraded: true,
		},
	}
	info := app.sandboxInfo()
	if info == nil || !info.Downgraded {
		t.Fatalf("sandboxInfo() = %+v, want Downgraded=true from the config diff alone", info)
	}
}

// TestSandboxInfoDowngraded_RuntimeFallback asserts the case
// config.ResolveSandboxForRun cannot see on its own: the resolved config
// asked for the sandbox, but the live view reports disabled -- the backend
// turned out unavailable and fail_if_unavailable was false, so Manager ran
// unconfined instead of refusing to start.
func TestSandboxInfoDowngraded_RuntimeFallback(t *testing.T) {
	app := &AgentApp{
		sandbox: agent.NoopSandbox{}, // Enabled() == false
		sandboxResolved: config.SandboxResolution{
			Config:     config.SandboxConfig{Enabled: true}, // asked for it
			Downgraded: false,                               // config diff alone sees no override
		},
	}
	info := app.sandboxInfo()
	if info == nil || !info.Downgraded {
		t.Fatalf("sandboxInfo() = %+v, want Downgraded=true: asked enabled, runtime disabled", info)
	}
}

// TestSandboxInfoDowngraded_NoDowngrade asserts the ordinary case -- resolved
// config matches the live view and config.ResolveSandboxForRun saw no
// override -- reports Downgraded=false.
func TestSandboxInfoDowngraded_NoDowngrade(t *testing.T) {
	app := &AgentApp{
		sandbox: agent.NoopSandbox{}, // Enabled() == false
		sandboxResolved: config.SandboxResolution{
			Config:     config.SandboxConfig{Enabled: false},
			Downgraded: false,
		},
	}
	info := app.sandboxInfo()
	if info == nil || info.Downgraded {
		t.Fatalf("sandboxInfo() = %+v, want Downgraded=false", info)
	}
}
