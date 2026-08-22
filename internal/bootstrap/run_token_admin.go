package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/server/authtoken"
)

// The operator-side half of the run token. Workers are given one automatically
// at dispatch; this exists for the case that used to be covered by copying the
// deployment-wide worker token: driving a worker route by hand to diagnose a
// run.
//
// It reads the same server.yaml the server does, so running it in a container
// or a pod needs no extra configuration.

// RunTokenCommandUsage is the help text for `buildmax-server run-token`.
const RunTokenCommandUsage = `Usage: buildmax-server run-token <task_run_id> [flags]

Prints a credential for one task run, for diagnosing that run by hand:

  curl -H "Authorization: Bearer $(buildmax-server run-token r_...)" \
       $SERVER/api/worker/task-runs/r_.../

Flags:
  --ttl duration   How long the token is valid (default: worker.run_token_ttl)

The token authorizes that one run and nothing else, and stops working when the
run reaches a terminal status. See docs/design/worker-run-token.md.
`

// RunRunTokenCommand executes `buildmax-server run-token ...`. args excludes
// the "run-token" word itself.
func RunRunTokenCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, RunTokenCommandUsage)
		return errors.New("run-token: a task run id is required")
	}
	taskRunID := args[0]

	fs := flag.NewFlagSet("run-token", flag.ContinueOnError)
	fs.SetOutput(out)
	ttl := fs.Duration("ttl", 0, "how long the token is valid")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	sc, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("server config: %w", err)
	}
	if sc.JWTSecret == "" {
		return fmt.Errorf("jwt_secret is required (set in server.yaml or %s)", config.EnvKeyBuildmaxJWTSecret)
	}
	if *ttl == 0 {
		*ttl = sc.Worker.RunTokenTTL
	}

	store, err := openStore(ctx, sc.Database)
	if err != nil {
		return err
	}
	// The claims come from the run, exactly as the scheduler builds them, so a
	// token minted here carries no more authority than the worker's own.
	run, task, err := store.GetTaskRunWithTask(ctx, taskRunID)
	if err != nil {
		return fmt.Errorf("load run: %w", err)
	}
	if run == nil || task == nil {
		return fmt.Errorf("no task run %s", taskRunID)
	}

	token, err := authtoken.MintRun(sc.JWTSecret, authtoken.RunClaims{
		UserID:    task.CreatedBy,
		TeamID:    task.TeamID,
		TaskRunID: run.ID,
		TaskID:    task.ID,
	}, *ttl, time.Now())
	if err != nil {
		return fmt.Errorf("mint run token: %w", err)
	}
	fmt.Fprintln(out, token)
	return nil
}
