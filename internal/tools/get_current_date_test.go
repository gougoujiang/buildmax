package tools

import (
	"context"
	"regexp"
	"testing"
)

func TestGetCurrentDate_Name(t *testing.T) {
	var g GetCurrentDate
	if g.Name() != ToolNameGetCurrentDate {
		t.Errorf("Name() = %q, want %s", g.Name(), ToolNameGetCurrentDate)
	}
}

func TestGetCurrentDate_Execute(t *testing.T) {
	ctx := context.Background()
	var g GetCurrentDate
	out, err := g.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// YYYY-MM-DD
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	if !re.MatchString(out) {
		t.Errorf("Execute returned %q, want YYYY-MM-DD format", out)
	}
}
