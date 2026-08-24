package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// HistoryVersion is the journal format this build writes and is the only one it
// reads. See docs/design/local-session-storage.md §6.1.
const HistoryVersion = 1

// Item types. The set is closed for the semantics defined here; a reader that
// meets a type outside it decides what to do from Item.Required, not from this
// list. See docs/design/local-session-storage.md §6.3 and §6.4.
const (
	ItemTurnStarted          = "turn_started"
	ItemMessage              = "message"
	ItemToolExecutionStarted = "tool_execution_started"
	ItemToolResult           = "tool_result"
	ItemCompaction           = "compaction"
	ItemNotesReplaced        = "notes_replaced"
	ItemTodosReplaced        = "todos_replaced"
	ItemAdditionalPromptSet  = "additional_prompt_set"
	ItemHeadSelected         = "head_selected"
	ItemCheckpoint           = "checkpoint"
	ItemTurnFinished         = "turn_finished"
	ItemTurnRecovered        = "turn_recovered"
)

// Tool outcome statuses carried by a ToolResult.
//
// ToolStatusUnknown is not an outcome the tool reported. It is what BuildMax
// writes for a call that crossed the execution boundary without returning,
// whether the turn was cancelled, interrupted, or lost with its process: the
// call may already have changed the world, and saying so is the only honest
// answer available.
const (
	// The first three are aliases rather than fresh literals: the agent loop
	// classifies a finished call and this package writes that classification
	// down, so one definition keeps the file format and the loop from drifting.
	ToolStatusCompleted = agent.ToolStatusCompleted
	ToolStatusFailed    = agent.ToolStatusFailed
	ToolStatusDenied    = agent.ToolStatusDenied
	// ToolStatusUnknown has no counterpart in the loop because the loop never
	// produces it: it is written by recovery, for a call the loop did not live
	// long enough to classify.
	ToolStatusUnknown = "unknown"
)

// Terminal turn statuses. Canceled and interrupted are separate because they are
// separate events — a person stopping the turn against the process being shut
// down under it — and they must not read the same. The run layer draws the same
// line; see docs/design/graceful-shutdown.md.
const (
	TurnCompleted   = "completed"
	TurnFailed      = "failed"
	TurnCanceled    = "canceled"
	TurnInterrupted = "interrupted"
)

// Header is the journal's immutable first record.
type Header struct {
	Type      string    `json:"type"`
	Version   int       `json:"version"`
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
}

// NewHeader returns the header for a new session journal.
func NewHeader(sessionID string, createdAt time.Time) Header {
	return Header{Type: "history", Version: HistoryVersion, SessionID: sessionID, CreatedAt: createdAt.UTC()}
}

// Validate rejects a header this build cannot interpret. A session ID that
// disagrees with the directory is corruption rather than an alternate spelling,
// but only the caller knows the directory, so that check lives with it.
func (h Header) Validate() error {
	if h.Type != "history" {
		return fmt.Errorf("%w: header type %q", ErrHistoryCorrupt, h.Type)
	}
	if h.Version != HistoryVersion {
		return fmt.Errorf("%w: journal version %d, this build reads %d", ErrHistoryVersion, h.Version, HistoryVersion)
	}
	if h.SessionID == "" {
		return fmt.Errorf("%w: header has no session id", ErrHistoryCorrupt)
	}
	return nil
}

// Payload is one item type's body. Implementations are the types below; an
// unrecognised type decodes to UnknownPayload rather than failing, so that
// Item.Required rather than the decoder decides whether the journal is usable.
type Payload interface {
	itemType() string
	// modelVisible reports whether applying this item changes what the model
	// sees. It is what Item.Required answers for a reader that does know the
	// type, and it keeps the two from drifting apart.
	modelVisible() bool
}

// TurnStarted opens a turn and fixes the runtime identity it ran under.
//
// Model and WorkspaceRoot are what this turn actually used. The session's
// current selections live in metadata and can differ, which is the point:
// resuming after switching either must not restate earlier turns.
type TurnStarted struct {
	RunID         string `json:"run_id"`
	Model         string `json:"model,omitempty"`
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
	InputKind     string `json:"input_kind,omitempty"`
}

func (TurnStarted) itemType() string   { return ItemTurnStarted }
func (TurnStarted) modelVisible() bool { return false }

// MessageItem carries one complete portable message, not only its text: an
// assistant record keeps its tool calls and its opaque provider state, and a
// user record keeps background provenance and non-text parts.
type MessageItem struct {
	Message llm.Message `json:"message"`
}

func (MessageItem) itemType() string   { return ItemMessage }
func (MessageItem) modelVisible() bool { return true }

