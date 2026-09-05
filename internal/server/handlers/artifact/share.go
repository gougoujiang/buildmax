package artifact

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
)

// shareResponse is one link as the management API presents it. The token and
// the URLs are set only on creation — a hashed token cannot be reconstructed,
// so a later listing shows the link's metadata but never the link itself, the
// same as an API key shown once.
type shareResponse struct {
	ShareID         string     `json:"share_id"`
	ArtifactID      string     `json:"artifact_id"`
	URL             string     `json:"url,omitempty"`
	DownloadURL     string     `json:"download_url,omitempty"`
	Token           string     `json:"token,omitempty"`
	CreatedByType   string     `json:"created_by_type"`
	CreatedByID     string     `json:"created_by_id,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	RetrievalCount  int64      `json:"retrieval_count"`
	LastRetrievedAt *time.Time `json:"last_retrieved_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func toShareResponse(rec coreartifact.ArtifactShare) shareResponse {
	return shareResponse{
		ShareID:         rec.ShareID,
		ArtifactID:      rec.ArtifactID,
		CreatedByType:   rec.CreatedByType,
		CreatedByID:     rec.CreatedByID,
		ExpiresAt:       rec.ExpiresAt,
		RevokedAt:       rec.RevokedAt,
		RetrievalCount:  rec.RetrievalCount,
		LastRetrievedAt: rec.LastRetrievedAt,
		CreatedAt:       rec.CreatedAt,
	}
}

