package desktop

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// Rewind and fork bindings.
//
// Desktop holds no session between calls — a run owns one for its life and
// releases it at the end — so each of these opens what it needs and closes it
// again. That is also what makes "not while a run is in flight" enforce
// itself: the run holds the writer lock, and taking it is how a move discovers
// it cannot proceed.

// HistoryToolEffect is one tool call on the far side of a chosen point.
type HistoryToolEffect struct {
	Name string `json:"name"`
	// Interrupted marks a call that entered its tool and never reported back.
	// It is the worse of the two: it may have changed as much as one that
	// finished, and nothing recorded what.
	Interrupted bool `json:"interrupted"`
}

// HistoryPoint is one message a session can be rewound to or forked from.
//
// Messages and Tools describe the same span for both operations, and mean
// opposite things about it. A rewind drops that span from this conversation
// and leaves the tools' effects on disk; a fork drops nothing — the original
// keeps all of it — but the copy begins without knowing that work happened.
// The surface chooses the reading; the binding reports the span once.
type HistoryPoint struct {
	ItemID   string              `json:"item_id"`
	Role     string              `json:"role"`
	Content  string              `json:"content"`
	Messages int                 `json:"messages"`
	Tools    []HistoryToolEffect `json:"tools,omitempty"`
	// IsHead marks the end of the conversation. Forking from it is the common
	// case — branch off from where we are — while rewinding to it is not a
	// move, so a surface offers one and not the other.
	IsHead bool `json:"is_head"`
}

// HistoryPointsResult is the picker's whole input, consequences included.
//
// Every point carries what choosing it would affect, rather than the surface
// asking again each time the selection moves: the computation is in-memory
// over a branch that is already loaded, and a round trip per keystroke would
// be the expensive half of an otherwise free question.
type HistoryPointsResult struct {
	SessionID string         `json:"session_id"`
	Points    []HistoryPoint `json:"points"`
}

// HistoryMoveResult is what happened, for the report shown afterwards.
type HistoryMoveResult struct {
	// SessionID is the session to display next: the same one after a rewind,
	// the new child after a fork.
	SessionID string              `json:"session_id"`
	Messages  int                 `json:"messages"`
	Tools     []HistoryToolEffect `json:"tools,omitempty"`
}

// GetHistoryPoints lists the messages this session can be rewound to or forked
// from, most recent first.
//
// It reads without the writer lock, so the picker still opens while a run is in
// flight. Only the move itself is refused then.
func (a *App) GetHistoryPoints(sessionID string) (HistoryPointsResult, error) {
	if sessionID == "" {
		return HistoryPointsResult{}, fmt.Errorf("session ID required")
	}
	sess, err := sessionManager().Read(sessionID, "")
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return HistoryPointsResult{}, fmt.Errorf("session not found: %s", sessionID)
		}
		return HistoryPointsResult{}, fmt.Errorf("read session: %w", err)
	}
	points := agentapp.RewindPoints(sess)
	out := make([]HistoryPoint, 0, len(points))
	for i, p := range points {
		hp := HistoryPoint{
			ItemID:  p.ItemID,
			Role:    historyRole(p),
			Content: p.Content,
			IsHead:  i == len(points)-1,
		}
		// The head has nothing after it, so there is no span to describe and
		// AbandonedBy rightly refuses to answer for it.
		if !hp.IsHead {
			affected, err := sess.AbandonedBy(p.ItemID)
			if err != nil {
				return HistoryPointsResult{}, fmt.Errorf("inspect %s: %w", p.ItemID, err)
			}
			hp.Messages = affected.Messages
			hp.Tools = historyTools(affected)
		}
		out = append(out, hp)
	}
	reverseHistoryPoints(out)
	return HistoryPointsResult{SessionID: sessionID, Points: out}, nil
}

