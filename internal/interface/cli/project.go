package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/spf13/cobra"
)

// relinkCommandHint is what a surface names when it has just registered a
// Project beside one whose locator no longer resolves. It is a constant so the
// message and the command cannot drift apart.
const relinkCommandHint = "buildmax project relink <project-id>"

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Inspect and repair the local projects sessions and memory belong to",
		Long: `A project is the local unit of work a session belongs to: one Git repository
including every one of its worktrees, or one plain folder. It is what --continue,
the session picker, and project memory are scoped to.

A project is found again by a locator — a repository's common Git directory, or
a folder's path — so moving a repository leaves the old project unreachable and
the next run there registers a new, empty one. Relinking is how the old project,
with its memory, is pointed at where the repository now is. Nothing infers that:
joining two memory domains by guess would be undetectable afterwards.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
	}
	cmd.AddCommand(newProjectListCommand(), newProjectRelinkCommand(), newProjectForgetCommand())
	return cmd
}

func newProjectListCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "list",
		Short:        "List the local projects, newest use first",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rows, err := agentapp.NewProjectManager(config.ProjectsDir()).Store().List(cmd.Context())
			if err != nil {
				return fmt.Errorf("list projects: %w", err)
			}
			writeProjectList(cmd.OutOrStdout(), rows)
			return nil
		},
	}
}

// writeProjectList prints one row per Project, marking the ones whose locator
// no longer resolves — the candidates for a relink.
func writeProjectList(w io.Writer, rows []localproject.Summary) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No local projects yet. The first run in a directory registers one.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tKIND\tLOCATOR")
	for _, r := range rows {
		locator := r.Locator
		if _, err := os.Stat(locator); err != nil {
			locator += "  (missing)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.Name, r.Kind, locator)
	}
	_ = tw.Flush()
}

func newProjectRelinkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relink <project-id>",
		Short: "Point an existing project at this directory",
		Long: `Point an existing project — and the memory and sessions that hang off it — at
the directory given, after a repository or folder has moved.

The project is named explicitly because the alternative is a heuristic, and a
heuristic that joined two memory domains would leave no trace of having done so.
Run ` + "`buildmax project list`" + ` to see which projects no longer resolve.`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace, _ := cmd.Flags().GetString("workspace")
			relinked, err := agentapp.NewProjectManager(config.ProjectsDir()).
				Relink(cmd.Context(), args[0], workspace)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s) now resolves from %s\n",
				relinked.Name, relinked.ID, relinked.DefaultWorkspace)
			return nil
		},
	}
	cmd.Flags().String("workspace", "", "directory to point the project at (default: current directory)")
	return cmd
}

func newProjectForgetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forget [memory-name]",
		Short: "Delete one of this project's memories, or all of them",
		Long: `Delete what this project remembers.

Memories are Markdown files under <BUILDMAX_HOME>/projects/<id>/memory/ and can
be edited or removed there directly; this is the same operation with the index
regenerated for you. It touches no sessions, and no session touches it:
forgetting and clearing a project's history are separate decisions.

Run ` + "`buildmax doctor`" + ` to see how many memories this project holds and where.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			workspace, _ := cmd.Flags().GetString("workspace")
			if (len(args) == 0) == !all {
				return errors.New("name one memory, or pass --all to forget every one")
			}
			project, err := currentProject(cmd.Context(), workspace)
			if err != nil {
				return err
			}
			store := agentapp.NewProjectManager(config.ProjectsDir()).Store()
			if all {
				removed, err := store.ClearMemories(cmd.Context(), project.ID)
				if err != nil {
					return err
				}
				noun := "memories"
				if removed == 1 {
					noun = "memory"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Forgot %d %s of %s. Its sessions are untouched.\n",
					removed, noun, project.Name)
				return nil
			}
			if err := store.DeleteMemory(cmd.Context(), project.ID, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Forgot %s.\n", args[0])
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "forget every memory of this project")
	cmd.Flags().String("workspace", "", "directory whose project to forget from (default: current directory)")
	return cmd
}

// currentProject resolves workspace to its Project, registering one if this is
// the first run there.
func currentProject(ctx context.Context, workspace string) (localproject.Project, error) {
	return agentapp.NewProjectManager(config.ProjectsDir()).Resolve(ctx, workspace)
}
