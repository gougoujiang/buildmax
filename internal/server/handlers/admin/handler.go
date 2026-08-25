// Package admin serves the deployment-scoped routes.
//
// It is a package rather than a file group because its boundary is real: every
// route here requires a system_admin grant and none is team-scoped. Keeping it
// beside the team-scoped routes meant one Handler could reach every store, so
// nothing but review stopped an admin route from growing a team's data or a
// team route from consulting a grant. This Config names what administration
// needs and nothing else.
package admin

import (
	"net/http"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/server/access"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
	"github.com/gougoujiang/buildmax/internal/service/quota"
)

type Config struct {
	JWTSecret        string
	DefaultQuotaTier string

	Users         model.UserStore
	LoginCodes    model.LoginCodeStore
	RefreshTokens model.RefreshTokenStore
	Teams         coreteam.Store
	Grants        model.SystemGrantStore
	Audits        coreaudit.Store
	Models        coregw.ModelStore
	Schema        model.SchemaStore
	TaskRuns      model.TaskRunStore

	Quota *quota.Service
	// Plugins publishes releases and manages catalog entries. Nil is a
	// deployment with no Marketplace, which every route here reports rather
	// than pretending an empty catalog.
	Plugins *pluginsvc.Service
	// Audit records who did what. Nil discards it, which is what a deployment
	// without a database has.
	Audit *audit.Recorder

	Deployment       DeploymentInfo
	DependencyProbes []DependencyProbe
	// RedactedConfig is the operator-facing view of server.yaml, built by
	// internal/config so the decision about which fields may be shown lives
	// next to the struct.
	RedactedConfig any
}

type Handler struct{ cfg Config }

func New(cfg Config) *Handler { return &Handler{cfg: cfg} }

func (h *Handler) guard() *access.Guard {
	return &access.Guard{
		JWTSecret: h.cfg.JWTSecret,
		Users:     h.cfg.Users,
		Teams:     h.cfg.Teams,
		Grants:    h.cfg.Grants,
		Audit:     h.cfg.Audit,
	}
}

// Register adds the deployment-scoped routes.
//
// None takes a {team_id}: an admin route that looked team-scoped would invite
// exactly the confusion the boundary exists to prevent. See
// docs/design/system-administration.md.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/me", h.adminMeHandler)
	mux.HandleFunc("GET /api/admin/grants", h.listAdminGrantsHandler)
	mux.HandleFunc("POST /api/admin/grants", h.createAdminGrantHandler)
	mux.HandleFunc("DELETE /api/admin/grants/{user_id}", h.deleteAdminGrantHandler)
	mux.HandleFunc("GET /api/admin/users", h.listAdminUsersHandler)
	mux.HandleFunc("POST /api/admin/users", h.createAdminUserHandler)
	mux.HandleFunc("GET /api/admin/users/{user_id}", h.getAdminUserHandler)
	mux.HandleFunc("POST /api/admin/users/{user_id}/login-code", h.issueAdminLoginCodeHandler)
	mux.HandleFunc("POST /api/admin/users/{user_id}/disable", h.setAdminUserDisabledHandler(true))
	mux.HandleFunc("POST /api/admin/users/{user_id}/enable", h.setAdminUserDisabledHandler(false))
	mux.HandleFunc("DELETE /api/admin/users/{user_id}/sessions", h.revokeAdminUserSessionsHandler)
	mux.HandleFunc("GET /api/admin/system", h.adminSystemHandler)
	mux.HandleFunc("GET /api/admin/config", h.adminConfigHandler)
	mux.HandleFunc("GET /api/admin/audit-events", h.listAdminAuditEventsHandler)
	mux.HandleFunc("GET /api/admin/audit-events/export", h.exportAdminAuditEventsHandler)
	mux.HandleFunc("GET /api/admin/teams", h.listAdminTeamsHandler)
	mux.HandleFunc("GET /api/admin/teams/{team_id}", h.getAdminTeamHandler)
	mux.HandleFunc("GET /api/admin/llm/models", h.listAdminModelsHandler)
	mux.HandleFunc("POST /api/admin/llm/models/{model_id}/enable", h.setAdminModelEnabledHandler(true))
	mux.HandleFunc("POST /api/admin/llm/models/{model_id}/disable", h.setAdminModelEnabledHandler(false))
	mux.HandleFunc("GET /api/admin/plugins", h.listAdminPluginsHandler)
	mux.HandleFunc("POST /api/admin/plugins", h.createAdminPluginHandler)
	mux.HandleFunc("GET /api/admin/plugins/{plugin_name}/releases", h.listAdminPluginReleasesHandler)
	mux.HandleFunc("POST /api/admin/plugins/{plugin_name}/releases", h.publishAdminPluginReleaseHandler)
	mux.HandleFunc("POST /api/admin/plugins/{plugin_name}/releases/{version}/yank", h.yankAdminPluginReleaseHandler)
	mux.HandleFunc("POST /api/admin/plugins/{plugin_name}/archive", h.setAdminPluginArchivedHandler(true))
	mux.HandleFunc("POST /api/admin/plugins/{plugin_name}/unarchive", h.setAdminPluginArchivedHandler(false))
}

// systemRoleAdmin keeps admin_system.go from importing the model package for
// one constant.
func systemRoleAdmin() string { return model.SystemRoleAdmin }
