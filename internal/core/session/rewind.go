package session

import (
	"errors"
	"fmt"
)

// ErrAlreadyHead reports that a point is where the branch already ends, so
// nothing follows it. Rewinding there is a caller mistake; forking there is
// the ordinary "branch off from here", so a fork surface reads this as an
// empty span rather than a failure.
var ErrAlreadyHead = errors.New("this point is already the head")

// ErrNoLanding reports that a message cannot be rewound because nothing on the
// branch precedes it. A picker filters those out; a surface handed one anyway
// needs to say why rather than fail as "not found".
var ErrNoLanding = errors.New("no record precedes this message")

// AbandonedEffect is one tool call the conversation is about to move past.
type AbandonedEffect struct {
	ToolCallID string
	ToolName   string
	// Returned reports whether the tool produced a result. A call that crossed
	// the execution boundary without returning is listed too, and is the more
	// dangerous of the two: it may have changed as much as one that finished,
	// and nothing recorded what.
	Returned bool
}

// AbandonedWork is what a rewind moves the conversation past and does not undo.
//
// It exists because rewind is honest about a hazard rather than hiding it (§8.1
// of docs/design/local-session-storage.md). The model's history returns to an
// earlier point; files, processes and network calls do not. A surface that
// offers rewind without showing this leaves the user believing the opposite,
// and leaves the model reasoning from a workspace picture that is no longer
// true.
type AbandonedWork struct {
	// Messages is how many model-visible messages leave the branch.
	Messages int
	// Effects are the tool calls that reached their tools, in the order they
	// ran. Empty means the abandoned span was conversation only — the one case
	// where a rewind really does undo everything it moved past.
	Effects []AbandonedEffect
}

// Undoable reports whether the rewind moves past nothing that touched the
// world, so a surface can say so plainly instead of warning about nothing.
func (a AbandonedWork) Undoable() bool { return len(a.Effects) == 0 }

// Abandoned reports what rewinding from head to target would leave in place.
//
// target must be on the branch ending at head, and must not be head itself:
// asking about where the branch already ends is answered with ErrAlreadyHead
// rather than an empty span, because a surface that computed the wrong target
// would otherwise report "nothing abandoned" and look correct.
func Abandoned(items []Item, head, target string) (AbandonedWork, error) {
	branch, err := Branch(items, head)
	if err != nil {
		return AbandonedWork{}, err
	}
	cut := -1
	for i, it := range branch {
		if it.ID == target {
			cut = i
			break
		}
	}
	switch {
	case cut < 0:
		return AbandonedWork{}, fmt.Errorf("%w: %s is not on the branch ending at %s", ErrHeadNotFound, target, head)
	case cut == len(branch)-1:
		return AbandonedWork{}, fmt.Errorf("%w: %s", ErrAlreadyHead, target)
	}

	var out AbandonedWork
	// Names come from the execution record rather than the assistant message,
	// because that is the record that proves the tool was actually entered.
	entered := map[string]string{}
	returned := map[string]bool{}
	var order []string
	for _, it := range branch[cut+1:] {
		switch p := it.Payload.(type) {
		case MessageItem:
			out.Messages++
		case ToolResult:
			out.Messages++
			returned[p.ToolCallID] = true
		case ToolExecutionStarted:
			if _, seen := entered[p.ToolCallID]; !seen {
				order = append(order, p.ToolCallID)
			}
			entered[p.ToolCallID] = p.ToolName
		}
	}
	for _, id := range order {
		out.Effects = append(out.Effects, AbandonedEffect{
			ToolCallID: id,
			ToolName:   entered[id],
			Returned:   returned[id],
		})
	}
	return out, nil
}

// RewindLanding returns the record a rewind removing messageID lands on.
//
// Rewind is exclusive: the message a person picks is the one they want back in
// the input box, so it leaves the branch along with everything after it, and
// the head has to name the record before it rather than the message itself.
//
// That record is the physical predecessor on the branch, not the previous
// message: notes, todos, a compaction, and the `turn_finished` of the turn
// before all sit between two messages, and they belong to work that is being
// kept. The one record stepped over is `turn_started`, because §7.1 opens a
// turn before the prompt that starts it — landing there would leave the branch
// inside the turn being dropped.
//
// A message with nothing before it has no landing. That is the first prompt of
// a session, and rewinding it would ask for a branch with no records at all;
// starting a new session says the same thing honestly.
func RewindLanding(items []Item, head, messageID string) (string, error) {
	branch, err := Branch(items, head)
	if err != nil {
		return "", err
	}
	cut := -1
	for i, it := range branch {
		if it.ID == messageID {
			cut = i
			break
		}
	}
	if cut < 0 {
		return "", fmt.Errorf("%w: %s is not on the branch ending at %s", ErrHeadNotFound, messageID, head)
	}
	for i := cut - 1; i >= 0; i-- {
		if _, opensTurn := branch[i].Payload.(TurnStarted); opensTurn {
			continue
		}
		return branch[i].ID, nil
	}
	return "", fmt.Errorf("%w: %s is the first message of this session", ErrNoLanding, messageID)
}
