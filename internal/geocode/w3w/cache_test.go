package w3w

import (
	"sync"
	"testing"
	"time"
)

func TestTTLCacheHitBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newTTLCache[string](time.Minute, 0, func() time.Time { return now })

	c.Put("k", "v")
	got, ok := c.Get("k")
	if !ok || got != "v" {
		t.Fatalf("Get() = %q, %v, want %q, true", got, ok, "v")
	}

	now = now.Add(59 * time.Second)
	got, ok = c.Get("k")
	if !ok || got != "v" {
		t.Fatalf("Get() just before expiry = %q, %v, want %q, true", got, ok, "v")
	}
}

func TestTTLCacheMissAfterExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newTTLCache[string](time.Minute, 0, func() time.Time { return now })

	c.Put("k", "v")
	now = now.Add(61 * time.Second)

	if _, ok := c.Get("k"); ok {
		t.Fatal("Get() after TTL expiry: got hit, want miss")
	}
}

func TestTTLCacheExpiredEntryDeletedOnRead(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newTTLCache[string](time.Minute, 0, func() time.Time { return now })

	c.Put("k", "v")
	now = now.Add(2 * time.Minute)
	c.Get("k") // triggers deletion

	c.mu.Lock()
	_, stillPresent := c.m["k"]
	c.mu.Unlock()
	if stillPresent {
		t.Fatal("expired entry was not deleted from the underlying map on read")
	}
}

func TestTTLCacheEvictsSoonestExpiringAtMax(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newTTLCache[string](time.Minute, 2, func() time.Time { return now })

	c.Put("a", "1") // expires now+1m
	now = now.Add(10 * time.Second)
	c.Put("b", "2") // expires now+1m10s — soonest is still "a"
	now = now.Add(10 * time.Second)
	c.Put("c", "3") // at cap (2): evicts "a" (soonest expiry), then inserts "c"

	if _, ok := c.Get("a"); ok {
		t.Error("expected \"a\" to have been evicted as the soonest-expiring entry")
	}
	if _, ok := c.Get("b"); !ok {
		t.Error("expected \"b\" to survive eviction")
	}
	if _, ok := c.Get("c"); !ok {
		t.Error("expected \"c\" to have been inserted")
	}
}

func TestTTLCacheClearRemovesEverything(t *testing.T) {
	now := time.Now()
	c := newTTLCache[string](time.Minute, 0, func() time.Time { return now })
	c.Put("a", "1")
	c.Put("b", "2")
	c.Clear()
	if _, ok := c.Get("a"); ok {
		t.Error("Get(\"a\") after Clear: got hit, want miss")
	}
	if _, ok := c.Get("b"); ok {
		t.Error("Get(\"b\") after Clear: got hit, want miss")
	}
}

func TestTTLCacheConcurrentAccess(t *testing.T) {
	c := newTTLCache[int](time.Minute, 100, time.Now)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			c.Put("k", i)
		}(i)
		go func() {
			defer wg.Done()
			c.Get("k")
		}()
	}
	wg.Wait()
}
