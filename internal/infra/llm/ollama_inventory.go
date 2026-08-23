package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
)

// OllamaModel is one model a local daemon holds, as `buildmax models --local`
// and `buildmax doctor` describe it.
type OllamaModel struct {
	Model         string   `json:"model"`
	ParameterSize string   `json:"parameter_size,omitempty"`
	Quantization  string   `json:"quantization,omitempty"`
	Family        string   `json:"family,omitempty"`
	SizeBytes     int64    `json:"size_bytes,omitempty"`
	ContextWindow int      `json:"context_window,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
}

// Ollama capability names, as /api/show reports them. A model missing
// OllamaCapabilityTools cannot run the agent loop at all, which is worth saying
// before a run rather than after a turn of prose where a tool call belonged.
const (
	OllamaCapabilityTools    = "tools"
	OllamaCapabilityVision   = "vision"
	OllamaCapabilityThinking = "thinking"
)

// HasCapability reports whether the daemon listed name for this model.
func (m OllamaModel) HasCapability(name string) bool {
	for _, capability := range m.Capabilities {
		if capability == name {
			return true
		}
	}
	return false
}

// ollamaProbeTimeout bounds a diagnostic call to the daemon. It is short
// because every caller has something sensible to do without the answer, and
// none of them should hang on a daemon that is busy loading a model.
var ollamaProbeTimeout = 3 * time.Second

// OllamaInventory lists the models a local daemon has pulled.
//
// It answers "is the daemon there, and does it hold what settings.yaml names",
// which is the first question both `doctor` and `models --local` ask. Context
// windows and capabilities are per-model and come from OllamaShow.
func OllamaInventory(ctx context.Context, baseURL string) ([]OllamaModel, error) {
	var payload struct {
		Models []struct {
			Model string `json:"model"`
			Name  string `json:"name"`
			Size  int64  `json:"size"`
			// Newer daemons list capabilities here too. Older ones do not, so
			// this is a shortcut for a listing, never the answer to "can this
			// model call tools" — OllamaShow is.
			Capabilities []string `json:"capabilities"`
			Details      struct {
				Family           string `json:"family"`
				ParameterSize    string `json:"parameter_size"`
				QuantizationName string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := ollamaGet(ctx, baseURL, "/api/tags", &payload); err != nil {
		return nil, err
	}
	out := make([]OllamaModel, 0, len(payload.Models))
	for _, m := range payload.Models {
		name := m.Model
		if name == "" {
			name = m.Name
		}
		out = append(out, OllamaModel{
			Model:         name,
			ParameterSize: m.Details.ParameterSize,
			Quantization:  m.Details.QuantizationName,
			Family:        m.Details.Family,
			SizeBytes:     m.Size,
			Capabilities:  m.Capabilities,
		})
	}
	return out, nil
}

// OllamaShow returns what the daemon knows about one installed model: the
// context length it was trained for, and the capabilities it declares.
func OllamaShow(ctx context.Context, baseURL, model string) (OllamaModel, error) {
	return ollamaShow(ctx, http.DefaultClient, baseURL, model)
}

func ollamaShow(ctx context.Context, client *http.Client, baseURL, model string) (OllamaModel, error) {
	var payload struct {
		Capabilities []string       `json:"capabilities"`
		ModelInfo    map[string]any `json:"model_info"`
		Details      struct {
			Family           string `json:"family"`
			ParameterSize    string `json:"parameter_size"`
			QuantizationName string `json:"quantization_level"`
		} `json:"details"`
	}
	err := ollamaPost(ctx, client, baseURL, "/api/show", map[string]string{"model": model}, &payload)
	if err != nil {
		return OllamaModel{}, err
	}
	return OllamaModel{
		Model:         model,
		ParameterSize: payload.Details.ParameterSize,
		Quantization:  payload.Details.QuantizationName,
		Family:        payload.Details.Family,
		ContextWindow: modelInfoContextLength(payload.ModelInfo),
		Capabilities:  payload.Capabilities,
	}, nil
}

// modelInfoContextLength reads the trained context length out of the
// architecture-keyed model info. The key is prefixed with the architecture
// ("qwen3.context_length"), so the architecture is read first and any
// context_length key is accepted as a fallback — a new architecture must not
// make this silently report none.
func modelInfoContextLength(info map[string]any) int {
	if len(info) == 0 {
		return 0
	}
	if architecture, ok := info["general.architecture"].(string); ok && architecture != "" {
		if n, ok := numberFrom(info[architecture+".context_length"]); ok {
			return n
		}
	}
	for key, value := range info {
		if strings.HasSuffix(key, ".context_length") {
			if n, ok := numberFrom(value); ok {
				return n
			}
		}
	}
	return 0
}

func numberFrom(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), typed > 0
	case json.Number:
		n, err := typed.Int64()
		return int(n), err == nil && n > 0
	}
	return 0, false
}

// ollamaContextWindow decides the window for an entry that set none.
//
// It is the number sent as num_ctx and the number history is trimmed against —
// one value, so the two cannot disagree. The daemon's answer is a ceiling
// rather than a target: a model's full trained length can exceed what the
// machine can allocate, and context_window is how an operator asks for more.
//
// Every branch produces a window, including the one where the daemon did not
// answer. A failed probe must not end with num_ctx unset, which is the silent
// truncation this provider exists to prevent.
func ollamaContextWindow(cfg Config) int {
	ctx, cancel := context.WithTimeout(context.Background(), ollamaProbeTimeout)
	defer cancel()

	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	shown, err := ollamaShow(ctx, client, cfg.BaseURL, cfg.Model)
	if err != nil {
		slog.Warn("ollama context window probe failed, using the default",
			"model", cfg.Model, "default", config.DefaultContextWindow, "err", err)
		return config.DefaultContextWindow
	}
	if shown.ContextWindow <= 0 {
		return config.DefaultContextWindow
	}
	return min(shown.ContextWindow, config.DefaultContextWindow)
}

// --- HTTP --------------------------------------------------------------------

func ollamaGet(ctx context.Context, baseURL, path string, out any) error {
	url := ollamaBaseURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+path, nil)
	if err != nil {
		return err
	}
	return ollamaDo(http.DefaultClient, req, url, out)
}

func ollamaPost(ctx context.Context, client *http.Client, baseURL, path string, payload, out any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := ollamaBaseURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return ollamaDo(client, req, url, out)
}

// ollamaDo sends a diagnostic request and decodes its reply. Failures reuse the
// adapter's own error shaping, so "the daemon is not running" reads the same
// whether a run or a diagnostic found it.
func ollamaDo(client *http.Client, req *http.Request, baseURL string, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return ollamaTransportError(baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		message := strings.TrimSpace(string(body))
		var envelope struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &envelope) == nil && envelope.Error != "" {
			message = envelope.Error
		}
		return fmt.Errorf("%s: %w", req.URL.Path, &apiError{status: resp.StatusCode, message: message})
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: decode response: %w", req.URL.Path, err)
	}
	return nil
}
