// Package coordination provides the shared cross-replica primitives that make a
// multi-instance server keep its live state consistent: publish/subscribe fan-out,
// per-conversation distributed leases, and per-task replayable streams.
//
// It holds one Redis client and vends generic primitives; the server-side
// packages (websocket, turnqueue) build their own interface implementations on
// top. The dependency runs server -> infra -> core, so nothing here imports a
// server type. See docs/design/server-coordination.md.
package coordination

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Options is the Redis connection this backend dials. It is defined here rather
// than taken from config so this package, and the server packages that build on
// it, stay independent of the config layer.
type Options struct {
	Address  string
	Username string
	Password string
	DB       int
	TLS      bool
}

// Backend is the shared Redis coordination backend. A nil *Backend is never
// vended: callers select the in-process behavior by not constructing one.
type Backend struct {
	rdb *redis.Client
}

// New dials Redis and verifies the connection with one PING. It returns an error
// rather than a degraded backend so the caller can fail closed: serving with
// process-local coordination under a multi-replica manifest is the corruption
// this package exists to prevent.
func New(ctx context.Context, opts Options) (*Backend, error) {
	opt := &redis.Options{
		Addr:     opts.Address,
		Username: opts.Username,
		Password: opts.Password,
		DB:       opts.DB,
	}
	if opts.TLS {
		opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	rdb := redis.NewClient(opt)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("coordination: dial redis %q: %w", opts.Address, err)
	}
	return &Backend{rdb: rdb}, nil
}

// Close releases the Redis connection pool.
func (b *Backend) Close() error {
	if b == nil || b.rdb == nil {
		return nil
	}
	return b.rdb.Close()
}

// Publish sends one message to a channel. Every replica subscribed to the channel
// — including this one — receives it.
func (b *Backend) Publish(ctx context.Context, channel string, payload []byte) error {
	return b.rdb.Publish(ctx, channel, payload).Err()
}

// Subscribe returns a receive channel of the raw messages published to channel.
// It runs until ctx is cancelled, at which point the goroutine stops and the
// channel is closed. The buffer absorbs a brief consumer lag; a message dropped
// because the buffer is full is a stale live delta, not durable state.
func (b *Backend) Subscribe(ctx context.Context, channel string) <-chan []byte {
	out := make(chan []byte, subscribeBuffer)
	sub := b.rdb.Subscribe(ctx, channel)
	go func() {
		defer close(out)
		defer func() { _ = sub.Close() }()
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				select {
				case out <- []byte(msg.Payload):
				case <-ctx.Done():
					return
				default:
					// Consumer is lagging; drop this live delta rather than block
					// the whole channel's fan-out. Durable state is elsewhere.
				}
			}
		}
	}()
	return out
}

// subscribeBuffer bounds how far a slow subscriber may lag before live messages
// are dropped rather than blocking the fan-out.
const subscribeBuffer = 256
