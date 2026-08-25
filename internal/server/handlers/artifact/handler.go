// Package artifact serves the durable files a team keeps.
//
// It is its own package because an artifact is its own object, and because its
// authorization is a different shape from every other route here: an artifact
// is addressed by its ar_ ID, and the team comes from the record rather than
// from the path. Folding it into the work surface would put that different
// shape next to routes that all take their team from the URL, which is the
// mistake most likely to be copied. See docs/design/unified-artifacts.md.
package artifact

import (
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/server/access"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"github.com/gougoujiang/buildmax/internal/service/audit"
)

type Config struct {
	JWTSecret string

	Users model.UserStore
	Teams coreteam.Store

	// Artifacts is the capability itself. Nil means this deployment has no
	// artifact store, and every route here answers 503.
	Artifacts *artifactsvc.Service
	Audit     *audit.Recorder
}

type Handler struct{ cfg Config }

func New(cfg Config) *Handler { return &Handler{cfg: cfg} }

func (h *Handler) guard() *access.Guard {
	return &access.Guard{
		JWTSecret: h.cfg.JWTSecret,
		Users:     h.cfg.Users,
		Teams:     h.cfg.Teams,
		Audit:     h.cfg.Audit,
	}
}

// Register adds the artifact routes.
//
// The split is deliberate: an artifact is reached by its ID, and the
// team-scoped route is the team's listing and upload surface, not a second
// address for one artifact.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/teams/{team_id}/artifacts", h.listArtifactsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/artifacts", h.uploadArtifactHandler)
	// The same creation, for a client that has a login but has not chosen a
	// team: an optional ?team_id= is honoured, and no team means the caller's
	// personal one. CLI and Desktop reach artifacts this way.
	mux.HandleFunc("POST /api/artifacts", h.uploadToDefaultTeamHandler)
	mux.HandleFunc("GET /api/artifacts/{artifact_id}", h.getArtifactHandler)
	mux.HandleFunc("GET /api/artifacts/{artifact_id}/content", h.artifactContentHandler)
	mux.HandleFunc("DELETE /api/artifacts/{artifact_id}", h.deleteArtifactHandler)
}
