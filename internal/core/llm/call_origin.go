package llm

import "context"

// CallOrigin is runtime-owned correlation metadata for one model call. It is
// not sent as prompt content and never carries user, workspace, or session
// identifiers.
type CallOrigin struct {
	// Surface is where the call began, such as cli, desktop, server, or worker.
	Surface string
	// ViaGateway says a BuildMax gateway forwarded the call to its provider.
	ViaGateway bool
}

type callOriginKey struct{}

// WithCallOrigin attaches runtime-owned origin metadata to one model call.
func WithCallOrigin(ctx context.Context, origin CallOrigin) context.Context {
	return context.WithValue(ctx, callOriginKey{}, origin)
}

// CallOriginFromContext returns the origin metadata for one model call.
func CallOriginFromContext(ctx context.Context) (CallOrigin, bool) {
	origin, ok := ctx.Value(callOriginKey{}).(CallOrigin)
	return origin, ok
}