// ToolExecutionStarted marks that an approved call is about to cross into the
// tool. It is the record that makes an interrupted call distinguishable from one
// that never ran, and it is worthless unless it reaches stable storage before
// the tool may change anything.
type ToolExecutionStarted struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
}

func (ToolExecutionStarted) itemType() string   { return ItemToolExecutionStarted }
func (ToolExecutionStarted) modelVisible() bool { return false }

// ToolResult closes one call. It is stored as its own item rather than as a
// second generic message; the reducer projects it to the tool-role message that
// provider adapters require.
type ToolResult struct {
	ToolCallID string            `json:"tool_call_id"`
	Status     string            `json:"status"`
	Content    string            `json:"content,omitempty"`
	Parts      []llm.ContentPart `json:"parts,omitempty"`
}

func (ToolResult) itemType() string   { return ItemToolResult }
func (ToolResult) modelVisible() bool { return true }

// Compaction replaces the model-visible prefix of this branch.
//
// CoveredHeadID names the last item the summary accounts for, which is what
// keeps compaction branch-scoped: a summary produced after a fork point cannot
// be reused by a branch that does not contain the items it summarised.
type Compaction struct {
	CoveredHeadID string `json:"covered_head_id"`
	Summary       string `json:"summary"`
}

func (Compaction) itemType() string   { return ItemCompaction }
func (Compaction) modelVisible() bool { return true }

// NotesReplaced carries the complete stamped list, not a delta. Durable state is
// small and rewritten wholesale, so a full list costs little and removes any
// question about how two partial writes combine.
type NotesReplaced struct {
	Notes []agent.Note `json:"notes"`
}

func (NotesReplaced) itemType() string   { return ItemNotesReplaced }
func (NotesReplaced) modelVisible() bool { return true }

// TodosReplaced carries the complete stamped list. See NotesReplaced.
type TodosReplaced struct {
	Todos []agent.Todo `json:"todos"`
}

func (TodosReplaced) itemType() string   { return ItemTodosReplaced }
func (TodosReplaced) modelVisible() bool { return true }

// AdditionalPromptSet replaces the durable additional system prompt.
type AdditionalPromptSet struct {
	Text string `json:"text"`
}

func (AdditionalPromptSet) itemType() string   { return ItemAdditionalPromptSet }
func (AdditionalPromptSet) modelVisible() bool { return true }

// HeadSelected records a branch choice, which is why rewind is one append and
// not an append plus a metadata write that could disagree with it.
//
// The item being returned to is ParentID. It is not repeated in the payload:
// every other record's parent is its physical predecessor, and this is the one
// record that deliberately points somewhere else, so the parent link already
// says everything a target field would. Storing it twice would only create a
// pair that could disagree.
type HeadSelected struct {
	Reason string `json:"reason,omitempty"`
}

func (HeadSelected) itemType() string   { return ItemHeadSelected }
func (HeadSelected) modelVisible() bool { return true }

