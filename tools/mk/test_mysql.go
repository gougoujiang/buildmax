package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/gougoujiang/buildmax/internal/config"
)

// mysqlScopePackages is what the persistence gate runs. It is the store package
// today because that is where the real-store tests are; a service or handler
// scope joins this list when it grows tests that need the database rather than
// a mock. See docs/design/verification-program.md §4.1.
var mysqlScopePackages = []string{"./internal/infra/db"}

// tempDatabasePrefix marks a database this command created. Nothing without it
// is ever dropped: the DSN a contributor exports usually names a database they
// care about, and a scope that both connects to it and issues DROP has to be
// unable to confuse the two.
const tempDatabasePrefix = "buildmax_test_"

// tempDatabaseName is the exact shape createTempDatabase generates, and the
// only shape dropTempDatabase will drop.
var tempDatabaseName = regexp.MustCompile(`^` + tempDatabasePrefix + `[0-9a-f]{16}$`)

// cmdTestMySQL runs the store scope against a real MySQL, on a database it
// creates and drops.
//
// It exists because the ordinary suite cannot prove persistence: every test in
// mysqlScopePackages skips itself when BUILDMAX_TEST_DSN is unset, so a green
// `./make test` says nothing about schema, query, transaction, or MySQL-specific
// behavior. This command refuses to be that quiet — an absent DSN is an error
// rather than a skip, and a test that skips for the DSN's absence anyway fails
// the run.
//
// It never starts Docker. A contributor points it at a MySQL they already run;
// CI points it at a service container. Starting one as a side effect of a test
// command would make `./make test` mutate the machine.
func cmdTestMySQL(flags []string) error {
	if stray, found := packageAfterFlag(flags); found {
		return usageErrorf("test", "%q is a package pattern, but `test mysql` selects its own scope: %s",
			stray, strings.Join(mysqlScopePackages, " "))
	}
	baseDSN := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if baseDSN == "" {
		return usageErrorf("test", "%s is not set, and this scope has no default to fall back on.\n"+
			"Point it at a MySQL you are willing to let a test suite write to, using an\n"+
			"account that may CREATE DATABASE — this scope runs on a database of its own:\n"+
			"  export %s='root:root@tcp(127.0.0.1:3306)/buildmax'\n"+
			"See docs/contribute/testing.md for one way to start that server.",
			config.EnvKeyBuildmaxTestDSN, config.EnvKeyBuildmaxTestDSN)
	}
	cfg, err := mysqldriver.ParseDSN(baseDSN)
	if err != nil {
		return fmt.Errorf("%s is not a DSN the MySQL driver can parse: %w", config.EnvKeyBuildmaxTestDSN, err)
	}
	if _, err := useSandboxHome(); err != nil {
		return err
	}

	name, err := newTempDatabaseName()
	if err != nil {
		return err
	}
	if err := createTempDatabase(cfg, name); err != nil {
		return err
	}
	// Dropped even when the tests fail: the diagnosis is in the output, and a
	// run that left its database behind would fill the server one merge at a
	// time. The name is printed above, so a contributor who wants to keep one
	// creates it themselves.
	defer func() {
		if dropErr := dropTempDatabase(cfg, name); dropErr != nil {
			logf("test", "could not drop %s: %v", name, dropErr)
		}
	}()

	scopeCfg := *cfg
	scopeCfg.DBName = name
	scopeDSN := scopeCfg.FormatDSN()

	fmt.Printf("Running the MySQL scope in %s (BUILDMAX_HOME=%s)...\n",
		strings.Join(mysqlScopePackages, " "), sandboxDir)
	fmt.Printf("  server:   %s\n", redactedTarget(cfg))
	fmt.Printf("  database: %s (created for this run, dropped after it)\n", name)

	testErr := runMySQLScope(scopeDSN, flags)

	// Read after the run rather than before it: the schema is built by the
	// scope's own db.New calls, which is the AutoMigrate-and-migrations path a
	// server start takes. Reporting it here describes what the tests actually
	// ran against.
	if applied, err := appliedMigrationIDs(scopeCfg); err != nil {
		logf("test", "could not read schema_migration: %v", err)
	} else {
		fmt.Printf("  schema:   %s\n", describeMigrations(applied))
	}
	if testErr != nil {
		fmt.Printf("  reproduce: %s='%s' ./make test mysql%s\n",
			config.EnvKeyBuildmaxTestDSN, redactedDSN(cfg), flagSuffix(flags))
	}
	return testErr
}

// runMySQLScope runs the scope and fails when a test skipped itself for want of
// the DSN. `go test` reports a skip as success, so a scope that silently
// stopped testing anything would otherwise pass -- which is the exact failure
// this gate exists to make impossible.
func runMySQLScope(dsn string, flags []string) error {
	args := append([]string{"test", "-json", "-count=1"}, mysqlScopePackages...)
	args = append(args, flags...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), config.EnvKeyBuildmaxTestDSN+"="+dsn)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	result, scanErr := scanTestStream(out, os.Stdout)
	runErr := cmd.Wait()
	if scanErr != nil {
		return scanErr
	}
	if len(result.dsnSkips) > 0 {
		return fmt.Errorf("%d test(s) skipped because %s was not set, in a scope that exists to run them: %s\n"+
			"The DSN reached this command but not the test binary",
			len(result.dsnSkips), config.EnvKeyBuildmaxTestDSN, strings.Join(result.dsnSkips, ", "))
	}
	if runErr != nil && len(result.failedPackages) > 0 {
		return fmt.Errorf("%w\nfailing package(s): %s", runErr, strings.Join(result.failedPackages, ", "))
	}
	return runErr
}

