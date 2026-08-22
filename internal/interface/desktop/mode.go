package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/config"
)

// Desktop runs in one of two modes, the same two the CLI has always had:
//
//   - local: the agent runs here against the models in settings.yaml, and no
//     server is involved. This is the single-user install.
//   - server: the same local agent, plus a signed-in BuildMax account — managed
//     models, and the bridge to a team's work.
//
// The CLI reaches both without asking: it runs locally and `buildmax login` is
// optional. Desktop used to open on a login form, which made a server look
// required to run an agent on your own machine. The choice is remembered so the
// app opens where it was left.
const (
	ModeLocal  = "local"
	ModeServer = "server"
)

// desktopState is what the app remembers about itself between launches. It is
// deliberately separate from credentials: a mode is a preference, and clearing
// it must not sign anyone out.
type desktopState struct {
	Mode string `json:"mode,omitempty"`
}

func desktopStatePath() string {
	return filepath.Join(config.DataDir(), "desktop", "state.json")
}

// readDesktopState never fails the caller: an unreadable or corrupt state file
// means the app asks which mode to use, which is the same thing a first launch
// does.
func readDesktopState() desktopState {
	data, err := os.ReadFile(desktopStatePath())
	if err != nil {
		return desktopState{}
	}
	var state desktopState
	if err := json.Unmarshal(data, &state); err != nil {
		return desktopState{}
	}
	return state
}

func writeDesktopState(state desktopState) error {
	path := desktopStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
