package cli

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestTUIRunOwnerCloseCancelsAndWaits(t *testing.T) {
	owner := newTUIRunOwner(context.Background())
	exited := make(chan struct{})
	if !owner.Go(func(ctx context.Context) {
		<-ctx.Done()
		close(exited)
	}) {
		t.Fatal("run owner refused its first run")
	}

	owner.Close()

	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("Close returned before the run observed cancellation")
	}
	if owner.Go(func(context.Context) {}) {
		t.Fatal("closed run owner accepted another goroutine")
	}
}

func TestSendTUIMessageStopsWhenTheProgramCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sendTUIMessage(ctx, make(chan tea.Msg), streamDeltaMsg{Delta: "late"}) {
		t.Fatal("a canceled TUI blocked trying to deliver a late stream event")
	}
}

func TestCtrlCCancelsForegroundRuns(t *testing.T) {
	model := NewModel(TUIOpts{})
	exited := make(chan struct{})
	if !model.runs.Go(func(ctx context.Context) {
		<-ctx.Done()
		close(exited)
	}) {
		t.Fatal("run owner refused the foreground run")
	}

	_, cmd := handleKeyMsg(model, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c did not ask Bubble Tea to quit")
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("ctrl+c did not cancel the foreground run")
	}
	model.Close()
}
