package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestCacheWithMiniredis is like newTestCache but also returns the
// miniredis instance, needed to fast-forward its clock: miniredis doesn't
// tie key expiry to real wall-clock time the way Redis does.
func newTestCacheWithMiniredis(t *testing.T) (*Cache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &Cache{client: client, softTTL: time.Minute, hardTTL: HardTTL}, mr
}

func TestAllow_PermitsUnderLimit(t *testing.T) {
	c := newTestCache(t, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		allowed, _, err := c.Allow(ctx, "1.2.3.4", 3, time.Minute)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if !allowed {
			t.Fatalf("call %d: expected allowed, got blocked", i)
		}
	}
}

func TestAllow_BlocksOverLimit(t *testing.T) {
	c := newTestCache(t, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, _, err := c.Allow(ctx, "1.2.3.4", 3, time.Minute); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}

	allowed, retryAfter, err := c.Allow(ctx, "1.2.3.4", 3, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected 4th call over a limit of 3 to be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected a positive retryAfter, got %v", retryAfter)
	}
}

func TestAllow_ResetsAfterWindow(t *testing.T) {
	c, mr := newTestCacheWithMiniredis(t)
	ctx := context.Background()
	// Redis EXPIRE has a 1s minimum resolution, so the window can't go below that.
	window := time.Second

	for i := 0; i < 2; i++ {
		if _, _, err := c.Allow(ctx, "1.2.3.4", 2, window); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}

	if allowed, _, _ := c.Allow(ctx, "1.2.3.4", 2, window); allowed {
		t.Fatal("expected 3rd call within the window to be blocked")
	}

	mr.FastForward(1100 * time.Millisecond) // let the window lapse

	allowed, _, err := c.Allow(ctx, "1.2.3.4", 2, window)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected a fresh window to allow the request again")
	}
}

func TestAllow_NoopWithoutRedis(t *testing.T) {
	c := &Cache{}
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		allowed, _, err := c.Allow(ctx, "1.2.3.4", 1, time.Minute)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if !allowed {
			t.Fatalf("call %d: expected always-allowed with no Redis configured", i)
		}
	}
}
