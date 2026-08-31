package main

import (
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/gougoujiang/buildmax/internal/config"
)

// The gate's whole purpose is that an unset DSN stops the run. Falling back to
// a default, or skipping the way the tests themselves do, would reproduce the
// silence it exists to end.
func TestMySQLScopeRefusesAnAbsentDSN(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxTestDSN, "")
	err := cmdTestMySQL(nil)
	if err == nil {
		t.Fatal("cmdTestMySQL ran without a DSN")
	}
	if !strings.Contains(err.Error(), config.EnvKeyBuildmaxTestDSN) {
		t.Errorf("error = %v, want it to name the variable to set", err)
	}
}

func TestMySQLScopeRefusesAnUnparseableDSN(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxTestDSN, "not a dsn")
	if err := cmdTestMySQL(nil); err == nil {
		t.Fatal("cmdTestMySQL accepted a DSN the driver cannot parse")
	}
}

// The scope owns its packages, so a pattern is a contributor believing they
// narrowed a run that would have widened instead -- the same mistake cmdTest
// refuses for a package after a flag.
func TestMySQLScopeRefusesAPackagePattern(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxTestDSN, "root:pw@tcp(127.0.0.1:3306)/buildmax")
	if err := cmdTestMySQL([]string{"./internal/server"}); err == nil {
		t.Fatal("cmdTestMySQL accepted a package pattern")
	}
}

// A skip for the DSN's absence has to fail the run. `go test` reports one as
// success, so without this the gate could pass while testing nothing -- the
// acceptance criterion in docs/design/verification-program.md §4.3.
func TestScanTestStreamCatchesADSNSkip(t *testing.T) {
	stream := `{"Action":"run","Package":"p","Test":"TestCreateUser"}
{"Action":"output","Package":"p","Test":"TestCreateUser","Output":"    store_test.go:103: BUILDMAX_TEST_DSN not set, skipping store integration test\n"}
{"Action":"skip","Package":"p","Test":"TestCreateUser"}
`
	var out strings.Builder
	got, err := scanTestStream(strings.NewReader(stream), &out)
	if err != nil {
		t.Fatalf("scanTestStream: %v", err)
	}
	if len(got.dsnSkips) != 1 || got.dsnSkips[0] != "p.TestCreateUser" {
		t.Errorf("dsnSkips = %v, want [p.TestCreateUser]", got.dsnSkips)
	}
	if !strings.Contains(out.String(), "skipping store integration test") {
		t.Errorf("output = %q, want the original `go test` text reprinted", out.String())
	}
}

// A test that skips for its own reasons is the author's decision. Reading every
// skip as a failure would make the gate an obstacle to writing one.
func TestScanTestStreamIgnoresOtherSkips(t *testing.T) {
	stream := `{"Action":"output","Package":"p","Test":"TestSomething","Output":"    x_test.go:9: needs Docker\n"}
{"Action":"skip","Package":"p","Test":"TestSomething"}
`
	got, err := scanTestStream(strings.NewReader(stream), &strings.Builder{})
	if err != nil {
		t.Fatalf("scanTestStream: %v", err)
	}
	if len(got.dsnSkips) != 0 {
		t.Errorf("dsnSkips = %v, want none", got.dsnSkips)
	}
}

// The summary has to name the package that failed, per
// docs/design/verification-program.md §4.1. A failing test inside a package is
// not the same event as the package failing, and only the latter names it.
func TestScanTestStreamNamesTheFailingPackage(t *testing.T) {
	stream := `{"Action":"fail","Package":"p","Test":"TestOne"}
{"Action":"fail","Package":"p"}
{"Action":"pass","Package":"q"}
`
	got, err := scanTestStream(strings.NewReader(stream), &strings.Builder{})
	if err != nil {
		t.Fatalf("scanTestStream: %v", err)
	}
	if len(got.failedPackages) != 1 || got.failedPackages[0] != "p" {
		t.Errorf("failedPackages = %v, want [p]", got.failedPackages)
	}
}

// The command connects to a server it is allowed to drop databases on, so the
// name it drops has to be one it generated. Anything else is a contributor's
// own data.
func TestDropTempDatabaseRefusesAForeignName(t *testing.T) {
	cfg, err := mysqldriver.ParseDSN("root:pw@tcp(127.0.0.1:3306)/buildmax")
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	for _, name := range []string{"buildmax", "mysql", "buildmax_test_", "buildmax_test_zzzz", "buildmax_test_00`; DROP DATABASE x"} {
		if err := dropTempDatabase(cfg, name); err == nil {
			t.Errorf("dropTempDatabase(%q) was allowed", name)
		}
	}
}

func TestNewTempDatabaseNameIsDroppable(t *testing.T) {
	name, err := newTempDatabaseName()
	if err != nil {
		t.Fatalf("newTempDatabaseName: %v", err)
	}
	if !tempDatabaseName.MatchString(name) {
		t.Errorf("generated %q, which dropTempDatabase would refuse to drop", name)
	}
}

// Output a contributor may paste into an issue must not carry the password.
func TestRedactionKeepsThePasswordOut(t *testing.T) {
	cfg, err := mysqldriver.ParseDSN("root:hunter2@tcp(127.0.0.1:3306)/buildmax")
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	for _, got := range []string{redactedTarget(cfg), redactedDSN(cfg)} {
		if strings.Contains(got, "hunter2") {
			t.Errorf("%q leaks the password", got)
		}
	}
}
