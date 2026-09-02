package team

import (
	"net/http"
	"time"

	coresecret "github.com/gougoujiang/buildmax/internal/core/secret"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	secretsvc "github.com/gougoujiang/buildmax/internal/service/secret"
)

// secretResponse is a Secret's metadata. It never carries an item value:
// item_names lists the keys present, and there is no reveal route.
type secretResponse struct {
	ID          string    `json:"id"`
	TeamID      string    `json:"team_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Provider    string    `json:"provider"`
	State       string    `json:"state"`
	ItemNames   []string  `json:"item_names"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type secretListResponse struct {
	Secrets []secretResponse `json:"secrets"`
}

type createSecretRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Items       map[string]string `json:"items"`
}

// editSecretRequest carries one of two shapes over the same route. Items,
// when present, replaces the whole map -- what a raw-JSON editor sends. Set and
// Remove patch named keys -- what a row editor sends. Sending both Items and a
// patch is rejected.
type editSecretRequest struct {
	Items  map[string]string `json:"items,omitempty"`
	Set    map[string]string `json:"set,omitempty"`
	Remove []string          `json:"remove,omitempty"`
}

type setSecretStateRequest struct {
	State string `json:"state"`
}

func secretToResponse(s coresecret.Secret) secretResponse {
	names := s.ItemNames
	if names == nil {
		names = []string{}
	}
	return secretResponse{
		ID:          s.ID,
		TeamID:      s.TeamID,
		Name:        s.Name,
		Description: s.Description,
		Provider:    string(s.Provider),
		State:       string(s.State),
		ItemNames:   names,
		CreatedBy:   s.CreatedBy,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

// secretService returns the service, or writes 503 and false when the feature
// is off. Every route calls it first, so a deployment with no KEK file answers
// a clear "not configured" rather than a nil dereference.
func (h *Handler) secretService(w http.ResponseWriter) (*secretsvc.Service, bool) {
	if h.cfg.SecretService == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "secrets not configured")
		return nil, false
	}
	return h.cfg.SecretService, true
}

// authorizeSecrets resolves the caller and team and requires owner. Every
// secret route is owner-only in this slice; see docs/design/team-secrets.md
// §10 and open question §20.
func (h *Handler) authorizeSecrets(w http.ResponseWriter, r *http.Request) (userID, teamID string, ok bool) {
	userID, teamID, ok = h.guard().UserAndPathTeam(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return "", "", false
	}
	if _, ok = h.guard().TeamAction(w, r, userID, teamID, coreteam.ActionManageSecrets); !ok {
		return "", "", false
	}
	return userID, teamID, true
}

func (h *Handler) listSecretsHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.authorizeSecrets(w, r)
	if !ok {
		return
	}
	svc, ok := h.secretService(w)
	if !ok {
		return
	}
	secrets, err := svc.List(r.Context(), teamID)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_secrets", "team_id", teamID)
		return
	}
	out := secretListResponse{Secrets: make([]secretResponse, 0, len(secrets))}
	for _, s := range secrets {
		out.Secrets = append(out.Secrets, secretToResponse(s))
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) getSecretHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.authorizeSecrets(w, r)
	if !ok {
		return
	}
	svc, ok := h.secretService(w)
	if !ok {
		return
	}
	secretID, ok := httputil.PathValue(w, r, "secret_id")
	if !ok {
		return
	}
	sec, err := svc.Get(r.Context(), teamID, secretID)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_secret", "secret_id", secretID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, secretToResponse(*sec))
}

func (h *Handler) createSecretHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.authorizeSecrets(w, r)
	if !ok {
		return
	}
	svc, ok := h.secretService(w)
	if !ok {
		return
	}
	var req createSecretRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	created, err := svc.Create(r.Context(), secretsvc.CreateCmd{
		TeamID:      teamID,
		CreatedBy:   userID,
		Name:        req.Name,
		Description: req.Description,
		Items:       req.Items,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_secret", "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, secretToResponse(*created))
}

// editSecretHandler applies a whole-map replace or a per-item patch. Both edit
// the same Secret through the same route; sending both shapes at once is a bad
// request.
func (h *Handler) editSecretHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.authorizeSecrets(w, r)
	if !ok {
		return
	}
	svc, ok := h.secretService(w)
	if !ok {
		return
	}
	secretID, ok := httputil.PathValue(w, r, "secret_id")
	if !ok {
		return
	}
	var req editSecretRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	hasReplace := req.Items != nil
	hasPatch := len(req.Set) > 0 || len(req.Remove) > 0
	if hasReplace && hasPatch {
		httputil.WriteJSONError(w, http.StatusBadRequest, "send items to replace, or set/remove to patch, not both")
		return
	}
	if !hasReplace && !hasPatch {
		httputil.WriteJSONError(w, http.StatusBadRequest, "no edit: send items, or set/remove")
		return
	}
	var (
		updated *coresecret.Secret
		err     error
	)
	if hasReplace {
		updated, err = svc.ReplaceItems(r.Context(), teamID, secretID, req.Items)
	} else {
		updated, err = svc.PatchItems(r.Context(), teamID, secretID, req.Set, req.Remove)
	}
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "edit_secret", "secret_id", secretID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, secretToResponse(*updated))
}

func (h *Handler) setSecretStateHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.authorizeSecrets(w, r)
	if !ok {
		return
	}
	svc, ok := h.secretService(w)
	if !ok {
		return
	}
	secretID, ok := httputil.PathValue(w, r, "secret_id")
	if !ok {
		return
	}
	var req setSecretStateRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	updated, err := svc.SetState(r.Context(), teamID, secretID, coresecret.State(req.State))
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "set_secret_state", "secret_id", secretID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, secretToResponse(*updated))
}
