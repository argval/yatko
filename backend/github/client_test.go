package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckBudget(t *testing.T) {
	c := &Client{token: "test-token", remaining: -1}
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
		token:     "test-token",
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
		token:     "test-token",
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
		if r.URL.Query().Get("sort") != "stars" {
			t.Errorf("unexpected sort %q", r.URL.Query().Get("sort"))
		}
		if r.URL.Query().Get("order") != "desc" {
			t.Errorf("unexpected order %q", r.URL.Query().Get("order"))
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
				"archived": false,
				"owner": {"login": "BurntSushi", "avatar_url": "https://avatars.example/bs"}
			}, {
				"name": "old-ripgrep",
				"full_name": "someone/old-ripgrep",
				"description": "archived",
				"stargazers_count": 99,
				"archived": true,
				"owner": {"login": "someone", "avatar_url": "https://avatars.example/x"}
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
		t.Fatalf("len(items)=%d, want 1 (empty name + archived skipped)", len(items))
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

func TestRepoAPIPath_EscapesSegments(t *testing.T) {
	got := repoAPIPath("acme", "app", "/releases/tags/"+url.PathEscape("v1.0?foo=bar"))
	want := "https://api.github.com/repos/acme/app/releases/tags/v1.0%3Ffoo=bar"
	if got != want {
		t.Fatalf("repoAPIPath = %q, want %q", got, want)
	}
	if got := repoAPIPath("o#wner", "re/po", ""); got != "https://api.github.com/repos/o%23wner/re%2Fpo" {
		t.Fatalf("unexpected escape of owner/repo: %q", got)
	}
}

func TestGetLatestRelease_TokenFailover(t *testing.T) {
	const primaryToken = "test-primary"
	const backupToken = "test-backup"
	const primary = "Bearer " + primaryToken
	const backup = "Bearer " + backupToken
	const anonymous = ""

	tests := []struct {
		name          string
		primaryToken  string
		backupToken   string
		primaryStatus int
		backupStatus  int
		wantStatus    int
		wantRequests  []string
	}{
		{"primary works", primaryToken, backupToken, http.StatusOK, http.StatusOK,
			http.StatusOK, []string{primary, primary}},
		{"backup works", primaryToken, backupToken, http.StatusUnauthorized, http.StatusOK,
			http.StatusOK, []string{primary, backup, backup}},
		{"both rejected", primaryToken, backupToken, http.StatusUnauthorized, http.StatusUnauthorized,
			http.StatusOK, []string{primary, backup, anonymous, anonymous}},
		{"no backup", primaryToken, "", http.StatusUnauthorized, http.StatusOK,
			http.StatusOK, []string{primary, anonymous, anonymous}},
		{"duplicate backup", primaryToken, primaryToken, http.StatusUnauthorized, http.StatusOK,
			http.StatusOK, []string{primary, anonymous, anonymous}},
		{"backup only", "", backupToken, http.StatusUnauthorized, http.StatusOK,
			http.StatusOK, []string{backup, backup}},
		{"rate limited", primaryToken, backupToken, http.StatusTooManyRequests, http.StatusOK,
			http.StatusTooManyRequests, []string{primary, primary}},
		{"forbidden", primaryToken, backupToken, http.StatusForbidden, http.StatusOK,
			http.StatusForbidden, []string{primary, primary}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", tc.primaryToken)
			t.Setenv("GITHUB_BACKUP_TOKEN", tc.backupToken)
			c := NewClient()
			var requests []string
			c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				auth := r.Header.Get("Authorization")
				requests = append(requests, auth)
				status, remaining := http.StatusOK, "58"
				switch auth {
				case primary:
					status, remaining = tc.primaryStatus, "4999"
				case backup:
					status, remaining = tc.backupStatus, "4999"
				}
				response := &http.Response{StatusCode: status, Header: make(http.Header), Body: http.NoBody}
				if status == http.StatusOK {
					response.Header.Set("X-RateLimit-Remaining", remaining)
					response.Body = io.NopCloser(strings.NewReader(`{"tag_name":"v1.0"}`))
				}
				return response, nil
			})

			for _, repo := range []string{"tool", "another-tool"} {
				release, _, _, err := c.GetLatestRelease(t.Context(), "acme", repo, "")
				if tc.wantStatus == http.StatusOK {
					if err != nil || release == nil || release.TagName != "v1.0" {
						t.Fatalf("GetLatestRelease(%q) = %v, %v, want release v1.0", repo, release, err)
					}
					continue
				}
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.wantStatus {
					t.Fatalf("GetLatestRelease(%q) error = %#v, want status %d", repo, err, tc.wantStatus)
				}
			}
			if !slices.Equal(requests, tc.wantRequests) {
				t.Fatalf("Authorization headers = %q, want %q", requests, tc.wantRequests)
			}
		})
	}
}

