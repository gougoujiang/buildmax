package log

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

// With returns a context whose attrs are added to every record logged with it.
//
// Call sites use the stdlib slog.*Context functions and import nothing from
// here: the handler Init installs reads the attrs back out. That indirection is
// what lets internal/core log a run's identifiers, since it may not import
// infra. Attrs accumulate, so a worker can set the run once and a request can
// add to it.
func With(ctx context.Context, args ...any) context.Context {
	if len(args) == 0 {
		return ctx
	}
	// Let slog itself apply its key/value pairing rules rather than reimplement
	// them, including how it handles a trailing key with no value.
	var rec slog.Record
	rec.Add(args...)

	existing := attrsFrom(ctx)
	merged := make([]slog.Attr, 0, len(existing)+rec.NumAttrs())
	merged = append(merged, existing...)
	rec.Attrs(func(a slog.Attr) bool {
		merged = append(merged, a)
		return true
	})
	return context.WithValue(ctx, ctxKey{}, merged)
}

func attrsFrom(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	attrs, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	return attrs
}

type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := attrsFrom(ctx)
	if len(attrs) == 0 {
		return h.Handler.Handle(ctx, r)
	}
	clone := r.Clone()
	clone.AddAttrs(attrs...)
	return h.Handler.Handle(ctx, clone)
}

// WithAttrs and WithGroup rewrap, because the embedded handler would otherwise
// return a bare handler and slog.With would silently drop context attrs.
func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}
