package architecture_test

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"
)

// The rules in this file are docs/design/timestamp-representation.md. A moment
// in time is a time.Time in Go, a DATETIME(6) column in MySQL, and RFC 3339 on
// the wire; a duration or a count is none of those and keeps its unit in its
// name.
//
// They are parsed from source for the same reason the entity-identity rules
// are: the row structs are the schema, and a rule that needs MySQL to run is a
// rule that does not run in CI.

// instantExempt are fields whose name ends in At or Time but which are not
// instants. Each is listed with the reason, because that reason is the whole
// test — an entry added without one is how the rule erodes.
var instantExempt = map[string]string{
	// A loop counter, not a clock. Named for what it counts.
	"agent.Note.WrittenIteration": "the loop iteration an entry first appeared at",
	"agent.Todo.WrittenIteration": "the loop iteration an entry last changed status at",
}

// integerTypes are the Go types an instant must never have. The rule is not
// about width: an instant carried as a number is one a caller can add 1000 to
// and still compile.
var integerTypes = map[string]bool{
	"int": true, "int32": true, "int64": true, "uint": true, "uint32": true, "uint64": true,
	"*int": true, "*int32": true, "*int64": true, "*uint": true, "*uint32": true, "*uint64": true,
}

// TestStoredInstantsAreDatetimeColumns fails when a row struct stores a moment
// in time as anything but a time.Time.
//
// The shape this catches is the schema this replaced: every timestamp was a
// bigint of Unix seconds, which read as an unusable integer in any hand-written
// query and admitted a millisecond value without complaint.
func TestStoredInstantsAreDatetimeColumns(t *testing.T) {
	seen := 0
	for _, f := range rowFields(t) {
		if !strings.HasSuffix(f.column, "_at") {
			continue
		}
		seen++
		qualified := f.table + "." + f.column
		if f.goType != "time.Time" && f.goType != "*time.Time" {
			t.Errorf("%s: %s is %s; a stored instant is a time.Time, or a *time.Time when it can be absent",
				f.file, qualified, f.goType)
		}
		tag := string(f.tag)
		if strings.Contains(tag, "bigint") || strings.Contains(tag, "type:int") {
			t.Errorf("%s: %s declares an integer column type", f.file, qualified)
		}
		// A sentinel zero is a second spelling of "absent" beside NULL, and it
		// is also a legitimate instant.
		if strings.Contains(tag, "default:0") {
			t.Errorf("%s: %s defaults to 0; absence is NULL and a *time.Time, never a sentinel",
				f.file, qualified)
		}
	}
	if seen == 0 {
		t.Fatal("found no *_at columns; the parser stopped seeing the row structs")
	}
}

// TestInstantFieldsAreNotIntegers fails when a field named for a moment in time
// carries a number, anywhere the domain or the store can see.
//
// The name is the rule's only handle: int64 is equally the type of a duration,
// a token count, and a retry attempt, so nothing but the name distinguishes a
// stored instant from them. A field that genuinely counts something belongs in
// instantExempt with a name that says what it counts.
func TestInstantFieldsAreNotIntegers(t *testing.T) {
	root := moduleRoot(t)
	seen := 0
	// internal/core is scanned whole rather than package by package: a domain
	// moving into its own package must not move out of this rule with it.
	for _, dir := range []string{"internal/core", "internal/infra/db"} {
		for _, path := range goFiles(t, filepath.Join(root, dir)) {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			file := parseFile(t, path)
			ast.Inspect(file, func(n ast.Node) bool {
				spec, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				st, ok := spec.Type.(*ast.StructType)
				if !ok {
					return true
				}
				for _, field := range st.Fields.List {
					for _, name := range field.Names {
						if !instantName(name.Name) {
							continue
						}
						key := file.Name.Name + "." + spec.Name.Name + "." + name.Name
						if instantExempt[key] != "" {
							continue
						}
						seen++
						if integerTypes[exprString(field.Type)] {
							t.Errorf("%s: %s is %s; name it for what it counts, or make it a time.Time",
								rel(root, path), key, exprString(field.Type))
						}
					}
				}
				return true
			})
		}
	}
	if seen == 0 {
		t.Fatal("found no instant-named fields; the parser stopped matching struct declarations")
	}
}

// instantName reports whether a Go field name claims to be a moment in time.
// A name ending in a unit — TimeoutSeconds, DurationMs — is claiming a
// duration, and durations stay numbers.
func instantName(name string) bool {
	return strings.HasSuffix(name, "At") || name == "Time" || strings.HasSuffix(name, "Time")
}
