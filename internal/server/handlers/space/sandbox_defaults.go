package space

import (
	"net/http"

	coreaudit "github.com/icloudbb/buildmax/internal/core/audit"
	"github.com/icloudbb/buildmax/internal/server/httputil"
	spacesvc "github.com/icloudbb/buildmax/internal/service/space"
)

// sandboxDefaultsResponse is also the PUT request body: a space's default
// sandbox tiers are the whole resource, so GET and PUT share one shape the
// way plugin curation's SetCurationRequest does.
type sandboxDefaultsResponse struct {
	NetworkTier    string `json:"sandbox_network_tier,omitempty"`
	FilesystemTier string `json:"sandbox_filesystem_tier,omitempty"`
}

func (h *Handler) getSandboxDefaultsHandler(w http.ResponseWriter, r *http.Request) {
	userID, spaceID, ok := h.guard().UserAndPathSpace(w, r, h.cfg.Spaces, "spaces not configured")
	if !ok {
		return
	}
	space, err := h.cfg.Spaces.GetSpace(r.Context(), spaceID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_sandbox_defaults", "user_id", userID, "space_id", spaceID)
		return
	}
	if space == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "space not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sandboxDefaultsResponse{
		NetworkTier:    space.DefaultSandboxNetworkTier,
		FilesystemTier: space.DefaultSandboxFilesystemTier,
	})
}

func (h *Handler) setSandboxDefaultsHandler(w http.ResponseWriter, r *http.Request) {
	userID, spaceID, ok := h.guard().UserAndPathSpace(w, r, h.cfg.Spaces, "spaces not configured")
	if !ok {
		return
	}
	var req sandboxDefaultsResponse
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	err := h.spaceService().SetSandboxDefaults(r.Context(), spacesvc.SetSandboxDefaultsCmd{
		SpaceID:        spaceID,
		ActorID:        userID,
		NetworkTier:    req.NetworkTier,
		FilesystemTier: req.FilesystemTier,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "set_sandbox_defaults", "user_id", userID, "space_id", spaceID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, spaceID, coreaudit.SpaceSandboxDefaultsSet, "space", spaceID,
		req.NetworkTier+"/"+req.FilesystemTier)
	httputil.WriteJSON(w, http.StatusOK, req)
}
