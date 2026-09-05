package worker

import (
	"context"
	"errors"
	"log/slog"
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

// SecretGrantRecorder records the non-secret audit of a materialized grant. The
// db store satisfies it. Nil disables the audit write, which is fail-open: the
// run already got its grant, and a failed audit insert must not fail the run.
type SecretGrantRecorder interface {
	RecordEnvGrant(ctx context.Context, in coresecret.GrantRecord) error
}

// resolvedGrant is one item this run received, ready to deliver and to audit.
type resolvedGrant struct {
	secretID string
	itemName string
	envName  string
	value    string
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
		writeEmptySecrets(w)
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
	// The worker must have claimed the run before it can read a Secret. A run
	// that is not RUNNING has either not been claimed or has finished, and a
	// finished run reading a credential is exactly what a leaked token would do.
	// See docs/design/team-secrets.md §7.
	if !requireRunning(w, run.Status) {
		return
	}

	agentID, revision, cons, ok := h.pinnedConsumption(r.Context(), run, task)
	if !ok || cons.IsEmpty() {
		writeEmptySecrets(w)
		return
	}

	grants, err := resolveEnvGrants(r.Context(), h.cfg.Secrets, task.TeamID, cons)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "resolve_secret_grants", "task_run_id", taskRunID)
		return
	}

	h.recordGrants(r.Context(), taskRunID, agentID, revision, grants)

	env := make(map[string]string, len(grants))
	for _, g := range grants {
		env[g.envName] = g.value
	}
	writeNoStore(w)
	httputil.WriteJSON(w, http.StatusOK, workerclient.TaskRunSecretsResponse{Env: env})
}

// recordGrants writes the audit snapshot of what this run received, against the
// pinned revision that authorized it. It is fail-open: the run already has its
// grants, so a failed insert is logged, not returned. Nil recorder means the
// deployment records nothing here.
func (h *Handler) recordGrants(ctx context.Context, taskRunID, agentID string, revision int, grants []resolvedGrant) {
	if h.cfg.SecretAudit == nil {
		return
	}
	for _, g := range grants {
		err := h.cfg.SecretAudit.RecordEnvGrant(ctx, coresecret.GrantRecord{
			TaskRunID:     taskRunID,
			SecretID:      g.secretID,
			ItemName:      g.itemName,
			AgentID:       agentID,
			AgentRevision: revision,
			EnvName:       g.envName,
		})
		if err != nil {
			slog.Warn("worker handler: could not record a secret materialization",
				"task_run_id", taskRunID, "secret_id", g.secretID, "err", err)
		}
	}
}

// pinnedConsumption resolves the Secret consumption this run is authorized for
// from the Agent revision pinned onto the TaskRun, not the agent's current
// revision. Pinning at claim is what stops a consumption config edited mid-run
// from widening what an in-flight run receives — the whole point of snapshotting
// the authorization. See docs/design/team-secrets.md §6 and §7.
//
// The agent is still resolved once, to confirm it belongs to the run's team: a
// revision carries no team of its own, and the run token names the team the
// agent must be owned by. No agent, a team mismatch, or a run with no pinned
// revision yields ok=false, which the caller treats as no consumption rather
// than falling back to the live config.
func (h *Handler) pinnedConsumption(ctx context.Context, run *coretask.Run, task *coretask.Task) (agentID string, revision int, cons agentdef.SecretConsumption, ok bool) {
	if task.AgentID == nil || *task.AgentID == "" || run.AgentRevision == nil {
		return "", 0, agentdef.SecretConsumption{}, false
	}
	agent, err := h.cfg.Agents.GetAgentIncludingDeleted(ctx, *task.AgentID)
	if err != nil {
		componentLog().Warn("worker handler: agent unavailable", "task_id", task.ID, "agent_id", *task.AgentID, "err", err)
		return "", 0, agentdef.SecretConsumption{}, false
	}
	if agent == nil || agent.TeamID != task.TeamID {
		return "", 0, agentdef.SecretConsumption{}, false
	}
	rev, err := h.cfg.Agents.GetAgentRevision(ctx, *task.AgentID, *run.AgentRevision)
	if err != nil {
		componentLog().Warn("worker handler: pinned agent revision unavailable",
			"task_id", task.ID, "agent_id", *task.AgentID, "revision", *run.AgentRevision, "err", err)
		return "", 0, agentdef.SecretConsumption{}, false
	}
	if rev == nil {
		componentLog().Warn("worker handler: pinned agent revision is gone",
			"task_id", task.ID, "agent_id", *task.AgentID, "revision", *run.AgentRevision)
		return "", 0, agentdef.SecretConsumption{}, false
	}
	return agent.ID, *run.AgentRevision, rev.SecretConsumption, true
}

func writeNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func writeEmptySecrets(w http.ResponseWriter) {
	writeNoStore(w)
	httputil.WriteJSON(w, http.StatusOK, workerclient.TaskRunSecretsResponse{})
}

// resolveEnvGrants turns a consumption config into resolved grants. Each grant
// materializes its Secret against the run's team. A required grant that fails is
// returned as an error; an optional one is skipped. Names cannot collide -- the
// agent service refused a config that would, when it was saved.
func resolveEnvGrants(ctx context.Context, mat SecretMaterializer, teamID string, cons agentdef.SecretConsumption) ([]resolvedGrant, error) {
	var out []resolvedGrant
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
				out = append(out, resolvedGrant{secretID: g.Secret, itemName: name, envName: g.Prefix + name, value: val})
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
		out = append(out, resolvedGrant{secretID: g.Secret, itemName: g.Item, envName: g.EnvName, value: val})
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