// sharedMetaResponse is the public metadata the preview page needs. It is a
// narrow subset: enough to render and to name a download, and nothing about the
// team, the producer, or the storage.
type sharedMetaResponse struct {
	Filename  string    `json:"filename"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	Title     string    `json:"title,omitempty"`
	Preview   string    `json:"preview"`
	CreatedAt time.Time `json:"created_at"`
}

// createShareHandler mints a public link for an artifact the caller can read.
// Any member may create one (§10); resolve already proved membership.
func (h *Handler) createShareHandler(w http.ResponseWriter, r *http.Request) {
	rec, userID, ok := h.resolve(w, r)
	if !ok {
		return
	}
	svc, ok := h.service(w)
	if !ok {
		return
	}
	share, err := svc.CreateShare(r.Context(), artifactsvc.CreateShareInput{
		ArtifactID:    rec.ID,
		TTL:           shareTTLFromRequest(r),
		CreatedByType: coreartifact.CreatorUser,
		CreatedByID:   userID,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_artifact_share", "artifact_id", rec.ID)
		return
	}
	out := toShareResponse(share.Record)
	out.Token = share.Token
	out.URL = share.PageURL
	out.DownloadURL = share.DownloadURL
	httputil.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) listSharesHandler(w http.ResponseWriter, r *http.Request) {
	rec, _, ok := h.resolve(w, r)
	if !ok {
		return
	}
	svc, ok := h.service(w)
	if !ok {
		return
	}
	shares, err := svc.ListShares(r.Context(), rec.ID)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_artifact_shares", "artifact_id", rec.ID)
		return
	}
	out := make([]shareResponse, len(shares))
	for i := range shares {
		out[i] = toShareResponse(shares[i])
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) revokeShareHandler(w http.ResponseWriter, r *http.Request) {
	rec, userID, ok := h.resolve(w, r)
	if !ok {
		return
	}
	svc, ok := h.service(w)
	if !ok {
		return
	}
	shareID, ok := httputil.PathValue(w, r, "share_id")
	if !ok {
		return
	}
	role, ok := h.guard().TeamRole(w, r, userID, rec.TeamID)
	if !ok {
		return
	}
	if !h.mayRevokeShare(r, svc, rec, shareID, role, userID, w) {
		return
	}
	if err := svc.RevokeShare(r.Context(), rec, shareID, coreartifact.CreatorUser, userID); err != nil {
		if errors.Is(err, artifactsvc.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "share not found")
			return
		}
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "revoke_artifact_share", "artifact_id", rec.ID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mayRevokeShare implements §10: an admin or owner revokes any link; a member
// revokes only one they created. Ownership is read from the link itself, which
// is why a member's revoke costs a listing lookup.
func (h *Handler) mayRevokeShare(r *http.Request, svc *artifactsvc.Service, rec *coreartifact.Artifact, shareID, role, userID string, w http.ResponseWriter) bool {
	if role == coreteam.RoleAdmin || role == coreteam.RoleOwner {
		return true
	}
	shares, err := svc.ListShares(r.Context(), rec.ID)
	if err != nil {
		if !httputil.WriteServiceError(w, err) {
			httputil.WriteInternalError(w, err, "handler error", "handler", "revoke_artifact_share", "artifact_id", rec.ID)
		}
		return false
	}
	for i := range shares {
		if shares[i].ShareID != shareID {
			continue
		}
		if shares[i].CreatedByType == coreartifact.CreatorUser && shares[i].CreatedByID == userID && userID != "" {
			return true
		}
		httputil.WriteJSONError(w, http.StatusForbidden, "not allowed to revoke this share")
		return false
	}
	// A member asking about a share that is not there gets the same 404 the
	// revoke itself would give, rather than a role verdict on a nonexistent link.
	httputil.WriteJSONError(w, http.StatusNotFound, "share not found")
	return false
}

// sharedMetaHandler answers the public preview page's metadata request. No
// session: the token is the whole authorization.
func (h *Handler) sharedMetaHandler(w http.ResponseWriter, r *http.Request) {
	resolved, _, ok := h.resolveShared(w, r)
	if !ok {
		return
	}
	art := resolved.Artifact
	httputil.WriteJSON(w, http.StatusOK, sharedMetaResponse{
		Filename:  art.Filename,
		MediaType: art.MediaType,
		SizeBytes: art.SizeBytes,
		Title:     art.Title,
		Preview:   string(previewModeFor(art.MediaType)),
		CreatedAt: art.CreatedAt,
	})
}

// sharedContentHandler streams a shared artifact's bytes, then counts the
// retrieval. It reuses the same header logic as the authenticated route, so an
// HTML artifact reaches a public viewer under the same opaque-origin sandbox.
func (h *Handler) sharedContentHandler(w http.ResponseWriter, r *http.Request) {
	resolved, svc, ok := h.resolveShared(w, r)
	if !ok {
		return
	}
	art := resolved.Artifact
	body, err := svc.Open(r.Context(), &art)
	if err != nil {
		if errors.Is(err, artifactsvc.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, notFoundMessage)
			return
		}
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "shared_artifact_content", "artifact_id", art.ID)
		return
	}
	defer func() { _ = body.Close() }()

	// Counted before streaming: the bytes may take a while, and a client that
	// disconnects mid-download still asked for the file.
	svc.RecordShareRetrieval(r.Context(), resolved.Share.ShareID)

	writeContentHeaders(w, &art, forceDownload(r))
	if _, err := io.Copy(w, body); err != nil {
		slog.Warn("shared artifact content stream ended early", "err", err, "artifact_id", art.ID)
	}
}

// resolveShared turns the {token} path value into an artifact, answering 404 for
// any token a public caller must not tell apart from a bad one.
func (h *Handler) resolveShared(w http.ResponseWriter, r *http.Request) (*coreartifact.ResolvedShare, *artifactsvc.Service, bool) {
	svc, ok := h.service(w)
	if !ok {
		return nil, nil, false
	}
	token, ok := httputil.PathValue(w, r, "token")
	if !ok {
		return nil, nil, false
	}
	resolved, err := svc.ResolveShare(r.Context(), token)
	if err != nil {
		if errors.Is(err, artifactsvc.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, notFoundMessage)
			return nil, nil, false
		}
		if httputil.WriteServiceError(w, err) {
			return nil, nil, false
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "resolve_artifact_share")
		return nil, nil, false
	}
	return resolved, svc, true
}

// shareTTLFromRequest reads an optional ttl_hours override. The service clamps
// it to the deployment bound, so an unparseable or oversized value is harmless.
func shareTTLFromRequest(r *http.Request) time.Duration {
	hours, err := strconv.Atoi(r.URL.Query().Get("ttl_hours"))
	if err != nil || hours <= 0 {
		return 0
	}
	return time.Duration(hours) * time.Hour
}
