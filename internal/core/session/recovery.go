package session

// Tool outcome classifications for a call found on an interrupted branch. See
// docs/design/local-session-storage.md §7.3.
const (
	// OutcomeNotStarted: the model asked for the call and BuildMax never
	// recorded entering the tool, so nothing outside BuildMax happened.
	OutcomeNotStarted = "not_started"
	// OutcomeUnknown: BuildMax recorded entering the tool and never recorded a
	// result. The call may have changed the world. It is never retried
	// automatically.
	OutcomeUnknown = "outcome_unknown"
	// OutcomeKnown: a result was recorded, whatever it says.
	OutcomeKnown = "known"
)

// ToolCallOutcome is one call's classification after an interruption.
type ToolCallOutcome struct {
	ToolCallID string
	ToolName   string
	Class      string
}

// Recovery is what an interrupted branch needs before it accepts new work.
type Recovery struct {
	// TurnID is the interrupted turn. Empty when nothing needs repair.
	TurnID string
	// Uncertain are calls that crossed the execution boundary without
	// returning. Each needs a durable unknown result before the model sees the
	// branch again, so it is told to verify rather than left to assume.
	Uncertain []ToolCallOutcome
	// NotStarted are calls the model requested that never reached a tool. They
	// need no repair record; they are reported because a caller deciding what
	// to say to the user wants the distinction.
	NotStarted []ToolCallOutcome
}

// Needed reports whether this branch has an interrupted turn to close.
func (r Recovery) Needed() bool { return r.TurnID != "" }

// Analyze classifies the branch ending at head for interruption repair.
//
// A branch whose last turn was closed needs nothing: a turn that ended as
// completed, failed, canceled, or interrupted was ended by a process that knew
// what it had done, and re-deriving that judgement would only risk contradicting
// it. Only a turn with no terminal record is repaired.
func Analyze(items []Item, head string) (Recovery, error) {
	branch, err := Branch(items, head)
	if err != nil {
		return Recovery{}, err
	}

	// Walk forward, resetting at each turn boundary: only the final, still-open
	// turn can have calls in flight, and an earlier turn's calls were already
	// resolved or already repaired.
	var (
		turnID    string
		open      bool
		requested []ToolCallOutcome
		entered   = map[string]string{}
		answered  = map[string]bool{}
	)
	reset := func() {
		requested = nil
		entered = map[string]string{}
		answered = map[string]bool{}
	}
	for _, it := range branch {
		switch p := it.Payload.(type) {
		case TurnStarted:
			turnID, open = p.RunID, true
			reset()
		case TurnFinished:
			open = false
			reset()
		case MessageItem:
			for _, call := range p.Message.ToolCalls {
				requested = append(requested, ToolCallOutcome{ToolCallID: call.ID, ToolName: call.Name})
			}
		case ToolExecutionStarted:
			entered[p.ToolCallID] = p.ToolName
		case ToolResult:
			answered[p.ToolCallID] = true
		}
	}
	if !open {
		return Recovery{}, nil
	}

	rec := Recovery{TurnID: turnID}
	for _, call := range requested {
		if answered[call.ToolCallID] {
			continue
		}
		if name, ok := entered[call.ToolCallID]; ok {
			if name != "" {
				call.ToolName = name
			}
			call.Class = OutcomeUnknown
			rec.Uncertain = append(rec.Uncertain, call)
			continue
		}
		call.Class = OutcomeNotStarted
		rec.NotStarted = append(rec.NotStarted, call)
	}
	// A call can be entered without the assistant message that requested it
	// having reached storage — the message is committed first, so this means a
	// torn journal rather than normal operation. Reporting it as uncertain is
	// the conservative reading: the tool was entered.
	for id, name := range entered {
		if answered[id] || containsCall(rec.Uncertain, id) || containsCall(rec.NotStarted, id) {
			continue
		}
		rec.Uncertain = append(rec.Uncertain, ToolCallOutcome{ToolCallID: id, ToolName: name, Class: OutcomeUnknown})
	}
	return rec, nil
}

func containsCall(calls []ToolCallOutcome, id string) bool {
	for _, c := range calls {
		if c.ToolCallID == id {
			return true
		}
	}
	return false
}
