package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestLimiter(t *testing.T) *Limiter {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &Limiter{client: client}
}

func newTestLimiterWithMiniredis(t *testing.T) (*Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &Limiter{client: client}, mr
}

func TestAllow_PermitsUnderLimit(t *testing.T) {
	l := newTestLimiter(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		allowed, _, err := l.Allow(ctx, "1.2.3.4", 3, time.Minute)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if !allowed {
			t.Fatalf("call %d: expected allowed, got blocked", i)
		}
	}
}

func TestAllow_BlocksOverLimit(t *testing.T) {
	l := newTestLimiter(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, _, err := l.Allow(ctx, "1.2.3.4", 3, time.Minute); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}

	allowed, retryAfter, err := l.Allow(ctx, "1.2.3.4", 3, time.Minute)
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
	l, mr := newTestLimiterWithMiniredis(t)
	ctx := context.Background()
	// Redis EXPIRE has a 1s minimum resolution, so the window can't go below that.
	window := time.Second

	for i := 0; i < 2; i++ {
		if _, _, err := l.Allow(ctx, "1.2.3.4", 2, window); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}

	if allowed, _, _ := l.Allow(ctx, "1.2.3.4", 2, window); allowed {
		t.Fatal("expected 3rd call within the window to be blocked")
	}

	mr.FastForward(1100 * time.Millisecond) // let the window lapse

	allowed, _, err := l.Allow(ctx, "1.2.3.4", 2, window)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected a fresh window to allow the request again")
	}
}

func TestAllow_NoopWithoutRedis(t *testing.T) {
	l := &Limiter{}
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		allowed, _, err := l.Allow(ctx, "1.2.3.4", 1, time.Minute)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if !allowed {
			t.Fatalf("call %d: expected always-allowed with no Redis configured", i)
		}
	}
}

func TestAllow_FallsBackToLocalOnRedisError(t *testing.T) {
	l, mr := newTestLimiterWithMiniredis(t)
	ctx := context.Background()
	mr.Close() // force subsequent Redis ops to fail

	for i := 0; i < 2; i++ {
		allowed, _, err := l.Allow(ctx, "9.9.9.9", 2, time.Minute)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if !allowed {
			t.Fatalf("call %d: expected local fallback to allow under limit", i)
		}
	}

	allowed, retryAfter, err := l.Allow(ctx, "9.9.9.9", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected local fallback to block over limit")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retryAfter, got %v", retryAfter)
	}
}
