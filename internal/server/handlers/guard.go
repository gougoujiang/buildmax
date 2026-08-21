package handlers

import (
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	"log/slog"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/access"
)

// guard answers who is calling and whether they may proceed.
//
// Built per call from the stores this handler holds, so it stays correct if a
// store is swapped in a test after construction.
func (h *Handler) guard() *access.Guard {
	return &access.Guard{
		JWTSecret: h.cfg.JWTSecret,
		Users:     h.cfg.UserStore,
		Teams:     h.cfg.TeamStore,
		Grants:    h.cfg.SystemGrantStore,
		Audit:     h.cfg.Audit,
	}
}

// refuseDisabled answers a disabled account and reports whether the caller may
// continue.
//
// The log line names which login path refused, which the guard cannot know.
func (h *Handler) refuseDisabled(w http.ResponseWriter, r *http.Request, user *model.User, handler string) bool {
	if user != nil && user.Disabled() {
		slog.InfoContext(r.Context(), "refused a disabled account",
			"handler", handler, "user_id", user.UserID, "remote", r.RemoteAddr)
	}
	return h.guard().RejectDisabled(w, user)
}

// adminHandler builds the deployment-scoped surface from the fields it needs.
//
// Constructed here rather than injected so internal/server keeps assembling one
// Config; what changed is that administration can only see this slice of it.
func (h *Handler) adminHandler() *admin.Handler {
	return admin.New(admin.Config{
		JWTSecret:        h.cfg.JWTSecret,
		DefaultQuotaTier: h.cfg.DefaultQuotaTier,
		Users:            h.cfg.UserStore,
		LoginCodes:       h.cfg.LoginCodeStore,
		RefreshTokens:    h.cfg.RefreshTokenStore,
		Teams:            h.cfg.TeamStore,
		Grants:           h.cfg.SystemGrantStore,
		Audits:           h.cfg.AuditStore,
		Models:           h.cfg.LLMModelStore,
		Schema:           h.cfg.SchemaStore,
		TaskRuns:         h.cfg.TaskRunStore,
		Quota:            h.cfg.QuotaService,
		Audit:            h.cfg.Audit,
		Deployment:       h.cfg.Deployment,
		DependencyProbes: h.cfg.DependencyProbes,
		RedactedConfig:   h.cfg.RedactedConfig,
	})
}
