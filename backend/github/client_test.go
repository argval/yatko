package github

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckBudget(t *testing.T) {
	c := &Client{remaining: -1}
	if err := c.checkBudget(); err != nil {
		t.Fatalf("expected no error when budget unknown, got %v", err)
	}

	c.remaining = rateLimitReserve + 1
	if err := c.checkBudget(); err != nil {
		t.Fatalf("expected no error above reserve, got %v", err)
	}

	c.remaining = rateLimitReserve - 1
	c.resetAt = time.Now().Add(time.Hour).Unix()
	err := c.checkBudget()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 APIError below reserve, got %v", err)
	}
}

func TestCheckBudget_RecoversAfterReset(t *testing.T) {
	c := &Client{
		remaining: rateLimitReserve - 1,
		resetAt:   time.Now().Add(-time.Second).Unix(),
	}
	if err := c.checkBudget(); err != nil {
		t.Fatalf("expected budget to clear after reset, got %v", err)
	}
	if got := atomic.LoadInt32(&c.remaining); got != -1 {
		t.Fatalf("expected remaining cleared to -1 after reset, got %d", got)
	}
	if got := atomic.LoadInt64(&c.resetAt); got != 0 {
		t.Fatalf("expected resetAt cleared to 0 after reset, got %d", got)
	}
}

func TestCheckBudget_StillBlocksBeforeReset(t *testing.T) {
	c := &Client{
		remaining: rateLimitReserve - 1,
		resetAt:   time.Now().Add(time.Hour).Unix(),
	}
	err := c.checkBudget()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 before reset, got %v", err)
	}
	if got := atomic.LoadInt32(&c.remaining); got != rateLimitReserve-1 {
		t.Fatalf("remaining should stay below reserve before reset, got %d", got)
	}
}

func TestRecordRateLimit(t *testing.T) {
	c := &Client{remaining: -1}

	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "42")
	h.Set("X-RateLimit-Reset", "1700000000")
	c.recordRateLimit(h)
	if c.remaining != 42 {
		t.Fatalf("got remaining %d, want 42", c.remaining)
	}
	if c.resetAt != 1700000000 {
		t.Fatalf("got resetAt %d, want 1700000000", c.resetAt)
	}

	// A response missing the headers shouldn't clobber the last known values.
	c.recordRateLimit(http.Header{})
	if c.remaining != 42 {
		t.Fatalf("missing header changed remaining to %d, want unchanged 42", c.remaining)
	}
	if c.resetAt != 1700000000 {
		t.Fatalf("missing header changed resetAt to %d, want unchanged 1700000000", c.resetAt)
	}
}
