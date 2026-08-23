package worker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/pluginwire"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// Plugin resolution happens here, in the route where a worker claims its run,
// beside the agent revision that already resolves there. The server resolves
// and the worker never does: a worker reading its team's activations would be a
// run token reading team state.
//
// Resolving late is safe because an activation names an exact version and
// digest. Publishing a release changes nothing until somebody moves a pin, so
// the pin — not the timing — is what forbids "latest at start". Resolving here
// also means suspending an activation still stops a run that was dispatched and
// has not yet started.

// resolvePluginPins turns the plugin names an agent carries into the releases
// this run will materialize.
//
// The second return is the reason a run cannot proceed. A named plugin whose
// team has no enabled activation is not skipped: an agent that names a plugin
// has declared it needs one, and a background run that quietly does less than
// its definition says is acted on by somebody who was not watching it.
func (h *Handler) resolvePluginPins(r *http.Request, run *model.TaskRun, task *model.Task, agent *model.Agent) (pins []model.PluginPin, refusal string) {
	// Already resolved. A worker polls this route while it runs, and a team
	// moving a pin mid-run must not change what the run was given — the same
	// rule, and the same reason, as the agent revision's first-write-wins.
	if len(run.PluginPins) > 0 {
		return run.PluginPins, ""
	}
	if agent == nil || len(agent.Plugins) == 0 {
		return nil, ""
	}
	if h.cfg.Activations == nil {
		return nil, "this deployment cannot resolve plugin activations, and this agent names plugins"
	}
	out := make([]model.PluginPin, 0, len(agent.Plugins))
	for _, name := range agent.Plugins {
		activation, err := h.cfg.Activations.GetPluginActivation(r.Context(), task.TeamID, name)
		if err != nil {
			componentLog().Error("worker handler: activation lookup failed",
				"task_id", task.ID, "plugin_name", name, "err", err)
			return nil, fmt.Sprintf("could not read this team's activation of plugin %q", name)
		}
		if activation == nil {
			return nil, fmt.Sprintf("this agent names plugin %q, which this team has not activated", name)
		}
		if !activation.Enabled {
			return nil, fmt.Sprintf("this agent names plugin %q, whose activation is suspended", name)
		}
		out = append(out, model.PluginPin{
			PluginName: activation.PluginName,
			Version:    activation.Version,
			Digest:     activation.Digest,
		})
	}
	return out, ""
}

// recordPluginPins stores what the run was given, so that afterwards something
// other than a fail-open trace can say which versions it had.
func (h *Handler) recordPluginPins(r *http.Request, run *model.TaskRun, pins []model.PluginPin) {
	if len(pins) == 0 || len(run.PluginPins) > 0 || h.cfg.TaskRuns == nil {
		return
	}
	if err := h.cfg.TaskRuns.RecordTaskRunPluginPins(r.Context(), run.ID, pins); err != nil {
		componentLog().Warn("worker handler: plugin pins not recorded", "task_run_id", run.ID, "err", err)
	}
}

// downloadPluginPackage serves one package to the run that was pinned to it.
//
// The route is run-scoped and so is what it will serve: only the releases this
// run's own pins name. The user-facing catalog routes stay user-scoped — a run
// token is not a user, and letting it browse would make it one.
func (h *Handler) downloadPluginPackage(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	name, ok := httputil.PathValue(w, r, "plugin_name")
	if !ok {
		return
	}
	version, ok := httputil.PathValue(w, r, "version")
	if !ok {
		return
	}
	if h.cfg.Plugins == nil || h.cfg.TaskRuns == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "plugin packages are not configured")
		return
	}
	run, _, err := h.cfg.TaskRuns.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "download_plugin_package", "task_run_id", taskRunID)
		return
	}
	if run == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		return
	}

	// The recorded pins are the authorization. They were resolved when this run
	// claimed itself, so a team activation changed since then cannot widen what
	// the run may fetch, and a package the run was never pinned to reads as not
	// found rather than as forbidden.
	pin, found := findPin(run.PluginPins, name, version)
	if !found {
		httputil.WriteJSONError(w, http.StatusNotFound, "this run is not pinned to that release")
		return
	}
	release, err := h.cfg.Plugins.GetRelease(r.Context(), pin.PluginName, pin.Version)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "download_plugin_package", "task_run_id", taskRunID)
		return
	}
	if release == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "release not found")
		return
	}
	// A yanked release stays downloadable here with no acknowledgement, unlike
	// the user route: yank withdraws a release from default selection and does
	// not reach into a team's activation, so a run pinned to one must still run.
	body, size, err := h.cfg.Plugins.OpenPackage(r.Context(), *release)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "the package bytes for this release are missing")
			return
		}
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "download_plugin_package", "task_run_id", taskRunID)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set(pluginwire.DigestHeader, release.Digest)
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		componentLog().Warn("worker handler: package stream cut short", "task_run_id", taskRunID, "plugin_name", name, "err", err)
	}
}

func findPin(pins []model.PluginPin, name, version string) (model.PluginPin, bool) {
	for _, pin := range pins {
		if pin.PluginName == name && pin.Version == version {
			return pin, true
		}
	}
	return model.PluginPin{}, false
}

func toWirePlugins(pins []model.PluginPin) []workerclient.TaskRunPlugin {
	if len(pins) == 0 {
		return nil
	}
	out := make([]workerclient.TaskRunPlugin, 0, len(pins))
	for _, pin := range pins {
		out = append(out, workerclient.TaskRunPlugin{
			Name: pin.PluginName, Version: pin.Version, Digest: pin.Digest,
		})
	}
	return out
}