// testStream is what one `go test -json` run tells this command beyond its exit
// status: which packages failed, so the summary can name them, and which tests
// opted out for want of the DSN, so the gate can refuse to pass on them.
type testStream struct {
	failedPackages []string
	dsnSkips       []string
}

// scanTestStream copies a `go test -json` stream to w as plain text and
// classifies it.
//
// Reprinting each Output event verbatim keeps the familiar `go test` stream:
// -json is here to classify, not to change what a contributor reads.
func scanTestStream(r io.Reader, w io.Writer) (testStream, error) {
	// Matching the message text keeps this honest about which skip it means: a
	// test skipped for any other reason is the author's decision, not this
	// gate's business.
	wantedDSN := config.EnvKeyBuildmaxTestDSN + " not set"
	var out testStream
	dec := json.NewDecoder(r)
	for {
		var ev struct {
			Action  string
			Test    string
			Package string
			Output  string
		}
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, fmt.Errorf("reading `go test -json` output: %w", err)
		}
		// A package-level fail: Test is empty on the event that closes the
		// package, which is the one worth naming.
		if ev.Action == "fail" && ev.Test == "" && ev.Package != "" {
			out.failedPackages = append(out.failedPackages, ev.Package)
		}
		if ev.Output == "" {
			continue
		}
		if _, err := io.WriteString(w, ev.Output); err != nil {
			return out, err
		}
		if ev.Test != "" && strings.Contains(ev.Output, wantedDSN) {
			out.dsnSkips = append(out.dsnSkips, ev.Package+"."+ev.Test)
		}
	}
}

func newTempDatabaseName() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("naming a temporary database: %w", err)
	}
	return tempDatabasePrefix + hex.EncodeToString(buf[:]), nil
}

// serverConn opens a connection to the server rather than to any one database,
// which is what CREATE and DROP DATABASE need.
func serverConn(cfg *mysqldriver.Config) (*sql.DB, error) {
	serverCfg := *cfg
	serverCfg.DBName = ""
	return sql.Open("mysql", serverCfg.FormatDSN())
}

func createTempDatabase(cfg *mysqldriver.Config, name string) error {
	conn, err := serverConn(cfg)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", redactedTarget(cfg), err)
	}
	defer conn.Close()
	// utf8mb4 to match what the server's own ensureDatabase creates, so the
	// gate does not pass on a collation production never uses.
	if _, err := conn.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		var mysqlErr *mysqldriver.MySQLError
		// 1044/1045: the account reached the server but may not create this
		// database. Worth naming, because the obvious DSN to reach for is the
		// deployment's own, and that account is granted one schema by name.
		if errors.As(err, &mysqlErr) && (mysqlErr.Number == 1044 || mysqlErr.Number == 1045) {
			return fmt.Errorf("%s may not create a database on %s: %w\n"+
				"This scope runs on a database of its own, so %s needs an account with CREATE DATABASE",
				cfg.User, redactedTarget(cfg), err, config.EnvKeyBuildmaxTestDSN)
		}
		return fmt.Errorf("creating %s on %s: %w", name, redactedTarget(cfg), err)
	}
	return nil
}

func dropTempDatabase(cfg *mysqldriver.Config, name string) error {
	if !tempDatabaseName.MatchString(name) {
		return fmt.Errorf("refusing to drop %q: this command only drops the databases it creates", name)
	}
	conn, err := serverConn(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Exec("DROP DATABASE IF EXISTS `" + name + "`")
	return err
}

// appliedMigrationIDs reads what the run recorded in schema_migration. A
// missing table is not an error: AutoMigrate owns the additive schema, and the
// list of explicit migrations is empty until one is appended.
func appliedMigrationIDs(cfg mysqldriver.Config) ([]string, error) {
	conn, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.Query("SELECT id FROM schema_migration ORDER BY applied_at ASC, id ASC")
	if err != nil {
		var mysqlErr *mysqldriver.MySQLError
		// 1146: the table does not exist.
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1146 {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func describeMigrations(applied []string) string {
	if len(applied) == 0 {
		return "AutoMigrate only; no explicit migration recorded"
	}
	return fmt.Sprintf("%d applied, through %s", len(applied), applied[len(applied)-1])
}

// redactedTarget names the server without its password, for output that a
// contributor may paste into an issue.
func redactedTarget(cfg *mysqldriver.Config) string {
	return fmt.Sprintf("%s@%s(%s)", cfg.User, cfg.Net, cfg.Addr)
}

// redactedDSN is the reproduction command's DSN with the password replaced by a
// placeholder, so the line can be copied but not leaked as-is.
func redactedDSN(cfg *mysqldriver.Config) string {
	shown := *cfg
	if shown.Passwd != "" {
		shown.Passwd = "<password>"
	}
	return shown.FormatDSN()
}

func flagSuffix(flags []string) string {
	if len(flags) == 0 {
		return ""
	}
	return " " + strings.Join(flags, " ")
}
