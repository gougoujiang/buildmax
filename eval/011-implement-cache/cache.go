// Package cache provides a thread-safe in-memory key-value store with TTL support.
package cache

import "time"

// Cache is a thread-safe in-memory key-value store.
// Entries can have an optional TTL; expired entries are invisible to Get.
type Cache struct {
	// TODO: add fields — you will need a mutex, a map for entries,
	// and a channel or ticker for background cleanup.
}

// entry holds a cached value and its expiry time.
type entry struct {
	value   string
	expires time.Time // zero means no expiry
}

// New creates a Cache. If cleanupInterval > 0 a background goroutine
// periodically purges expired entries; call Close to stop it.
func New(cleanupInterval time.Duration) *Cache {
	// TODO: initialise the cache; start the cleanup goroutine if cleanupInterval > 0
	return &Cache{}
}

// Set stores value under key. If ttl > 0 the entry expires after ttl elapses.
// ttl == 0 means the entry never expires.
func (c *Cache) Set(key, value string, ttl time.Duration) {
	// TODO: implement (thread-safe)
}

// Get returns the value for key and true if the key exists and has not expired.
// Returns ("", false) for absent or expired keys.
func (c *Cache) Get(key string) (string, bool) {
	// TODO: implement (thread-safe)
	return "", false
}

// Delete removes key from the cache. No-op if key is absent.
func (c *Cache) Delete(key string) {
	// TODO: implement (thread-safe)
}

// Len returns the number of entries currently in the cache,
// including entries that may have expired but not yet been cleaned up.
func (c *Cache) Len() int {
	// TODO: implement (thread-safe)
	return 0
}

// Close stops the background cleanup goroutine if one is running.
func (c *Cache) Close() {
	// TODO: implement (no-op is acceptable when cleanupInterval == 0)
}
