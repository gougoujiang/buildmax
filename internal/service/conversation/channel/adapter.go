package channel

import "context"

// Adapter normalizes channel-specific input into a Turn and can deliver output
// back to that channel.
type Adapter interface {
	Receive(ctx context.Context, raw any) (Turn, error)
	Send(ctx context.Context, target string, output string) error
}
