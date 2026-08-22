package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/service/audit"
)

// The operator-side half of deployment administration.
//
// A System Administrator is a deployment-scoped authority, separate from every
// Team role. The first one has to come from somewhere, and this is that
// somewhere: a command on the machine that already holds the database
// credentials. There is deliberately no configuration value that grants the
// role — a second source of authority would be one the audit trail cannot
// describe and revocation cannot reach without a redeploy.
//
// Because the command needs only the database, it is also the recovery path.
// It behaves the same whether the deployment has zero admins or ten, so a
// deployment that has lost every admin is recovered with the same line that
// created the first one, and there is no break-glass credential to store,
// rotate, or leak.
//
// See docs/design/system-administration.md section 6.

// AdminCommandUsage is the help text for `buildmax-server admin`.
const AdminCommandUsage = `Usage: buildmax-server admin <command> [flags]

Commands:
  grant <email>    Grant system_admin to an existing account
  revoke <email>   Revoke system_admin from an account
  list             Show who holds system_admin

Flags for list:
  --all            Include revoked grants, newest first

A System Administrator can manage accounts, read deployment status, and search
the audit trail across teams. The grant carries no access to any team's issues,
conversations, artifacts, files, or run traces: those stay behind team
membership.

The account must exist first — granting does not create one:

  buildmax-server user create alice@example.com
  buildmax-server admin grant alice@example.com
  buildmax-server user login-code alice@example.com

Revoking the last grant is allowed here and refused through the API, because
this command is what recovers a deployment that has none.
See docs/design/system-administration.md.
`

// adminStore is the slice of the database the admin commands need. Taking an
// interface rather than *db.Store is what lets the command logic be tested
// without a database.
type adminStore interface {
	model.UserStore
	model.SystemGrantStore
	model.AuditWriter
}

// RunAdminCommand executes `buildmax-server admin ...`. args excludes the
// "admin" word itself.
func RunAdminCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, AdminCommandUsage)
		return errors.New("admin: a command is required")
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(out, AdminCommandUsage)
		return nil
	}
	store, err := openStoreFromConfig(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	switch args[0] {
	case "grant":
		return runAdminGrant(ctx, args[1:], out, store)
	case "revoke":
		return runAdminRevoke(ctx, args[1:], out, store)
	case "list":
		return runAdminList(ctx, args[1:], out, store)
	default:
		fmt.Fprint(out, AdminCommandUsage)
		return fmt.Errorf("admin: unknown command %q", args[0])
	}
}

// runAdminGrant grants system_admin to an existing account.
//
// It does not create the account it is given. Creating an account and minting
// deployment authority are two decisions, and keeping them apart is the same
// reason `user create` does not also issue a login code.
func runAdminGrant(ctx context.Context, args []string, out io.Writer, store adminStore) error {
	email, err := emailArg("admin grant", args, out)
	if err != nil {
		return err
	}
	user, err := lookupUser(ctx, store, email)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("no account for %s; create one first with: buildmax-server user create %s", email, email)
	}

	grant, err := store.GrantSystemRole(ctx, user.ID, model.SystemRoleAdmin, model.AuditActorOperator, time.Now().Unix())
	if err != nil {
		if errors.Is(err, model.ErrSystemGrantExists) {
			return fmt.Errorf("%s already holds %s", email, model.SystemRoleAdmin)
		}
		return fmt.Errorf("grant %s: %w", model.SystemRoleAdmin, err)
	}
	recordSystemGrantAudit(ctx, store, model.AuditSystemAdminGranted, user.ID)

	fmt.Fprintf(out, "Granted %s to %s (%s).\n", model.SystemRoleAdmin, user.Email, user.ID)
	fmt.Fprintf(out, "Grant %s, recorded in the audit trail.\n\n", grant.ID)
	if !user.HasPassword {
		fmt.Fprintf(out, "The account has no password yet. Let them in with:\n  buildmax-server user login-code %s\n\n", email)
	}
	fmt.Fprint(out, "The grant takes effect on their next request; no restart is needed.\n")
	return nil
}

