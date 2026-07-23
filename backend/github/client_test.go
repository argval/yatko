package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestCheckSearchBudget(t *testing.T) {
	c := &Client{searchRemaining: -1}
	if err := c.checkSearchBudget(); err != nil {
		t.Fatalf("expected no error when search budget unknown, got %v", err)
	}

	c.searchRemaining = searchRateLimitReserve + 1
	if err := c.checkSearchBudget(); err != nil {
		t.Fatalf("expected no error above search reserve, got %v", err)
	}

	c.searchRemaining = searchRateLimitReserve - 1
	c.searchResetAt = time.Now().Add(time.Hour).Unix()
	err := c.checkSearchBudget()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 APIError below search reserve, got %v", err)
	}
}

func TestCheckSearchBudget_RecoversAfterReset(t *testing.T) {
	c := &Client{
		searchRemaining: searchRateLimitReserve - 1,
		searchResetAt:   time.Now().Add(-time.Second).Unix(),
	}
	if err := c.checkSearchBudget(); err != nil {
		t.Fatalf("expected search budget to clear after reset, got %v", err)
	}
	if got := atomic.LoadInt32(&c.searchRemaining); got != -1 {
		t.Fatalf("expected searchRemaining cleared to -1 after reset, got %d", got)
	}
}

func TestSearchRepositories(t *testing.T) {
	var sawIfNoneMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "ripgrep" {
			t.Errorf("unexpected q %q", r.URL.Query().Get("q"))
		}
		sawIfNoneMatch = r.Header.Get("If-None-Match")
		if sawIfNoneMatch == `"abc"` {
			w.Header().Set("X-RateLimit-Remaining", "20")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("X-RateLimit-Remaining", "25")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"items": [{
				"name": "ripgrep",
				"full_name": "BurntSushi/ripgrep",
				"description": "regex search",
				"stargazers_count": 42000,
				"owner": {"login": "BurntSushi", "avatar_url": "https://avatars.example/bs"}
			}, {
				"name": "",
				"owner": {"login": "skip-me"}
			}]
		}`))
	}))
	defer srv.Close()

	c := &Client{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
				return http.DefaultTransport.RoundTrip(req)
			}),
		},
		remaining:       -1,
		searchRemaining: -1,
	}

	items, etag, notMod, err := c.SearchRepositories(context.Background(), "ripgrep", "")
	if err != nil {
		t.Fatalf("SearchRepositories: %v", err)
	}
	if notMod {
		t.Fatal("expected modified on first fetch")
	}
	if etag != `"abc"` {
		t.Fatalf("etag = %q, want \"abc\"", etag)
	}
	if len(items) != 1 {
		t.Fatalf("len(items)=%d, want 1 (empty name skipped)", len(items))
	}
	if items[0].Owner != "BurntSushi" || items[0].Repo != "ripgrep" || items[0].Stars != 42000 {
		t.Fatalf("unexpected item %+v", items[0])
	}
	if items[0].Description != "regex search" || items[0].AvatarURL != "https://avatars.example/bs" {
		t.Fatalf("unexpected description/avatar %+v", items[0])
	}

	// Core rate-limit budget must stay untouched by search responses.
	if got := atomic.LoadInt32(&c.remaining); got != -1 {
		t.Fatalf("search poisoned core remaining: got %d", got)
	}
	if got := atomic.LoadInt32(&c.searchRemaining); got != 25 {
		t.Fatalf("searchRemaining = %d, want 25", got)
	}

	_, _, notMod, err = c.SearchRepositories(context.Background(), "ripgrep", `"abc"`)
	if err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	if !notMod {
		t.Fatal("expected notModified on etag match")
	}
	if sawIfNoneMatch != `"abc"` {
		t.Fatalf("If-None-Match = %q, want \"abc\"", sawIfNoneMatch)
	}
	if got := atomic.LoadInt32(&c.searchRemaining); got != 20 {
		t.Fatalf("searchRemaining after 304 = %d, want 20", got)
	}
}

func TestSearchRepositories_RefusesWhenBudgetExhausted(t *testing.T) {
	c := &Client{
		searchRemaining: searchRateLimitReserve - 1,
		searchResetAt:   time.Now().Add(time.Hour).Unix(),
	}
	_, _, _, err := c.SearchRepositories(context.Background(), "ripgrep", "")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 when search budget exhausted, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
