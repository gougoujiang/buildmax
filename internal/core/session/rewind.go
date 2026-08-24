package session

import "fmt"

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
// rewinding to where you already are is a caller mistake, not a no-op worth
// silently accepting, because a surface that computed the wrong target would
// otherwise report "nothing abandoned" and look correct.
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
		return AbandonedWork{}, fmt.Errorf("rewind target %s is already the head", target)
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