// RewindSession moves a session's conversation back to itemID.
//
// No session lifecycle hook fires. Nothing is starting or ending here — the
// user is editing history — and the transient open this needs is an artifact
// of Desktop not holding sessions, not an event a hook should be told about.
// The TUI, which rewinds a session it already has open, fires none either.
func (a *App) RewindSession(projectID, sessionID, itemID string) (HistoryMoveResult, error) {
	sess, _, err := a.openForHistoryMove(projectID, sessionID, itemID, "rewind")
	if err != nil {
		return HistoryMoveResult{}, err
	}
	defer closeQuietly(sess)

	abandoned, err := sess.Rewind(itemID)
	if err != nil {
		return HistoryMoveResult{}, fmt.Errorf("rewind: %w", err)
	}
	return HistoryMoveResult{
		SessionID: sessionID,
		Messages:  abandoned.Messages,
		Tools:     historyTools(abandoned),
	}, nil
}

// ForkSession copies this session's history through itemID into a new session
// and returns it, leaving the original exactly as it was.
func (a *App) ForkSession(projectID, sessionID, itemID string) (HistoryMoveResult, error) {
	parent, ag, err := a.openForHistoryMove(projectID, sessionID, itemID, "fork")
	if err != nil {
		return HistoryMoveResult{}, err
	}
	defer closeQuietly(parent)

	// Read the span before forking, not after: the child's history stops at the
	// fork point, so nothing on it can answer what came later.
	var affected session.AbandonedWork
	if got, err := parent.AbandonedBy(itemID); err == nil {
		affected = got
	}
	child, err := sessionManager().Fork(parent, itemID, ag.DefaultModelName())
	if err != nil {
		return HistoryMoveResult{}, fmt.Errorf("fork: %w", err)
	}
	// Desktop holds no session between calls, so the child is closed like the
	// parent. Selecting it in the sidebar is what opens it again.
	if err := child.Close(); err != nil {
		return HistoryMoveResult{}, fmt.Errorf("fork: closing the new session: %w", err)
	}
	return HistoryMoveResult{
		SessionID: child.ID(),
		Messages:  affected.Messages,
		Tools:     historyTools(affected),
	}, nil
}

// openForHistoryMove validates the arguments and takes the session's writer
// lock, translating a busy session into something a person can act on.
func (a *App) openForHistoryMove(projectID, sessionID, itemID, verb string) (*agentapp.SessionContext, *agentapp.AgentApp, error) {
	if projectID == "" {
		return nil, nil, fmt.Errorf("project ID required")
	}
	if sessionID == "" {
		return nil, nil, fmt.Errorf("session ID required")
	}
	if itemID == "" {
		return nil, nil, fmt.Errorf("a message to %s from is required", verb)
	}
	ag, err := a.agentAppForProject(projectID)
	if err != nil {
		return nil, nil, err
	}
	sess, err := sessionManager().Open(sessionID, ag.DefaultModelName())
	if err != nil {
		switch {
		case errors.Is(err, session.ErrLocked):
			return nil, nil, fmt.Errorf("this session is busy; stop the run before you %s it", verb)
		case errors.Is(err, session.ErrSessionNotFound):
			return nil, nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, nil, fmt.Errorf("open session: %w", err)
	}
	return sess, ag, nil
}

func closeQuietly(sess *agentapp.SessionContext) {
	if err := sess.Close(); err != nil {
		slog.Warn("closing a session after a history move failed", "session_id", sess.ID(), "err", err)
	}
}

// historyRole is who the picker should say spoke. A background event arrives as
// a user message although the user did not say it, and calling it theirs would
// misattribute it.
func historyRole(p agentapp.RewindPoint) string {
	if p.Role == "user" && p.Source != "" {
		return "event"
	}
	return p.Role
}

func historyTools(a session.AbandonedWork) []HistoryToolEffect {
	if len(a.Effects) == 0 {
		return nil
	}
	out := make([]HistoryToolEffect, 0, len(a.Effects))
	for _, e := range a.Effects {
		name := e.ToolName
		if name == "" {
			name = "(unknown tool)"
		}
		out = append(out, HistoryToolEffect{Name: name, Interrupted: !e.Returned})
	}
	return out
}

// reverseHistoryPoints puts the newest first: working near the end of a
// conversation is far more common than working near its start.
func reverseHistoryPoints(p []HistoryPoint) {
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
}
