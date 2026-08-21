package handlers

import (
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

// systemRoleAdmin keeps admin_system.go from importing the model package for
// one constant.
func systemRoleAdmin() string { return model.SystemRoleAdmin }

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
