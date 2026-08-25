package admin

import (
	"net/http"

	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/llmcatalog"
)

// AdminModel is one catalog entry as an administrator sees it.
//
// coregw.Model carries no credential by construction — the key lives in the
// same table but leaves the store only through LLMModelCredential — so this
// embeds it rather than copying field by field.
//
// Nothing is added: every enabled model is callable by every user, so a row's
// name and enabled state are the whole answer to "can this be used".
type AdminModel struct {
	coregw.Model
}

// AdminModelsResponse is the managed catalog.
type AdminModelsResponse struct {
	Models []AdminModel `json:"models"`
	// DefaultModel is the model name a caller gets when it names none. Empty
	// means llm.default_model was not configured and the first enabled model
	// serves as the default.
	DefaultModel string `json:"default_model,omitempty"`
}

// listAdminModelsHandler serves GET /api/admin/llm/models.
func (h *Handler) listAdminModelsHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.guard().SystemAdmin(w, r); !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Models, "the model catalog is not configured") {
		return
	}
	models, err := h.cfg.Models.ListLLMModels(r.Context())
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_list_models")
		return
	}
	out := make([]AdminModel, 0, len(models))
	for _, m := range models {
		out = append(out, AdminModel{Model: m})
	}
	httputil.WriteJSON(w, http.StatusOK, AdminModelsResponse{
		Models:       out,
		DefaultModel: h.cfg.Deployment.DefaultModel,
	})
}

// setAdminModelEnabledHandler serves the enable and disable routes.
//
// Enable and disable are the operational half of the catalog and carry no
// secret, so they are here. Adding a model is not: `buildmax-server model add`
// stays the only way to put an API key into the catalog, because doing it over
// HTTP means a provider credential in a request body, in a proxy log, and in
// whatever the browser did with the form.
//
// What a change records is service/llmcatalog's; this decides only that the
// caller is a person and says which one.
func (h *Handler) setAdminModelEnabledHandler(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actorID, ok := h.guard().SystemAdmin(w, r)
		if !ok {
			return
		}
		if !httputil.RequireStore(w, h.cfg.Models, "the model catalog is not configured") {
			return
		}
		modelID, ok := httputil.PathValue(w, r, "model_id")
		if !ok {
			return
		}
		svc := &llmcatalog.Service{Models: h.cfg.Models, Audit: h.cfg.Audit}
		updated, err := svc.SetEnabled(r.Context(), modelID, enabled, llmcatalog.UserActor(actorID))
		if err != nil {
			if httputil.WriteServiceError(w, err) {
				return
			}
			httputil.WriteInternalError(w, err, "handler error", "handler", "admin_set_model_enabled", "model_id", modelID)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, AdminModel{Model: *updated})
	}
}
