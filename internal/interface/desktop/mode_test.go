package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

// A first launch has chosen nothing, which is what makes the app ask instead of
// opening the workbench or a login form on its own.
func TestGetAuthStatusStartsUndecided(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir())
	status, err := NewApp().GetAuthStatus()
	if err != nil {
		t.Fatalf("GetAuthStatus: %v", err)
	}
	if status.Mode != "" || status.LoggedIn {
		t.Errorf("GetAuthStatus() = %+v, want an undecided, signed-out status", status)
	}
}

func TestUseLocalModeIsRemembered(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir())
	app := NewApp()
	status, err := app.UseLocalMode()
	if err != nil {
		t.Fatalf("UseLocalMode: %v", err)
	}
	if status.Mode != ModeLocal || status.LoggedIn {
		t.Errorf("UseLocalMode() = %+v, want local and signed out", status)
	}
	// The next launch reads it back rather than asking again.
	reread, err := NewApp().GetAuthStatus()
	if err != nil {
		t.Fatalf("GetAuthStatus: %v", err)
	}
	if reread.Mode != ModeLocal {
		t.Errorf("GetAuthStatus().Mode = %q, want %q", reread.Mode, ModeLocal)
	}
}

func TestConnectToServerLeavesLocalMode(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir())
	app := NewApp()
	if _, err := app.UseLocalMode(); err != nil {
		t.Fatalf("UseLocalMode: %v", err)
	}
	status, err := app.ConnectToServer()
	if err != nil {
		t.Fatalf("ConnectToServer: %v", err)
	}
	if status.Mode != "" {
		t.Errorf("ConnectToServer().Mode = %q, want the sign-in form", status.Mode)
	}
}

// An unreadable state file is a first launch, not a crash: the app asks which
// mode to use, and the next choice rewrites the file.
func TestUnreadableStateAsksAgain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BUILDMAX_HOME", home)
	path := filepath.Join(home, "desktop", "state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := NewApp().GetAuthStatus()
	if err != nil {
		t.Fatalf("GetAuthStatus: %v", err)
	}
	if status.Mode != "" {
		t.Errorf("GetAuthStatus().Mode = %q, want the app to ask", status.Mode)
	}
	if _, err := NewApp().UseLocalMode(); err != nil {
		t.Fatalf("UseLocalMode over a corrupt state file: %v", err)
	}
}
