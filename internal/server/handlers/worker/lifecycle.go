package worker

import (
	"net/http"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// requireRunning gates the routes an agent exercises only while its run is
// executing: stream, artifacts, secrets, the issue read and comment, plugin
// download, and managed inference. It writes 409 and returns false unless the
// run is RUNNING.
//
// This is lifecycle authorization on top of the run token. The token proves the
// caller is this run's worker; the state proves that authority is currently
// exercisable. A run still SCHEDULED has not been claimed, and a terminal run
// has finished — so a leaked but unexpired token cannot stream, publish an
// artifact, read a Secret, comment, download a package, or spend tokens after
// the run is over. See docs/design/worker-api-network-boundary.md §8.
//
// getTaskRun is deliberately not gated: it is the heartbeat and cancellation
// poll, and a restarted pod must be able to read a run in any status —
// including terminal — to discover it is already done. The claim and the
// terminal report (patchTaskRun) are gated by their own atomic status
// transition, not by this helper.
func requireRunning(w http.ResponseWriter, status string) bool {
	if status == string(coretask.RunStatusRunning) {
		return true
	}
	httputil.WriteJSONError(w, http.StatusConflict, "task run is not executing")
	return false
}
