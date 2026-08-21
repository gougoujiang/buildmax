package admin

import (
	"net/http"
	"sort"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// AdminModel is one catalog entry as an administrator sees it.
//
// model.LLMModel carries no credential by construction — the key lives in the
// same table but leaves the store only through LLMModelCredential — so this
// embeds it rather than copying field by field. What is added is the operator's
// actual question: whether any team can call this model.
type AdminModel struct {
	model.LLMModel
	// Aliases are the deployment aliases pointing at this model. A model with
	// none is not reachable by any team however enabled it is, which is the
	// most common reason an operator's model "does not work".
	Aliases []string `json:"aliases"`
}

// AdminModelsResponse is the managed catalog.
type AdminModelsResponse struct {
	Models []AdminModel `json:"models"`
	// DefaultAlias is the alias a managed caller gets when it names none.
	DefaultAlias string `json:"default_alias,omitempty"`
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
		row := AdminModel{LLMModel: m, Aliases: []string{}}
		for alias, target := range h.cfg.Deployment.ModelAliases {
			if target == m.LLMModelID {
				row.Aliases = append(row.Aliases, alias)
			}
		}
		sort.Strings(row.Aliases)
		out = append(out, row)
	}
	httputil.WriteJSON(w, http.StatusOK, AdminModelsResponse{
		Models:       out,
		DefaultAlias: h.cfg.Deployment.DefaultModelAlias,
	})
}

// setAdminModelEnabledHandler serves the enable and disable routes.
//
// Enable and disable are the operational half of the catalog and carry no
// secret, so they are here. Adding a model is not: `buildmax-server model add`
// stays the only way to put an API key into the catalog, because doing it over
// HTTP means a provider credential in a request body, in a proxy log, and in
// whatever the browser did with the form.
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
		existing, err := h.cfg.Models.GetLLMModel(r.Context(), modelID)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "admin_set_model_enabled", "model_id", modelID)
			return
		}
		if existing == nil {
			httputil.WriteJSONError(w, http.StatusNotFound, "model not found")
			return
		}
		if err := h.cfg.Models.SetLLMModelEnabled(r.Context(), modelID, enabled); err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "admin_set_model_enabled", "model_id", modelID)
			return
		}

		// The same actions the operator command writes, so the trail does not
		// distinguish a catalog change by where it was made — only by who made
		// it. A command names the binary; this names a person.
		action := model.AuditModelDisabled
		if enabled {
			action = model.AuditModelEnabled
		}
		h.cfg.Audit.Record(r.Context(), model.AuditEvent{
			ActorType:  model.AuditActorUser,
			ActorID:    actorID,
			Action:     action,
			TargetType: "llm_model",
			TargetID:   modelID,
			Detail:     existing.Name,
		})

		updated, err := h.cfg.Models.GetLLMModel(r.Context(), modelID)
		if err != nil || updated == nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "admin_set_model_enabled", "reload")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, AdminModel{LLMModel: *updated, Aliases: []string{}})
	}
}
