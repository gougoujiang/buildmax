package websocket

import "log/slog"

// Identity belongs in an attr, not in every message string.
func componentLog() *slog.Logger { return slog.With("component", "websocket") }
