package agentapp

import (
	"context"
	"sync"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
	"github.com/gougoujiang/buildmax/internal/infra/hook"
)

// concurrentDriver is trackingDriver without the slice: Run reaches it from
// several goroutines here, and a recording fixture would be the thing the race
// detector reported.
type concurrentDriver struct{ typeName string }

func (d *concurrentDriver) Type() string { return d.typeName }
func (d *concurrentDriver) Run(context.Context, corehook.Entry, agent.HookInput) agent.HookOutput {
	return agent.HookOutput{}
}

// A parallel group gates its calls one at a time, but sub-agent runs overlap,
// so PreToolUse now reaches Run from several goroutines while a settings
// reload may swap the config underneath. Run under -race; without the locked
// read in Run this fails.
func TestHookManager_RunIsSafeAlongsideRefresh(t *testing.T) {
	m := NewHookManager(corehook.Config{
		PreToolUse: []corehook.Entry{{Type: corehook.TypeCommand, Matcher: "", Command: "x"}},
	}, map[string]hook.Driver{corehook.TypeCommand: &concurrentDriver{typeName: corehook.TypeCommand}})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				m.Run(context.Background(), agent.HookInput{Event: agent.HookPreToolUse, ToolName: "readfile"})
				m.Status()
			}
		})
	}
	wg.Go(func() {
		for i := range 50 {
			cmd := "x"
			if i%2 == 0 {
				cmd = "y"
			}
			m.Refresh(corehook.Config{
				PreToolUse: []corehook.Entry{{Type: corehook.TypeCommand, Command: cmd}},
			})
		}
	})
	wg.Wait()
}
