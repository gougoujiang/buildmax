package work

import (
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"os"
	"path/filepath"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
)

func (h *Handler) workspacesDir() string {
	return h.cfg.WorkspacesDir
}

// readRunGlobal reads one file from a task run's global directory.
//
// The persist backend is asked first, then the server's own disk. That second
// look is not a consolation prize for a lost object — on the default `local_fs`
// backend it is the only place the file ever is. That backend's PutRunGlobal
// and GetRunGlobal are deliberately no-ops, because the worker has already
// written the file under WorkspacesDir and copying it into a persist root would
// store it twice.
//
// So a caller that asks only the backend gets ErrNotFound for every run on
// every default deployment, and reports it as "storage lost this" when the file
// is sitting on disk the whole time. Both readers go through here to keep that
// from happening again.
func (h *Handler) readRunGlobal(ctx context.Context, task *coretask.Task, taskRunID, relPath string) ([]byte, error) {
	// The trace path is written by the worker and read back from the database,
	// so it is not trusted input by the time it reaches a filepath.Join.
	clean, err := blob.CleanRelPath(relPath)
	if err != nil {
		return nil, err
	}
	if h.cfg.PersistStorage != nil {
		data, err := h.cfg.PersistStorage.GetRunGlobal(ctx, blob.RunObjectRef{
			CreatedBy:      task.CreatedBy,
			ConversationID: task.ConversationID,
			TaskID:         task.ID,
			TaskRunID:      taskRunID,
			RelPath:        clean,
		})
		if err == nil {
			return data, nil
		}
		// Anything other than "the backend does not have it" is the backend's
		// problem, and looking on disk would only hide it.
		if !errors.Is(err, apierr.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return os.ReadFile(h.runGlobalPath(task, taskRunID, clean))
}

// runGlobalPath is where a worker writes a run's global files, and therefore
// where local_fs deployments keep them.
func (h *Handler) runGlobalPath(task *coretask.Task, taskRunID, cleanRelPath string) string {
	return filepath.Join(
		h.workspacesDir(),
		task.CreatedBy,
		"conversations", task.ConversationID,
		"tasks", task.ID,
		taskRunID,
		"global",
		filepath.FromSlash(cleanRelPath),
	)
}
