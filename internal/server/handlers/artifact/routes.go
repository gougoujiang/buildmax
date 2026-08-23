package artifact

import (
	"errors"
	"net/http"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
)

const notFoundMessage = "artifact not found"

// ArtifactResponse is one artifact as the API presents it.
//
// Built by hand rather than serialized from the model so that adding a column
// cannot publish it by accident — the storage key in particular must never
// leave the server.
type ArtifactResponse struct {
	ID            string `json:"id"`
	TeamID        string `json:"team_id"`
	Filename      string `json:"filename"`
	MediaType     string `json:"media_type"`
	SizeBytes     int64  `json:"size_bytes"`
	SHA256        string `json:"sha256"`
	Title         string `json:"title,omitempty"`
	CreatedByType string `json:"created_by_type"`
	CreatedByID   string `json:"created_by_id,omitempty"`
	SourceType    string `json:"source_type"`
	SourceID      string `json:"source_id,omitempty"`
	// Inline reports whether this deployment will render the content in a
	// browser. The client uses it to decide between a preview and a download
	// action instead of guessing from the media type itself.
	Inline    bool       `json:"inline"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type artifactListResponse struct {
	Items []ArtifactResponse `json:"items"`
	Total int                `json:"total"`
}

// ToResponse builds the wire form by hand. Exported for the worker API, which
// answers with the same shape.
func ToResponse(a *model.Artifact) ArtifactResponse {
	out := ArtifactResponse{
		ID:            a.ID,
		TeamID:        a.TeamID,
		Filename:      a.Filename,
		MediaType:     a.MediaType,
		SizeBytes:     a.SizeBytes,
		SHA256:        a.SHA256,
		Title:         a.Title,
		CreatedByType: a.CreatedByType,
		CreatedByID:   a.CreatedByID,
		SourceType:    a.SourceType,
		SourceID:      a.SourceID,
		Inline:        inlineAllowed(a.MediaType),
		CreatedAt:     a.CreatedAt,
	}
	out.ExpiresAt = a.ExpiresAt
	return out
}

// service reports the capability, refusing when the deployment has none.
//
// It is never the first check on a route. Answering an unauthenticated caller
// with "artifacts not configured" would tell them something about the
// deployment before they have proved anything about themselves.
func (h *Handler) service(w http.ResponseWriter) (*artifactsvc.Service, bool) {
	if h.cfg.Artifacts == nil || !h.cfg.Artifacts.Available() {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "artifacts not configured")
		return nil, false
	}
	return h.cfg.Artifacts, true
}

// teamCaller is the preamble of the two team-scoped routes: the caller is
// active and in the team the path names, and only then is the capability
// reported.
func (h *Handler) teamCaller(w http.ResponseWriter, r *http.Request) (userID, teamID string, svc *artifactsvc.Service, ok bool) {
	userID, teamID, ok = h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return "", "", nil, false
	}
	svc, ok = h.service(w)
	if !ok {
		return "", "", nil, false
	}
	return userID, teamID, svc, true
}

// resolve finds the artifact an ID names and authorizes the caller against the
// team the record says owns it.
//
// Absent, tombstoned, and not-yours are answered identically on purpose: the
// three are the same fact to anyone who should not have it.
func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) (*model.Artifact, string, bool) {
	userID, ok := h.guard().ActiveUser(w, r)
	if !ok {
		return nil, "", false
	}
	svc, ok := h.service(w)
	if !ok {
		return nil, "", false
	}
	artifactID, ok := httputil.PathValue(w, r, "artifact_id")
	if !ok {
		return nil, "", false
	}
	rec, err := svc.Get(r.Context(), artifactID)
	if err != nil {
		if errors.Is(err, artifactsvc.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, notFoundMessage)
			return nil, "", false
		}
		if httputil.WriteServiceError(w, err) {
			return nil, "", false
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "artifact", "artifact_id", artifactID)
		return nil, "", false
	}
	if !h.guard().MemberOfResourceTeam(w, r, userID, rec.TeamID, notFoundMessage) {
		return nil, "", false
	}
	return rec, userID, true
}

func (h *Handler) listArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, svc, ok := h.teamCaller(w, r)
	if !ok {
		return
	}
	limit, offset := httputil.LimitOffset(r.URL.Query(), "limit", "offset", 50, 200)
	items, total, err := svc.List(r.Context(), teamID, limit, offset)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_artifacts", "team_id", teamID)
		return
	}
	out := make([]ArtifactResponse, len(items))
	for i := range items {
		out[i] = ToResponse(&items[i])
	}
	httputil.WriteJSON(w, http.StatusOK, artifactListResponse{Items: out, Total: total})
}

func (h *Handler) getArtifactHandler(w http.ResponseWriter, r *http.Request) {
	rec, _, ok := h.resolve(w, r)
	if !ok {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, ToResponse(rec))
}

func (h *Handler) deleteArtifactHandler(w http.ResponseWriter, r *http.Request) {
	rec, userID, ok := h.resolve(w, r)
	if !ok {
		return
	}
	svc, ok := h.service(w)
	if !ok {
		return
	}
	role, ok := h.guard().TeamRole(w, r, userID, rec.TeamID)
	if !ok {
		return
	}
	if !mayDelete(role, userID, rec) {
		httputil.WriteJSONError(w, http.StatusForbidden, "not allowed to delete this artifact")
		return
	}
	if err := svc.Delete(r.Context(), rec, model.ArtifactCreatorUser, userID); err != nil {
		if errors.Is(err, artifactsvc.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, notFoundMessage)
			return
		}
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "delete_artifact", "artifact_id", rec.ID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mayDelete implements the first-slice policy: anyone may remove what they
// uploaded themselves, and an admin or owner may remove anything the team
// holds. A member cannot delete a colleague's file, and cannot delete what a
// run produced, because neither is theirs to withdraw.
func mayDelete(role, userID string, rec *model.Artifact) bool {
	if role == model.TeamRoleAdmin || role == model.TeamRoleOwner {
		return true
	}
	return rec.CreatedByType == model.ArtifactCreatorUser && rec.CreatedByID == userID && userID != ""
}