func TestGetLatestRelease_ConcurrentRejectionsKeepBackup(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-primary")
	t.Setenv("GITHUB_BACKUP_TOKEN", "test-backup")
	c := NewClient()
	var primaryCalls, backupCalls atomic.Int32
	bothStarted := make(chan struct{})
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.Header.Get("Authorization") {
		case "Bearer test-primary":
			if primaryCalls.Add(1) == 2 {
				close(bothStarted)
			}
			select {
			case <-bothStarted:
				return &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody}, nil
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		case "Bearer test-backup":
			backupCalls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v1.0"}`)),
			}, nil
		default:
			return &http.Response{StatusCode: http.StatusForbidden, Body: http.NoBody}, nil
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for _, repo := range []string{"tool", "another-tool"} {
		go func() {
			_, _, _, err := c.GetLatestRelease(ctx, "acme", repo, "")
			results <- err
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Errorf("release lookup failed: %v", err)
		}
	}
	if primaryCalls.Load() != 2 || backupCalls.Load() != 2 {
		t.Fatalf("want two requests to each token, got %d primary and %d backup", primaryCalls.Load(), backupCalls.Load())
	}
}

func TestMarkdownResponsesAreClipped(t *testing.T) {
	tooLong := strings.Repeat("x", 100_001)
	c := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body := tooLong
			if r.URL.Path == "/repos/acme/tool/releases/latest" {
				body = `{"body":"` + tooLong + `"}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    r,
			}, nil
		})},
		remaining: -1,
	}

	release, _, _, err := c.GetLatestRelease(context.Background(), "acme", "tool", "")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	readme, _, _, err := c.GetREADME(context.Background(), "acme", "tool", "")
	if err != nil {
		t.Fatalf("GetREADME: %v", err)
	}
	want := strings.Repeat("x", 100_000) + "\n\n…\n"
	if release.Body != want {
		t.Fatalf("release body = %d bytes, want clipped markdown", len(release.Body))
	}
	if readme != want {
		t.Fatalf("readme = %d bytes, want clipped markdown", len(readme))
	}
}

func TestGetLatestRelease_FallsBackToPrerelease(t *testing.T) {
	var paths []string
	c := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			paths = append(paths, r.URL.Path)
			switch r.URL.Path {
			case "/repos/acme/tool/releases/latest":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
					Request:    r,
				}, nil
			case "/repos/acme/tool/releases":
				body := `[{"tag_name":"Beta","name":"v0.beta.0","prerelease":true,"draft":false,"body":"notes","assets":[{"name":"app-debug.apk","browser_download_url":"https://example.com/app-debug.apk","size":100}]}]`
				h := make(http.Header)
				h.Set("ETag", `"list-1"`)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     h,
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    r,
				}, nil
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
				return nil, nil
			}
		})},
		remaining: -1,
	}

	release, etag, notModified, err := c.GetLatestRelease(context.Background(), "acme", "tool", "")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if notModified {
		t.Fatal("expected modified response")
	}
	if etag != `"list-1"` {
		t.Fatalf("etag = %q, want list etag", etag)
	}
	if release.TagName != "Beta" || !release.Prerelease {
		t.Fatalf("release = %+v, want Beta prerelease", release)
	}
	if len(release.Assets) != 1 || release.Assets[0].Name != "app-debug.apk" {
		t.Fatalf("assets = %+v, want app-debug.apk", release.Assets)
	}
	if len(paths) != 2 || paths[0] != "/repos/acme/tool/releases/latest" || paths[1] != "/repos/acme/tool/releases" {
		t.Fatalf("paths = %v, want latest then list", paths)
	}
}

func TestGetLatestRelease_FallbackSkipsDrafts(t *testing.T) {
	c := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/repos/acme/tool/releases/latest" {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
					Request:    r,
				}, nil
			}
			body := `[
				{"tag_name":"draft","draft":true,"prerelease":false,"assets":[]},
				{"tag_name":"rc1","draft":false,"prerelease":true,"assets":[{"name":"tool.apk"}]}
			]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    r,
			}, nil
		})},
		remaining: -1,
	}

	release, _, _, err := c.GetLatestRelease(context.Background(), "acme", "tool", "")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if release.TagName != "rc1" {
		t.Fatalf("tag = %q, want rc1 (skipped draft)", release.TagName)
	}
}

func TestGetLatestRelease_FallbackEmptyStillNotFound(t *testing.T) {
	c := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/repos/acme/tool/releases/latest" {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
					Request:    r,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`[{"tag_name":"wip","draft":true}]`)),
				Request:    r,
			}, nil
		})},
		remaining: -1,
	}

	_, _, _, err := c.GetLatestRelease(context.Background(), "acme", "tool", "")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when only drafts exist, got %v", err)
	}
}

func TestGetLatestRelease_FallbackRevalidatesListWithETag(t *testing.T) {
	c := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/repos/acme/tool/releases/latest" {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
					Request:    r,
				}, nil
			}
			if r.Header.Get("If-None-Match") != `"list-1"` {
				t.Fatalf("If-None-Match = %q, want list etag", r.Header.Get("If-None-Match"))
			}
			return &http.Response{
				StatusCode: http.StatusNotModified,
				Header:     make(http.Header),
				Body:       http.NoBody,
				Request:    r,
			}, nil
		})},
		remaining: -1,
	}

	release, etag, notModified, err := c.GetLatestRelease(context.Background(), "acme", "tool", `"list-1"`)
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if !notModified || release != nil || etag != `"list-1"` {
		t.Fatalf("notModified=%v release=%v etag=%q", notModified, release, etag)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
