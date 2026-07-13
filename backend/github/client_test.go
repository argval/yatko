package github

import (
	"errors"
	"net/http"
	"testing"
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
	err := c.checkBudget()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 APIError below reserve, got %v", err)
	}
}

func TestRecordRateLimit(t *testing.T) {
	c := &Client{remaining: -1}

	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "42")
	c.recordRateLimit(h)
	if c.remaining != 42 {
		t.Fatalf("got remaining %d, want 42", c.remaining)
	}

	// A response missing the header shouldn't clobber the last known value.
	c.recordRateLimit(http.Header{})
	if c.remaining != 42 {
		t.Fatalf("missing header changed remaining to %d, want unchanged 42", c.remaining)
	}
}
