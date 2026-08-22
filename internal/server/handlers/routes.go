package handlers

import (
	"net/http"
)

// Register adds all routes to mux: auth (unauthenticated), user API (JWT), worker API (worker token), inbound webhook.
func (h *Handler) Register(mux *http.ServeMux) {
	// Establishing a session lives in its own package: those routes run before a
	// caller has one, and its Config holds no team store, so nothing there can
	// decide what a session may reach.
	h.authHandler().Register(mux)

	// What a team owns -- membership, agents, webhook keys, usage, audit trail --
	// lives in its own package, holding exactly the stores those routes read.
	h.teamHandler().Register(mux)

	// The work surface -- issues, workflows, tasks, conversations, and the files
	// and traces their runs leave behind -- is one package because those
	// entities are one story, not four that happen to sit together.
	h.workHandler().Register(mux)

	// Artifacts are their own object with their own authorization shape: a
	// route addressed by ar_ ID takes the team from the record, not the path.
	// See docs/design/unified-artifacts.md.
	h.artifactHandler().Register(mux)

	// The plugin catalog is deployment-scoped and readable by any active
	// account: browsing changes nothing, and a release only takes effect when
	// somebody installs it deliberately. Publishing is the administrator's
	// half and lives in the admin package.
	mux.HandleFunc("GET /api/plugins", h.listPluginsHandler)
	mux.HandleFunc("GET /api/plugins/{plugin_name}", h.getPluginHandler)
	mux.HandleFunc("GET /api/plugins/{plugin_name}/releases/{version}/download", h.downloadPluginReleaseHandler)

	// Managed LLM gateway
	mux.HandleFunc("GET /api/teams/{team_id}/llm/models", h.listLLMModelsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/llm/completions", h.llmCompletionsHandler)

	// WebSocket
	mux.HandleFunc("GET /api/teams/{team_id}/ws", h.wsUpgradeHandler)

	// System administration lives in its own package: every route there requires
	// a system_admin grant and none is team-scoped, so it holds a Config that
	// cannot reach a team's data at all.
	h.adminHandler().Register(mux)

	// The worker API lives in its own package: its routes authenticate with a
	// run token rather than a user's session, and a Config that holds neither
	// user store nor team store cannot be talked into honouring one.
	h.workerHandler().Register(mux)

	// Inbound webhook
	mux.HandleFunc("POST /api/webhook", h.serveWebhook)
}
