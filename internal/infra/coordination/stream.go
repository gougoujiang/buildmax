package coordination

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// DoneMarker is delivered on a Tail channel when the stream's run has finished.
// A consumer maps it to its own terminal sentinel. The value is chosen to be
// improbable in agent output so it is never confused with a real delta.
const DoneMarker = "\x00__buildmax_stream_done__\x00"

// streamFieldDelta is the entry field holding a streamed delta.
const streamFieldDelta = "d"

// streamFieldDone marks the terminal entry.
const streamFieldDone = "done"

// streamTailBlock bounds one blocking read so the tail loop can observe context
// cancellation between reads rather than blocking forever.
const streamTailBlock = 2 * time.Second

// StreamAppend adds a delta to the task's stream, trimming it to maxLen entries
// (approximate) and refreshing its ttl so an abandoned stream expires.
func (b *Backend) StreamAppend(ctx context.Context, key, delta string, maxLen int64, ttl time.Duration) error {
	if delta == "" {
		return nil
	}
	if err := b.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: maxLen,
		Approx: true,
		Values: map[string]any{streamFieldDelta: delta},
	}).Err(); err != nil {
		return err
	}
	return b.rdb.Expire(ctx, key, ttl).Err()
}

// StreamSnapshot returns the concatenated deltas buffered so far, skipping the
// terminal marker.
func (b *Backend) StreamSnapshot(ctx context.Context, key string) (string, error) {
	entries, err := b.rdb.XRange(ctx, key, "-", "+").Result()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, e := range entries {
		if d, ok := e.Values[streamFieldDelta].(string); ok {
			sb.WriteString(d)
		}
	}
	return sb.String(), nil
}

// StreamDone appends the terminal marker and shortens the ttl so the stream
// lingers briefly for stragglers, then expires.
func (b *Backend) StreamDone(ctx context.Context, key string, ttl time.Duration) error {
	if err := b.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		Values: map[string]any{streamFieldDone: "1"},
	}).Err(); err != nil {
		return err
	}
	return b.rdb.Expire(ctx, key, ttl).Err()
}

// StreamTail delivers deltas appended after the call. When it reads the terminal
// marker it sends DoneMarker and closes; when ctx is cancelled it closes. It
// tails from the stream's current end, matching the in-memory hub, which streams
// future deltas and serves the backlog through StreamSnapshot.
func (b *Backend) StreamTail(ctx context.Context, key string) <-chan string {
	out := make(chan string, subscribeBuffer)
	go func() {
		defer close(out)
		lastID := "$"
		for {
			if ctx.Err() != nil {
				return
			}
			res, err := b.rdb.XRead(ctx, &redis.XReadArgs{
				Streams: []string{key, lastID},
				Block:   streamTailBlock,
				Count:   256,
			}).Result()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					continue // block timed out with nothing new
				}
				if ctx.Err() != nil {
					return
				}
				continue // transient; the next read retries
			}
			for _, stream := range res {
				for _, msg := range stream.Messages {
					lastID = msg.ID
					if _, done := msg.Values[streamFieldDone]; done {
						select {
						case out <- DoneMarker:
						case <-ctx.Done():
						}
						return
					}
					if d, ok := msg.Values[streamFieldDelta].(string); ok {
						select {
						case out <- d:
						case <-ctx.Done():
							return
						default:
							// consumer lagging; drop this delta
						}
					}
				}
			}
		}
	}()
	return out
}