// Checkpoint names a stable conversation head. It does not capture the
// workspace: files, processes and network effects are outside what this journal
// can restore, and StateDigest covers only the reduced conversation so a restore
// can be checked rather than trusted.
type Checkpoint struct {
	HistoryHeadID string `json:"history_head_id"`
	StateDigest   string `json:"state_digest,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

func (Checkpoint) itemType() string   { return ItemCheckpoint }
func (Checkpoint) modelVisible() bool { return false }

// TurnFinished closes a turn. Its presence is what tells the next open that
// there is nothing to recover.
type TurnFinished struct {
	Status     string `json:"status"`
	ErrorClass string `json:"error_class,omitempty"`
}

func (TurnFinished) itemType() string   { return ItemTurnFinished }
func (TurnFinished) modelVisible() bool { return false }

// TurnRecovered makes a cold recovery explicit before new work is accepted, so
// the repair appears in the journal once rather than being re-derived on every
// open.
type TurnRecovered struct {
	TurnID               string   `json:"turn_id"`
	UncertainToolCallIDs []string `json:"uncertain_tool_call_ids,omitempty"`
}

func (TurnRecovered) itemType() string   { return ItemTurnRecovered }
func (TurnRecovered) modelVisible() bool { return false }

// UnknownPayload is an item this build does not define. It keeps the bytes so a
// reader that only passes the journal through does not destroy them, and it
// carries no opinion of its own: whether the session is usable is decided by
// Item.Required.
type UnknownPayload struct {
	Kind string
	Raw  json.RawMessage
}

func (u UnknownPayload) itemType() string { return u.Kind }
func (UnknownPayload) modelVisible() bool { return true }

// Item is one journal record. Seq is its physical position and ID/ParentID its
// logical one; the two are separate because rewind leaves abandoned branches in
// place rather than truncating them.
type Item struct {
	Seq      uint64
	ID       string
	ParentID string
	TS       time.Time
	Required bool
	TurnID   string
	Payload  Payload
}

// Type returns the item's discriminator, or "" when it carries no payload.
func (it Item) Type() string {
	if it.Payload == nil {
		return ""
	}
	return it.Payload.itemType()
}

// NewItem builds an item with Required derived from the payload, which is the
// only place the two are allowed to be decided together.
func NewItem(seq uint64, id, parentID string, ts time.Time, turnID string, payload Payload) Item {
	return Item{
		Seq:      seq,
		ID:       id,
		ParentID: parentID,
		TS:       ts.UTC(),
		Required: payload != nil && payload.modelVisible(),
		TurnID:   turnID,
		Payload:  payload,
	}
}

// itemWire is the on-disk shape. Item itself carries a decoded payload, which is
// what callers want; the split keeps the JSON contract in one place.
type itemWire struct {
	Seq      uint64          `json:"seq"`
	ID       string          `json:"id"`
	ParentID string          `json:"parent_id,omitempty"`
	TS       time.Time       `json:"ts"`
	Type     string          `json:"type"`
	Required bool            `json:"required"`
	TurnID   string          `json:"turn_id,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// MarshalJSON writes the record shape documented in §6.2. Required is always
// emitted, including when false, so an older reader meeting an unknown type
// always finds an explicit answer rather than having to assume one.
func (it Item) MarshalJSON() ([]byte, error) {
	w := itemWire{
		Seq:      it.Seq,
		ID:       it.ID,
		ParentID: it.ParentID,
		TS:       it.TS.UTC(),
		Type:     it.Type(),
		Required: it.Required,
		TurnID:   it.TurnID,
	}
	if u, ok := it.Payload.(UnknownPayload); ok {
		w.Data = u.Raw
		return json.Marshal(w)
	}
	if it.Payload != nil {
		data, err := json.Marshal(it.Payload)
		if err != nil {
			return nil, err
		}
		w.Data = data
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes a record, keeping an unrecognised type as an
// UnknownPayload instead of failing. Refusing here would make every forward
// extension a load error even when the record only adds information.
func (it *Item) UnmarshalJSON(b []byte) error {
	var w itemWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	payload, err := decodePayload(w.Type, w.Data)
	if err != nil {
		return err
	}
	*it = Item{
		Seq:      w.Seq,
		ID:       w.ID,
		ParentID: w.ParentID,
		TS:       w.TS,
		Required: w.Required,
		TurnID:   w.TurnID,
		Payload:  payload,
	}
	return nil
}

func decodePayload(kind string, data json.RawMessage) (Payload, error) {
	switch kind {
	case ItemTurnStarted:
		return decodePayloadInto[TurnStarted](kind, data)
	case ItemMessage:
		return decodePayloadInto[MessageItem](kind, data)
	case ItemToolExecutionStarted:
		return decodePayloadInto[ToolExecutionStarted](kind, data)
	case ItemToolResult:
		return decodePayloadInto[ToolResult](kind, data)
	case ItemCompaction:
		return decodePayloadInto[Compaction](kind, data)
	case ItemNotesReplaced:
		return decodePayloadInto[NotesReplaced](kind, data)
	case ItemTodosReplaced:
		return decodePayloadInto[TodosReplaced](kind, data)
	case ItemAdditionalPromptSet:
		return decodePayloadInto[AdditionalPromptSet](kind, data)
	case ItemHeadSelected:
		return decodePayloadInto[HeadSelected](kind, data)
	case ItemCheckpoint:
		return decodePayloadInto[Checkpoint](kind, data)
	case ItemTurnFinished:
		return decodePayloadInto[TurnFinished](kind, data)
	case ItemTurnRecovered:
		return decodePayloadInto[TurnRecovered](kind, data)
	case "":
		return nil, fmt.Errorf("%w: record has no type", ErrHistoryCorrupt)
	default:
		return UnknownPayload{Kind: kind, Raw: data}, nil
	}
}

// decodePayloadInto decodes into a value payload rather than a pointer, so items
// compare with == and no two callers share one record's backing struct.
func decodePayloadInto[T Payload](kind string, data json.RawMessage) (Payload, error) {
	var v T
	if len(data) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("%w: %s payload: %v", ErrHistoryCorrupt, kind, err)
	}
	return v, nil
}

// History errors. They are distinct because callers act on them differently: an
// unsupported version is a build that is too old, corruption is a file that
// cannot be trusted, and an unknown required type is a record this build would
// mis-reduce if it guessed.
var (
	ErrHistoryVersion  = errors.New("unsupported history version")
	ErrHistoryCorrupt  = errors.New("history corrupt")
	ErrUnknownRequired = errors.New("history contains an unknown required record")
	ErrHeadNotFound    = errors.New("history head not found")
)