func runAdminRevoke(ctx context.Context, args []string, out io.Writer, store adminStore) error {
	email, err := emailArg("admin revoke", args, out)
	if err != nil {
		return err
	}
	user, err := lookupUser(ctx, store, email)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("no account for %s", email)
	}

	revoked, err := store.RevokeSystemRole(ctx, user.ID, model.SystemRoleAdmin, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("revoke %s: %w", model.SystemRoleAdmin, err)
	}
	if !revoked {
		fmt.Fprintf(out, "%s does not hold %s; nothing to revoke.\n", email, model.SystemRoleAdmin)
		return nil
	}
	recordSystemGrantAudit(ctx, store, model.AuditSystemAdminRevoked, user.ID)

	fmt.Fprintf(out, "Revoked %s from %s (%s).\n", model.SystemRoleAdmin, user.Email, user.ID)
	fmt.Fprint(out, "It stops working on their next request. Their sessions are untouched;\n")
	fmt.Fprint(out, "revoke those separately if losing the role is not the whole intent.\n")

	// The API refuses to revoke the last grant. This command allows it, because
	// it is the recovery path — but an operator who did not mean to leave the
	// deployment with none should hear about it now rather than discover it
	// when nobody can open the admin area.
	remaining, err := store.CountActiveSystemGrants(ctx, model.SystemRoleAdmin)
	if err == nil && remaining == 0 {
		fmt.Fprintf(out, "\nThis deployment now has no %s. Portal's admin area is unreachable\n", model.SystemRoleAdmin)
		fmt.Fprint(out, "for everyone until you run:\n  buildmax-server admin grant <email>\n")
	}
	return nil
}

func runAdminList(ctx context.Context, args []string, out io.Writer, store adminStore) error {
	fs := flag.NewFlagSet("admin list", flag.ContinueOnError)
	fs.SetOutput(out)
	all := fs.Bool("all", false, "include revoked grants")
	if err := fs.Parse(args); err != nil {
		return err
	}
	grants, err := store.ListSystemGrants(ctx, *all)
	if err != nil {
		return fmt.Errorf("list grants: %w", err)
	}
	if len(grants) == 0 {
		if *all {
			fmt.Fprintln(out, "No system grants have ever been made.")
		} else {
			fmt.Fprintln(out, "No account holds a system role.")
		}
		fmt.Fprintln(out, "Grant one with: buildmax-server admin grant <email>")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMAIL\tUSER ID\tROLE\tGRANTED\tBY\tSTATUS")
	for _, g := range grants {
		status := "active"
		if !g.Active() {
			status = "revoked " + formatGrantTime(*g.RevokedAt)
		}
		// A grant outliving the account it names is not expected, but a list
		// command is the wrong place to fail on it: showing the user id is
		// more useful than refusing to print the table.
		email := "(unknown account)"
		if user, err := store.GetUser(ctx, g.UserID); err == nil && user != nil {
			email = user.Email
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			email, g.UserID, g.Role, formatGrantTime(g.GrantedAt), g.GrantedBy, status)
	}
	return w.Flush()
}

func formatGrantTime(unix int64) string {
	if unix == 0 {
		return "-"
	}
	return time.Unix(unix, 0).Format(time.RFC3339)
}

// recordSystemGrantAudit writes a change of deployment authority to the trail.
//
// The actor is the system rather than a user, for the same reason the model
// catalog commands say so: this runs from a shell on the machine that holds the
// database credentials, and inventing a user id would put a name in the record
// that nothing verified.
func recordSystemGrantAudit(ctx context.Context, store model.AuditWriter, action, userID string) {
	audit.NewRecorder(store).Record(ctx, model.AuditEvent{
		ActorType:  model.AuditActorSystem,
		ActorID:    model.AuditActorOperator,
		Action:     action,
		TargetType: "user",
		TargetID:   userID,
		Detail:     model.SystemRoleAdmin,
	})
}
