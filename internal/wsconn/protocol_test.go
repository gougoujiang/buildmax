package wsconn

import (
	"encoding/json"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	original := MessageDelta{ConversationID: "c_abc123", Delta: "hello world"}
	data, err := Encode(TypeMessageDelta, original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	env, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if env.Type != TypeMessageDelta {
		t.Errorf("Type = %q, want %q", env.Type, TypeMessageDelta)
	}

	got, err := DecodePayload[MessageDelta](env)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if got.ConversationID != original.ConversationID || got.Delta != original.Delta {
		t.Errorf("payload mismatch: got %+v, want %+v", got, original)
	}
}

func TestEncodeProducesValidJSON(t *testing.T) {
	data, err := Encode(TypeConversationCreated, ConversationCreated{ConversationID: "c_xyz"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !json.Valid(data) {
		t.Errorf("Encode produced invalid JSON: %s", data)
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != TypeConversationCreated {
		t.Errorf("Type = %q, want %q", env.Type, TypeConversationCreated)
	}
}

func TestDecodeClientEvents(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		payload string
	}{
		{"conversation.create", TypeConversationCreate, `{"message":"hi"}`},
		{"conversation.message", TypeConversationMessage, `{"conversation_id":"c_1","content":"hello"}`},
		{"subscribe.task", TypeSubscribeTask, `{"task_id":"t_1"}`},
		{"unsubscribe.task", TypeUnsubscribeTask, `{"task_id":"t_1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := `{"type":"` + tt.typ + `","payload":` + tt.payload + `}`
			env, err := Decode([]byte(raw))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if env.Type != tt.typ {
				t.Errorf("Type = %q, want %q", env.Type, tt.typ)
			}
		})
	}
}

func TestDecodePayloadTyped(t *testing.T) {
	data, _ := Encode(TypeConversationMessage, ConversationMessage{ConversationID: "c_abc", Content: "test"})
	env, _ := Decode(data)
	got, err := DecodePayload[ConversationMessage](env)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if got.ConversationID != "c_abc" || got.Content != "test" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestDecodeInvalidJSON(t *testing.T) {
	_, err := Decode([]byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestEncodeAllServerEvents(t *testing.T) {
	events := []struct {
		typ     string
		payload any
	}{
		{TypeConversationCreated, ConversationCreated{ConversationID: "c_1"}},
		{TypeMessageDelta, MessageDelta{ConversationID: "c_1", Delta: "hi"}},
		{TypeMessageCompleted, MessageCompleted{ConversationID: "c_1"}},
		{TypeConversationError, ConversationError{ConversationID: "c_1", Error: "fail"}},
		{TypeTaskStatusChanged, TaskStatusChanged{TaskID: "t_1", Status: "SUCCEEDED"}},
		{TypeTaskStreamDelta, TaskStreamDelta{TaskID: "t_1", Delta: "data"}},
		{TypeTaskStreamDone, TaskStreamDone{TaskID: "t_1"}},
		{TypeSystemError, SystemError{Error: "oops"}},
	}
	for _, e := range events {
		t.Run(e.typ, func(t *testing.T) {
			data, err := Encode(e.typ, e.payload)
			if err != nil {
				t.Fatalf("Encode %s: %v", e.typ, err)
			}
			env, err := Decode(data)
			if err != nil {
				t.Fatalf("Decode %s: %v", e.typ, err)
			}
			if env.Type != e.typ {
				t.Errorf("Type = %q, want %q", env.Type, e.typ)
			}
		})
	}
}
