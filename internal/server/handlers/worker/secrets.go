package worker

import (
	"context"
	"errors"
	"net/http"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coresecret "github.com/gougoujiang/buildmax/internal/core/secret"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// SecretMaterializer decrypts a team's Secret for a runtime consumer. The
// secret service satisfies it; an interface so the worker API does not depend
// on secret crypto or lifecycle.
type SecretMaterializer interface {
	Materialize(ctx context.Context, teamID, id string) (coresecret.Items, error)
}

// getTaskRunSecrets returns the env grants this run's agent declared, decrypted
// for delivery. The values ride this route alone, never the run bundle, so the
// response is no-store and unlogged.
//
// A required grant that cannot be produced fails the request, which the worker
// turns into a failed run: a run must not proceed without a credential its
// definition declared. An optional one is skipped.
func (h *Handler) getTaskRunSecrets(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if taskRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	// No secret service means the feature is off. The run's agent could not
	// have saved a consumption config in that case, so the answer is an empty
	// grant set, not an error.
	if h.cfg.Secrets == nil || h.cfg.Agents == nil || h.cfg.TaskRuns == nil {
		writeNoStore(w)
		httputil.WriteJSON(w, http.StatusOK, workerclient.TaskRunSecretsResponse{})
		return
	}
	run, task, err := h.cfg.TaskRuns.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "get_worker_task_run_secrets", "task_run_id", taskRunID)
		return
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		return
	}

	cons := h.runConsumption(r.Context(), task)
	if cons.IsEmpty() {
		writeNoStore(w)
		httputil.WriteJSON(w, http.StatusOK, workerclient.TaskRunSecretsResponse{})
		return
	}

	env, err := resolveEnvGrants(r.Context(), h.cfg.Secrets, task.TeamID, cons)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "resolve_secret_grants", "task_run_id", taskRunID)
		return
	}
	writeNoStore(w)
	httputil.WriteJSON(w, http.StatusOK, workerclient.TaskRunSecretsResponse{Env: env})
}

// runConsumption resolves the consumption config of the agent this run named,
// against the run's team. This mirrors how getTaskRun resolves the agent's
// instructions: the current revision is used, which is the revision recorded on
// the run at claim. No agent, or an agent whose team does not match the run's,
// yields no consumption.
func (h *Handler) runConsumption(ctx context.Context, task *coretask.Task) agentdef.SecretConsumption {
	if task.AgentID == nil || *task.AgentID == "" {
		return agentdef.SecretConsumption{}
	}
	a, err := h.cfg.Agents.GetAgentIncludingDeleted(ctx, *task.AgentID)
	if err != nil {
		componentLog().Warn("worker handler: agent consumption unavailable", "task_id", task.ID, "agent_id", *task.AgentID, "err", err)
		return agentdef.SecretConsumption{}
	}
	if a == nil || a.TeamID != task.TeamID {
		return agentdef.SecretConsumption{}
	}
	return a.SecretConsumption
}

func writeNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

// resolveEnvGrants turns a consumption config into resolved environment
// variables. Each grant materializes its Secret against the run's team. A
// required grant that fails is returned as an error; an optional one is
// skipped. Names cannot collide -- the agent service refused a config that
// would, when it was saved.
func resolveEnvGrants(ctx context.Context, mat SecretMaterializer, teamID string, cons agentdef.SecretConsumption) (map[string]string, error) {
	out := map[string]string{}
	for _, g := range cons.Env {
		items, err := mat.Materialize(ctx, teamID, g.Secret)
		if err != nil {
			if g.Optional && isSkippable(err) {
				continue
			}
			return nil, err
		}
		if g.WholeGroup() {
			for name, val := range items {
				out[g.Prefix+name] = val
			}
			continue
		}
		val, ok := items[g.Item]
		if !ok {
			if g.Optional {
				continue
			}
			return nil, apierr.New(apierr.KindInvalid, "secret grant: item "+g.Item+" is gone from secret "+g.Secret)
		}
		out[g.EnvName] = val
	}
	return out, nil
}

// isSkippable reports whether an optional grant may be silently skipped for
// this error: a Secret that is now absent, disabled, or destroyed. Any other
// error (a backend failure) fails the run even for an optional grant, because
// it is not evidence the Secret was withdrawn.
func isSkippable(err error) bool {
	if errors.Is(err, apierr.ErrNotFound) {
		return true
	}
	kind, ok := apierr.KindOf(err)
	if !ok {
		return false
	}
	return kind == apierr.KindNotFound || kind == apierr.KindConflict
}
