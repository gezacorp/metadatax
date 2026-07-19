package proxmox

import (
	"sync"
	"time"
)

// ttlValue caches a single unkeyed value, refreshed at most once per ttl.
// A concurrent cache miss that started fetching before the value currently
// cached was fetched never overwrites it - the entry's "freshness" is
// tracked by when its fetch started, not when it finished, so a slower
// fetch that happened to start earlier can't clobber a faster one that
// started later and already landed.
type ttlValue[V any] struct {
	mu        sync.Mutex
	value     V
	fetchedAt time.Time
	valid     bool
}

func (c *ttlValue[V]) getOrFetch(ttl time.Duration, fetch func() (V, error)) (V, error) {
	c.mu.Lock()
	if c.valid && time.Since(c.fetchedAt) < ttl {
		v := c.value
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()

	start := time.Now()
	value, err := fetch()
	if err != nil {
		var zero V
		return zero, err
	}

	c.mu.Lock()
	if !c.valid || !c.fetchedAt.After(start) {
		c.value = value
		c.fetchedAt = start
		c.valid = true
	}
	c.mu.Unlock()

	return value, nil
}

type ttlEntry[V any] struct {
	value     V
	fetchedAt time.Time
}

// ttlCache caches values by key, refreshed at most once per ttl per key,
// evicting expired entries whenever a new one is stored so it doesn't grow
// unbounded over a long-running process's lifetime. Same start-time-based
// freshness tracking as ttlValue, so a concurrent slower-but-earlier-started
// fetch can't clobber a faster-but-later-started one for the same key.
type ttlCache[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]ttlEntry[V]
}

func newTTLCache[K comparable, V any]() *ttlCache[K, V] {
	return &ttlCache[K, V]{entries: map[K]ttlEntry[V]{}}
}

func (c *ttlCache[K, V]) getOrFetch(key K, ttl time.Duration, fetch func() (V, error)) (V, error) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok && time.Since(e.fetchedAt) < ttl {
		c.mu.Unlock()
		return e.value, nil
	}
	c.mu.Unlock()

	start := time.Now()
	value, err := fetch()
	if err != nil {
		var zero V
		return zero, err
	}

	c.mu.Lock()
	c.evictExpiredLocked(ttl)
	if e, ok := c.entries[key]; !ok || !e.fetchedAt.After(start) {
		c.entries[key] = ttlEntry[V]{value: value, fetchedAt: start}
	}
	c.mu.Unlock()

	return value, nil
}

// evictExpiredLocked removes stale entries. Must be called with c.mu held.
func (c *ttlCache[K, V]) evictExpiredLocked(ttl time.Duration) {
	for key, e := range c.entries {
		if time.Since(e.fetchedAt) >= ttl {
			delete(c.entries, key)
		}
	}
}
