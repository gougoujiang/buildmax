package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// DefaultWebhookUserID is the CreatedBy user ID for webhook-originated runs when not configured.
	DefaultWebhookUserID = "webhook"
)

// WebhookRequest holds parsed webhook input for the adapter. Use this so Receive can be tested without transport-specific request types.
type WebhookRequest struct {
	Body   []byte
	Header map[string][]string
}

// WebhookCallbackSender is an optional transport adapter for delivering webhook
// results. Core owns the decision of whether a callback target exists; server or
// infra code owns the actual HTTP transport.
type WebhookCallbackSender interface {
	SendWebhookCallback(ctx context.Context, callbackURL string, output string) error
}

// WebhookAdapter implements ChannelAdapter for the webhook channel.
// It extracts the message from the JSON body using a configurable path (e.g. "message" or "body.text").
type WebhookAdapter struct {
	// MessagePath is the JSON path to the message in the body (e.g. "message", "body.text"). Default "message".
	MessagePath string
	// UserID is the user ID to set on the turn (CreatedBy for runs). Default DefaultWebhookUserID.
	UserID string
	// CallbackSender delivers output when conversationID is a callback URL. Optional.
	CallbackSender WebhookCallbackSender
}

// NewWebhookAdapter returns a WebhookAdapter with the given message path and optional user ID.
func NewWebhookAdapter(messagePath string, userID string) *WebhookAdapter {
	if messagePath == "" {
		messagePath = "message"
	}
	if userID == "" {
		userID = DefaultWebhookUserID
	}
	return &WebhookAdapter{MessagePath: messagePath, UserID: userID}
}

// Receive builds a ConversationTurn from a WebhookRequest. Raw must be *WebhookRequest.
func (a *WebhookAdapter) Receive(ctx context.Context, raw any) (ConversationTurn, error) {
	req, ok := raw.(*WebhookRequest)
	if !ok {
		return ConversationTurn{}, fmt.Errorf("webhook: raw must be *WebhookRequest, got %T", raw)
	}
	message, err := getJSONPathString(req.Body, a.MessagePath)
	if err != nil {
		return ConversationTurn{}, err
	}
	if message == "" {
		return ConversationTurn{}, fmt.Errorf("webhook: message empty at path %q", a.MessagePath)
	}
	rawMap := make(map[string]any)
	if len(req.Body) > 0 {
		_ = json.Unmarshal(req.Body, &rawMap)
	}
	callbackURL, _ := getJSONPathString(req.Body, "callback_url")
	if callbackURL != "" {
		if rawMap == nil {
			rawMap = make(map[string]any)
		}
		rawMap["callback_url"] = callbackURL
	}
	return ConversationTurn{
		Channel:        ChannelWebhook,
		ConversationID: "",
		UserID:         a.UserID,
		Message:        message,
		Raw:            rawMap,
	}, nil
}

// Send delivers output to the conversationID if it looks like a URL and a callback sender is configured. Otherwise no-op.
func (a *WebhookAdapter) Send(ctx context.Context, conversationID string, output string) error {
	if conversationID == "" {
		return nil
	}
	if !strings.HasPrefix(conversationID, "http://") && !strings.HasPrefix(conversationID, "https://") {
		return nil
	}
	if a.CallbackSender == nil {
		return nil
	}
	return a.CallbackSender.SendWebhookCallback(ctx, conversationID, output)
}

// getJSONPathString returns a string at the given dot-separated path in the JSON body.
// Supports one or two segments (e.g. "message" or "body.text").
func getJSONPathString(body []byte, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path required")
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	parts := strings.SplitN(path, ".", 2)
	var v any = m
	for _, key := range parts {
		mm, ok := v.(map[string]any)
		if !ok {
			return "", nil
		}
		v = mm[key]
		if v == nil {
			return "", nil
		}
	}
	switch s := v.(type) {
	case string:
		return s, nil
	case nil:
		return "", nil
	default:
		return fmt.Sprint(v), nil
	}
}
