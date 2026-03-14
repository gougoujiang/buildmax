package worker

import (
	"buildmax/internal/streamhub"
	"buildmax/internal/storage/entity"
)

// Config holds dependencies for the worker API (chat-run get, patch, stream).
type Config struct {
	Token        string                 // Required for Authorization: Bearer <token> or X-Worker-Token
	ChatRunStore entity.ChatRunStore    // Required for get/patch/stream
	Hub          streamhub.StreamHub    // Optional; required for POST .../stream and for Done on patch
}
