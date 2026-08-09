package mcp

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTransport(cfg config.MCPServerConfig, httpClient *http.Client) (mcpsdk.Transport, error) {
	switch cfg.Type {
	case "stdio":
		cmd := exec.Command(cfg.Command, cfg.Args...)
		if len(cfg.Env) > 0 {
			cmd.Env = mergedEnv(cfg.Env)
		}
		return &mcpsdk.CommandTransport{Command: cmd}, nil
	case "sse":
		if httpClient == nil {
			httpClient = http.DefaultClient
		}
		return &mcpsdk.SSEClientTransport{Endpoint: cfg.URL, HTTPClient: httpClient}, nil
	case "http":
		if httpClient == nil {
			httpClient = http.DefaultClient
		}
		return &mcpsdk.StreamableClientTransport{Endpoint: cfg.URL, HTTPClient: httpClient}, nil
	default:
		return nil, fmt.Errorf("unknown mcp transport type %q", cfg.Type)
	}
}

// mergedEnv returns os.Environ() with keys in overrides replaced or appended.
func mergedEnv(overrides map[string]string) []string {
	if len(overrides) == 0 {
		return nil
	}
	omit := make(map[string]struct{}, len(overrides))
	for k := range overrides {
		omit[k] = struct{}{}
	}
	base := os.Environ()
	out := make([]string, 0, len(base)+len(overrides))
	for _, e := range base {
		i := strings.IndexByte(e, '=')
		if i <= 0 {
			continue
		}
		name := e[:i]
		if _, skip := omit[name]; skip {
			continue
		}
		out = append(out, e)
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}
