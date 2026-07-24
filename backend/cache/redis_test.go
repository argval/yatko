package cache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestCache(t *testing.T, softTTL time.Duration) *Cache {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &Cache{client: client, softTTL: softTTL, hardTTL: HardTTL, l1: newL1(l1MaxEntries)}
}

func TestFetchCached_PopulatesOnMiss(t *testing.T) {
	c := newTestCache(t, time.Minute)
	ctx := context.Background()
	var calls int32

	got, err := FetchCached(ctx, c, "k", func(_ context.Context, etag string) (string, string, bool, error) {
		atomic.AddInt32(&calls, 1)
		if etag != "" {
			t.Fatalf("expected empty etag on first fetch, got %q", etag)
		}
		return "hello", "etag-1", false, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
	if calls != 1 {
		t.Fatalf("expected 1 fetch, got %d", calls)
	}
}

func TestFetchCached_ServesFreshValueWithoutFetching(t *testing.T) {
	c := newTestCache(t, time.Hour) // long soft TTL: second call should hit cache
	ctx := context.Background()
	var calls int32

	fetch := func(_ context.Context, etag string) (string, string, bool, error) {
		atomic.AddInt32(&calls, 1)
		return "v1", "etag-1", false, nil
	}

	if _, err := FetchCached(ctx, c, "k", fetch); err != nil {
		t.Fatalf("first call: %v", err)
	}
	got, err := FetchCached(ctx, c, "k", fetch)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got != "v1" {
		t.Fatalf("got %q, want %q", got, "v1")
	}
	if calls != 1 {
		t.Fatalf("expected value to be served from cache without a second fetch, got %d calls", calls)
	}
}

func TestFetchCached_ServesStaleAndRevalidatesInBackground(t *testing.T) {
	c := newTestCache(t, time.Millisecond) // expires almost immediately
	ctx := context.Background()
	var sawETag atomic.Value

	if _, err := FetchCached(ctx, c, "k", func(_ context.Context, etag string) (string, string, bool, error) {
		return "v1", "etag-1", false, nil
	}); err != nil {
		t.Fatalf("first call: %v", err)
	}

	time.Sleep(5 * time.Millisecond) // let the soft TTL lapse

	start := time.Now()
	got, err := FetchCached(ctx, c, "k", func(_ context.Context, etag string) (string, string, bool, error) {
		sawETag.Store(etag)
		time.Sleep(50 * time.Millisecond) // slow origin — caller must not wait
		return "", "", true, nil          // simulate GitHub responding 304 Not Modified
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got != "v1" {
		t.Fatalf("expected stale value immediately, got %q", got)
	}
	if elapsed > 25*time.Millisecond {
		t.Fatalf("stale hit blocked for %s; want immediate return", elapsed)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if v, ok := sawETag.Load().(string); ok && v == "etag-1" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected background revalidation to send etag %q", "etag-1")
}

func TestFetchCached_StaleWhileError(t *testing.T) {
	c := newTestCache(t, time.Millisecond)
	ctx := context.Background()

	if _, err := FetchCached(ctx, c, "k", func(_ context.Context, etag string) (string, string, bool, error) {
		return "good-value", "etag-1", false, nil
	}); err != nil {
		t.Fatalf("first call: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	// Soft-TTL expiry serves stale immediately; background revalidation fails.
	got, err := FetchCached(ctx, c, "k", func(_ context.Context, etag string) (string, string, bool, error) {
		return "", "", false, errors.New("github is down")
	})
	if err != nil {
		t.Fatalf("expected stale value instead of error, got error: %v", err)
	}
	if got != "good-value" {
		t.Fatalf("got %q, want stale value %q", got, "good-value")
	}
	// Let the background attempt finish so it doesn't race test teardown.
	time.Sleep(20 * time.Millisecond)
}

func TestFetchCached_PropagatesErrorWithNoCachedValue(t *testing.T) {
	c := newTestCache(t, time.Minute)
	ctx := context.Background()
	wantErr := errors.New("boom")

	_, err := FetchCached(ctx, c, "k", func(_ context.Context, etag string) (string, string, bool, error) {
		return "", "", false, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}
}

func TestFetchCached_CoalescesConcurrentMisses(t *testing.T) {
	c := newTestCache(t, time.Minute)
	ctx := context.Background()
	var calls int32
	release := make(chan struct{})

	fetch := func(_ context.Context, etag string) (string, string, bool, error) {
		atomic.AddInt32(&calls, 1)
		<-release // hold every concurrent caller here until we let them all through together
		return "value", "etag-1", false, nil
	}

	const n = 20
	results := make(chan string, n)
	for i := 0; i < n; i++ {
		go func() {
			v, err := FetchCached(ctx, c, "same-key", fetch)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			results <- v
		}()
	}

	time.Sleep(20 * time.Millisecond) // let all goroutines pile up on singleflight
	close(release)

	for i := 0; i < n; i++ {
		if got := <-results; got != "value" {
			t.Fatalf("got %q, want %q", got, "value")
		}
	}
	if calls != 1 {
		t.Fatalf("expected concurrent misses to be coalesced into 1 fetch, got %d", calls)
	}
}

func TestFetchCached_L1ServesWithoutRedisRoundTrip(t *testing.T) {
	c := newTestCache(t, time.Hour)
	ctx := context.Background()
	var calls int32

	fetch := func(_ context.Context, etag string) (string, string, bool, error) {
		atomic.AddInt32(&calls, 1)
		return "v1", "etag-1", false, nil
	}
	if _, err := FetchCached(ctx, c, "hot", fetch); err != nil {
		t.Fatalf("populate: %v", err)
	}

	// Drop Redis contents — L1 should still serve.
	if err := c.client.FlushAll(ctx).Err(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got, err := FetchCached(ctx, c, "hot", fetch)
	if err != nil {
		t.Fatalf("L1 hit: %v", err)
	}
	if got != "v1" {
		t.Fatalf("got %q, want v1", got)
	}
	if calls != 1 {
		t.Fatalf("expected no origin fetch after L1 hit, got %d calls", calls)
	}
}

func TestFetchCached_L1WorksWithoutRedis(t *testing.T) {
	c := &Cache{softTTL: time.Hour, hardTTL: HardTTL, l1: newL1(16)}
	ctx := context.Background()
	var calls int32

	fetch := func(_ context.Context, etag string) (string, string, bool, error) {
		atomic.AddInt32(&calls, 1)
		return "local", "e1", false, nil
	}
	if _, err := FetchCached(ctx, c, "k", fetch); err != nil {
		t.Fatalf("first: %v", err)
	}
	got, err := FetchCached(ctx, c, "k", fetch)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if got != "local" {
		t.Fatalf("got %q", got)
	}
	if calls != 1 {
		t.Fatalf("expected L1-only second hit, got %d calls", calls)
	}
}

func TestSetPermanent_NoExpiryRoundTrip(t *testing.T) {
	c := newTestCache(t, time.Minute)
	ctx := context.Background()
	if err := c.SetPermanent(ctx, "archive:repo:o/r", []byte(`{"tag":"1"}`)); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetPermanent(ctx, "archive:repo:o/r")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"tag":"1"}` {
		t.Fatalf("got %s", got)
	}
	miss, err := c.GetPermanent(ctx, "missing")
	if err != nil || miss != nil {
		t.Fatalf("miss = %v err=%v", miss, err)
	}
}
