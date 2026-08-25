// Package worker serves the routes a running worker calls back on.
//
// Its boundary is a different credential, not a different feature: every route
// here authenticates with the run token that names one task run, never with a
// user's access token. Sharing a Handler with the user-facing routes meant one
// type answered to both credentials, and the only thing keeping a worker route
// from reading a user's session was that nobody had written it.
package worker

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/server/handlers/runterminal"
	"net/http"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/websocket"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
)

type Config struct {
	// JWTSecret verifies the run token every route here requires. Empty means
	// this deployment mints none, so no worker call can be authenticated.
	JWTSecret string
	// WorkerLLM tells a worker how to reach a model. Nil means direct.
	WorkerLLM *workerclient.TaskRunLLM

	TaskRuns model.TaskRunStore
	Agents   agentdef.Store
	// Activations resolves what a run's team activated. Nil means this
	// deployment cannot, which is a refusal only for an agent that names a
	// plugin.
	Activations ActivationReader
	// Plugins serves the package bytes a run's pins name. Nil answers 503 on
	// the download route.
	Plugins *pluginsvc.Service
	Gateway *llmgateway.Service
	Hub     websocket.StreamHub
	// Artifacts lets a run's agent keep a file for the team. Nil means this
	// deployment has no artifact store, and the route answers 503 — which is
	// also what makes the worker leave the tool unregistered.
	Artifacts *artifactsvc.Service

	// OnTerminal is fired once a run reaches a terminal status, after the hub
	// has been told. The server supplies it; this package does not know who is
	// listening.
	OnTerminal func(ctx context.Context, info model.TaskRunTerminalInfo)
	// TerminalGroup owns those callbacks so a shutdown waits for them instead
	// of dropping them.
	TerminalGroup *runterminal.Group
}

// ActivationReader is the only team-plugin capability a run token receives.
// In particular, the worker surface cannot activate or repin a plugin.
type ActivationReader interface {
	GetPluginActivation(ctx context.Context, teamID, pluginName string) (*coreplugin.Activation, error)
}

type Handler struct{ cfg Config }

// New builds the worker API. A nil Hub gets one of its own, which is what the
// unified handler did: a deployment with nobody watching still has runs to
// stream.
func New(cfg Config) *Handler {
	if cfg.Hub == nil {
		cfg.Hub = websocket.NewStreamHub()
	}
	return &Handler{cfg: cfg}
}

// Register adds the worker API.
//
// Every route is scoped to one run, so every route authenticates with that
// run's token.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/worker/task-runs/{task_run_id}", h.runScopedWorkerMiddleware(http.HandlerFunc(h.getTaskRun)))
	mux.Handle("PATCH /api/worker/task-runs/{task_run_id}", h.runScopedWorkerMiddleware(http.HandlerFunc(h.patchTaskRun)))
	mux.Handle("POST /api/worker/task-runs/{task_run_id}/stream", h.runScopedWorkerMiddleware(http.HandlerFunc(h.postStream)))
	mux.Handle("POST /api/worker/task-runs/{task_run_id}/artifacts", h.runScopedWorkerMiddleware(http.HandlerFunc(h.postArtifact)))
	// Inference authenticates the same way but reads the claims itself: it
	// attributes the call to the token's user and team rather than only
	// admitting it.
	mux.HandleFunc("POST /api/worker/task-runs/{task_run_id}/llm/completions", h.workerLLMCompletionsHandler)
	// The bytes of a release this run is pinned to. Scoped like everything else
	// here, and to the run's own pins besides.
	mux.Handle("GET /api/worker/task-runs/{task_run_id}/plugins/{plugin_name}/{version}/download",
		h.runScopedWorkerMiddleware(http.HandlerFunc(h.downloadPluginPackage)))
}

func (h *Handler) announcer() *runterminal.Announcer {
	return &runterminal.Announcer{Runs: h.cfg.TaskRuns, Hub: h.cfg.Hub, On: h.cfg.OnTerminal, Group: h.cfg.TerminalGroup}
}
