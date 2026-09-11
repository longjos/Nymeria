package w3w

import (
	"sync"
	"time"
)

// ttlCache is a small mutex-protected TTL cache. The what3words grid is
// immutable, so a successful lookup can be cached for a long time; a clock
// func is injected so tests don't need to sleep. Eviction at the cap is
// "soonest expiry first" rather than true LRU — O(n) at the cap, n is small
// (CacheMax, default 512), and it's called at most once per API call, so a
// heap isn't worth the complexity.
type ttlCache[T any] struct {
	mu  sync.Mutex
	ttl time.Duration
	max int
	now func() time.Time
	m   map[string]entry[T]
}

type entry[T any] struct {
	val T
	exp time.Time
}

// newTTLCache creates a cache with the given TTL and max entry count. now
// defaults to time.Now when nil; max defaults to 512 when <= 0.
func newTTLCache[T any](ttl time.Duration, max int, now func() time.Time) *ttlCache[T] {
	if now == nil {
		now = time.Now
	}
	if max <= 0 {
		max = 512
	}
	return &ttlCache[T]{
		ttl: ttl,
		max: max,
		now: now,
		m:   make(map[string]entry[T]),
	}
}

// Get returns the cached value for k, deleting it first if it has expired.
func (c *ttlCache[T]) Get(k string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.m[k]
	if !ok {
		var zero T
		return zero, false
	}
	if c.now().After(e.exp) {
		delete(c.m, k)
		var zero T
		return zero, false
	}
	return e.val, true
}

// Put stores v under k, evicting the soonest-expiring entry first if the
// cache is at its cap and k is a new key.
func (c *ttlCache[T]) Put(k string, v T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.m[k]; !exists && len(c.m) >= c.max {
		c.evictSoonestLocked()
	}
	c.m[k] = entry[T]{val: v, exp: c.now().Add(c.ttl)}
}

// Clear removes every entry. Used to flush the autosuggest cache on API key
// rotation — forward/reverse results are key-independent facts about the
// world and are left alone.
func (c *ttlCache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = make(map[string]entry[T])
}

func (c *ttlCache[T]) evictSoonestLocked() {
	var soonestKey string
	var soonest time.Time
	first := true
	for k, e := range c.m {
		if first || e.exp.Before(soonest) {
			soonest, soonestKey, first = e.exp, k, false
		}
	}
	if !first {
		delete(c.m, soonestKey)
	}
}
