package session

import (
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

func TestValidateAcceptsLinearJournal(t *testing.T) {
	items := journal(
		TurnStarted{RunID: "run1"},
		MessageItem{Message: llm.Message{Role: "user", Content: "hi"}},
		TurnFinished{Status: TurnCompleted},
	)
	if err := Validate(items); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejectsStructuralBreakage(t *testing.T) {
	cases := []struct {
		name  string
		items func() []Item
		want  error
	}{
		{
			name: "duplicate id",
			items: func() []Item {
				items := journal(TurnStarted{RunID: "r"}, TurnFinished{Status: TurnCompleted})
				items[1].ID = items[0].ID
				return items
			},
			want: ErrHistoryCorrupt,
		},
		{
			name: "parent that never appears",
			items: func() []Item {
				items := journal(TurnStarted{RunID: "r"}, TurnFinished{Status: TurnCompleted})
				items[1].ParentID = "ghost"
				return items
			},
			want: ErrHistoryCorrupt,
		},
		{
			name: "parent that appears later",
			items: func() []Item {
				items := journal(TurnStarted{RunID: "r"}, TurnFinished{Status: TurnCompleted})
				// Forward reference: a writer can only chain to what it has
				// already committed, so this cannot happen without corruption.
				items[0].ParentID = items[1].ID
				return items
			},
			want: ErrHistoryCorrupt,
		},
		{
			name: "second root",
			items: func() []Item {
				items := journal(TurnStarted{RunID: "r"}, TurnFinished{Status: TurnCompleted})
				items[1].ParentID = ""
				return items
			},
			want: ErrHistoryCorrupt,
		},
		{
			name: "missing id",
			items: func() []Item {
				items := journal(TurnStarted{RunID: "r"})
				items[0].ID = ""
				return items
			},
			want: ErrHistoryCorrupt,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.items()); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateIgnoresSeqGaps(t *testing.T) {
	// A gap is not a corruption signal: tamper-evidence is a non-goal, and a
	// missing record that could change a reduction is caught by the parent
	// chain instead. See the design record §7.2.
	items := journal(
		TurnStarted{RunID: "run1"},
		MessageItem{Message: llm.Message{Role: "user", Content: "hi"}},
	)
	items[1].Seq = 99
	if err := Validate(items); err != nil {
		t.Fatalf("seq gap rejected: %v", err)
	}
}

func TestValidateRejectsUnknownRequiredButAllowsSkippable(t *testing.T) {
	items := journal(TurnStarted{RunID: "run1"})
	items = append(items, Item{
		Seq: 2, ID: "ib", ParentID: "ia", TS: testTime,
		Required: false, Payload: UnknownPayload{Kind: "future_note"},
	})
	if err := Validate(items); err != nil {
		t.Fatalf("skippable unknown rejected: %v", err)
	}

	items[1].Required = true
	err := Validate(items)
	if !errors.Is(err, ErrUnknownRequired) {
		t.Fatalf("err = %v, want ErrUnknownRequired", err)
	}
}

func TestHeadIsTheLastPhysicalItem(t *testing.T) {
	items := journal(
		TurnStarted{RunID: "run1"},
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "user", Content: "two"}},
	)
	head, err := Head(items)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head != items[len(items)-1].ID {
		t.Errorf("head = %q, want %q", head, items[len(items)-1].ID)
	}

	if _, err := Head(nil); !errors.Is(err, ErrHeadNotFound) {
		t.Errorf("empty journal err = %v, want ErrHeadNotFound", err)
	}
}

func TestHeadAfterRewindIsStillTheLastItem(t *testing.T) {
	// The rewind record chains to the item being returned to, so the head needs
	// no special rule: it is the last record either way.
	items := journal(
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "abandoned"}},
	)
	rewind := NewItem(3, "ic", items[0].ID, testTime, "run2", HeadSelected{Reason: "user_rewind"})
	items = append(items, rewind)

	if err := Validate(items); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	head, err := Head(items)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head != "ic" {
		t.Fatalf("head = %q, want ic", head)
	}
	branch, err := Branch(items, head)
	if err != nil {
		t.Fatalf("Branch: %v", err)
	}
	if len(branch) != 2 || branch[0].ID != "ia" || branch[1].ID != "ic" {
		t.Fatalf("branch = %v, want ia -> ic with the abandoned item skipped", ids(branch))
	}
}

func TestBranchIsAChainInLogicalOrder(t *testing.T) {
	items := journal(
		TurnStarted{RunID: "run1"},
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "two"}},
	)
	branch, err := Branch(items, "ic")
	if err != nil {
		t.Fatalf("Branch: %v", err)
	}
	if len(branch) != 3 {
		t.Fatalf("branch length = %d, want 3", len(branch))
	}
	if branch[0].ParentID != "" {
		t.Errorf("branch does not start at the root: %v", ids(branch))
	}
	for i := 1; i < len(branch); i++ {
		if branch[i].ParentID != branch[i-1].ID {
			t.Fatalf("branch is not a chain: %v", ids(branch))
		}
	}
}

func TestBranchRejectsUnknownHead(t *testing.T) {
	items := journal(TurnStarted{RunID: "run1"})
	if _, err := Branch(items, "nope"); !errors.Is(err, ErrHeadNotFound) {
		t.Fatalf("err = %v, want ErrHeadNotFound", err)
	}
}

func ids(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}
