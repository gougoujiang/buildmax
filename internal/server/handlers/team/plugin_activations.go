package team

import (
	"errors"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/pluginwire"
	"github.com/gougoujiang/buildmax/internal/server/access"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
)

// Activation routes are team-scoped where the catalog routes are
// deployment-scoped: the catalog says what exists, an activation says what one
// team's background runs may use.
//
// Reading is any member's, because "why did this run have this plugin" is a
// question anyone debugging a run asks. Changing is ActionManageAgents, which
// is the authority a team's other shared automation already needs.

func (h *Handler) requirePluginService(w http.ResponseWriter) bool {
	if h.cfg.Plugins == nil || h.cfg.Plugins.Activations == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "plugin activation is not configured")
		return false
	}
	return true
}

// writePluginActivationError maps the refusals this package's callers can act
// on. Everything else falls through to the caller's internal-error path.
func writePluginActivationError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, pluginsvc.ErrExecutableContent),
		errors.Is(err, pluginsvc.ErrNoActivatableRelease),
		errors.Is(err, pluginsvc.ErrInvalidCuration):
		httputil.WriteJSONError(w, http.StatusUnprocessableEntity, err.Error())
		return true
	case errors.Is(err, model.ErrPluginAlreadyActivated):
		// Already activated is a conflict rather than a failure: moving to
		// another release is the PATCH, and saying so is more use than 500.
		httputil.WriteJSONError(w, http.StatusConflict, err.Error())
		return true
	case errors.Is(err, model.ErrNotFound):
		httputil.WriteJSONError(w, http.StatusNotFound, "no such plugin or release")
		return true
	}
	return httputil.WriteServiceError(w, err)
}

func (h *Handler) listPluginActivationsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	if !h.requirePluginService(w) {
		return
	}
	activations, err := h.cfg.Plugins.ListActivations(r.Context(), teamID)
	if err != nil {
		if writePluginActivationError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_plugin_activations", "user_id", userID, "team_id", teamID)
		return
	}
	team, err := h.cfg.Teams.GetTeam(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_plugin_activations", "user_id", userID, "team_id", teamID)
		return
	}
	if team == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "team not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, pluginwire.ActivationsResponse{
		Curation:    model.NormalizePluginCuration(string(team.PluginCuration)),
		Activations: activations,
	})
}

func (h *Handler) activatePluginHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, access.ActionManageAgents); !ok {
		return
	}
	if !h.requirePluginService(w) {
		return
	}
	var req pluginwire.ActivateRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	if req.PluginName == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "plugin_name is required")
		return
	}
	activated, err := h.cfg.Plugins.Activate(r.Context(), pluginsvc.ActivateInput{
		TeamID:     teamID,
		PluginName: req.PluginName,
		Version:    req.Version,
		ActorID:    userID,
	})
	if err != nil {
		if writePluginActivationError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "activate_plugin", "user_id", userID, "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, activated)
}

func (h *Handler) patchPluginActivationHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, access.ActionManageAgents); !ok {
		return
	}
	if !h.requirePluginService(w) {
		return
	}
	pluginName, ok := httputil.PathValue(w, r, "plugin_name")
	if !ok {
		return
	}
	var req pluginwire.UpdateActivationRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	if req.Version == nil && req.Enabled == nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "name version or enabled: a request that changes neither is not a change")
		return
	}

	// The pin moves first. Suspending and repointing in one request is
	// unusual but well defined, and doing the pin first means a refused move
	// leaves the activation as it was rather than suspended-and-unmoved.
	var current *model.PluginActivation
	if req.Version != nil {
		moved, err := h.cfg.Plugins.MovePin(r.Context(), pluginsvc.ActivateInput{
			TeamID:     teamID,
			PluginName: pluginName,
			Version:    *req.Version,
			ActorID:    userID,
		})
		if err != nil {
			if writePluginActivationError(w, err) {
				return
			}
			httputil.WriteInternalError(w, err, "handler error", "handler", "move_plugin_pin", "user_id", userID, "team_id", teamID)
			return
		}
		current = moved
	}
	if req.Enabled != nil {
		updated, err := h.cfg.Plugins.SetActivationEnabled(r.Context(), teamID, pluginName, *req.Enabled, userID)
		if err != nil {
			if writePluginActivationError(w, err) {
				return
			}
			httputil.WriteInternalError(w, err, "handler error", "handler", "set_plugin_activation_enabled", "user_id", userID, "team_id", teamID)
			return
		}
		current = updated
	}
	httputil.WriteJSON(w, http.StatusOK, current)
}

func (h *Handler) setPluginCurationHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, access.ActionManageAgents); !ok {
		return
	}
	if !h.requirePluginService(w) {
		return
	}
	var req pluginwire.SetCurationRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	if err := h.cfg.Plugins.SetCuration(r.Context(), teamID, req.Curation, userID); err != nil {
		if writePluginActivationError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "set_plugin_curation", "user_id", userID, "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, pluginwire.SetCurationRequest{Curation: req.Curation})
}
