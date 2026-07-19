package proxmox

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTTLCacheGetOrFetchCachesWithinTTL(t *testing.T) {
	c := newTTLCache[string, int]()

	calls := 0
	fetch := func() (int, error) {
		calls++
		return 42, nil
	}

	v, err := c.getOrFetch("key", time.Minute, fetch)
	require.NoError(t, err)
	assert.Equal(t, 42, v)

	v, err = c.getOrFetch("key", time.Minute, fetch)
	require.NoError(t, err)
	assert.Equal(t, 42, v)
	assert.Equal(t, 1, calls, "second call within TTL should be served from cache")
}

func TestTTLCacheEvictsExpiredEntries(t *testing.T) {
	c := newTTLCache[string, int]()

	// Store a stale entry directly, then trigger the eviction sweep via a
	// fresh fetch for a different key.
	c.entries["stale"] = ttlEntry[int]{value: 1, fetchedAt: time.Now().Add(-2 * time.Minute)}

	_, err := c.getOrFetch("fresh", time.Minute, func() (int, error) { return 2, nil })
	require.NoError(t, err)

	_, staleStillCached := c.entries["stale"]
	_, freshStillCached := c.entries["fresh"]
	assert.False(t, staleStillCached)
	assert.True(t, freshStillCached)
}

func TestTTLCacheDoesNotLetAnOlderFetchClobberANewerOne(t *testing.T) {
	c := newTTLCache[string, int]()

	slowStarted := make(chan struct{})
	slowRelease := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)

	// This fetch starts first, but is held open (via slowRelease) until
	// after the "fast" fetch below has already completed and cached its
	// result - simulating a slower request that started earlier.
	go func() {
		defer wg.Done()
		_, _ = c.getOrFetch("key", time.Minute, func() (int, error) {
			close(slowStarted)
			<-slowRelease
			return 1, nil // stale-by-the-time-it-lands value
		})
	}()

	<-slowStarted

	// This fetch starts after the slow one but completes first.
	_, err := c.getOrFetch("key", time.Minute, func() (int, error) {
		return 2, nil // the value that should win
	})
	require.NoError(t, err)

	close(slowRelease)
	wg.Wait()

	c.mu.Lock()
	got := c.entries["key"]
	c.mu.Unlock()

	assert.Equal(t, 2, got.value, "the fetch that started later must win even though the earlier-started fetch finished later")
}

func TestTTLValueGetOrFetchCachesWithinTTL(t *testing.T) {
	c := &ttlValue[int]{}

	calls := 0
	fetch := func() (int, error) {
		calls++
		return 7, nil
	}

	v, err := c.getOrFetch(time.Minute, fetch)
	require.NoError(t, err)
	assert.Equal(t, 7, v)

	v, err = c.getOrFetch(time.Minute, fetch)
	require.NoError(t, err)
	assert.Equal(t, 7, v)
	assert.Equal(t, 1, calls, "second call within TTL should be served from cache")
}

func TestTTLValueRefetchesAfterExpiry(t *testing.T) {
	c := &ttlValue[int]{}

	_, err := c.getOrFetch(time.Millisecond, func() (int, error) { return 1, nil })
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	calls := 0
	v, err := c.getOrFetch(time.Millisecond, func() (int, error) {
		calls++
		return 2, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, v)
	assert.Equal(t, 1, calls)
}
