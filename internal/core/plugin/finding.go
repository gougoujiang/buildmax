package plugin

import "fmt"

// Severity separates what stops a plugin from loading from what a reader
// should merely be told.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

func (s Severity) String() string {
	if s == SeverityWarning {
		return "warning"
	}
	return "error"
}

// Finding is one thing parsing or validation noticed. Line is 1-based and 0
// when the position is unknown, so callers can print a plain message instead
// of a fake location.
type Finding struct {
	Severity Severity
	Field    string
	Line     int
	Message  string

	// Plugins names the plugins a finding concerns, so a surface can attribute
	// a collision to every side of it. Filtering on the message text instead
	// would break the first time a message was reworded.
	Plugins []string
}

// Concerns reports whether this finding is about the named plugin.
func (f Finding) Concerns(name string) bool {
	for _, p := range f.Plugins {
		if p == name {
			return true
		}
	}
	return false
}

func (f Finding) String() string {
	loc := ""
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d: ", ManifestFile, f.Line)
	}
	field := ""
	if f.Field != "" {
		field = f.Field + ": "
	}
	return loc + f.Severity.String() + ": " + field + f.Message
}

// HasErrors reports whether any finding blocks use of the manifest.
func HasErrors(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Errors returns only the blocking findings, for a caller that reports the
// reason a directory was rejected.
func Errors(findings []Finding) []Finding {
	var out []Finding
	for _, f := range findings {
		if f.Severity == SeverityError {
			out = append(out, f)
		}
	}
	return out
}
