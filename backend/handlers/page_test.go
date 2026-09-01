package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/gin-gonic/gin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestPageHandlerDefersNonCriticalData(t *testing.T) {
	t.Setenv("REDIS_URL", "")
	t.Setenv("KV_URL", "")
	t.Setenv("UPSTASH_REDIS_URL", "")
	t.Setenv("CACHE_REFRESH_SECRET", "refresh")

	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	nonCriticalRequested := make(chan string, 1)
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/repos/acme/tool/releases/latest":
			body = `{"tag_name":"v1.0.0","published_at":"2026-01-01T00:00:00Z","html_url":"https://github.com/acme/tool/releases/tag/v1.0.0","assets":[]}`
		case "/repos/acme/tool", "/repos/acme/tool/releases":
			nonCriticalRequested <- r.URL.Path
			body = `{}`
		default:
			t.Fatalf("unexpected GitHub request: %s", r.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})

	gh := github.NewClient()
	c := cache.New()
	page := NewPageHandler(NewRedirectHandler(gh, c), gh, c)
	router := gin.New()
	router.GET("/api/release/:owner/:repo", page.Handle)
	router.GET("/api/repo/:owner/:repo", page.HandleRepoMeta)

	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/release/acme/tool", nil))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		<-done
		t.Fatal("release page waited for non-critical GitHub data")
	}

	select {
	case path := <-nonCriticalRequested:
		t.Fatalf("release page requested non-critical GitHub data: %s", path)
	default:
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got, want := response.Header().Get("Cache-Control"), "public, max-age=0, s-maxage=3600, stale-while-revalidate=82800"; got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
	if bytes.Contains(response.Body.Bytes(), []byte(`"releases"`)) ||
		bytes.Contains(response.Body.Bytes(), []byte(`"description"`)) ||
		bytes.Contains(response.Body.Bytes(), []byte(`"avatar_url"`)) {
		t.Fatal("release payload unexpectedly includes non-critical data")
	}

	metaResponse := httptest.NewRecorder()
	router.ServeHTTP(metaResponse, httptest.NewRequest(http.MethodGet, "/api/repo/acme/tool", nil))
	if metaResponse.Code != http.StatusOK {
		t.Fatalf("metadata status = %d, want %d", metaResponse.Code, http.StatusOK)
	}
	if !bytes.Contains(metaResponse.Body.Bytes(), []byte(`"description"`)) {
		t.Fatal("metadata payload omitted repo description")
	}
	select {
	case path := <-nonCriticalRequested:
		if path != "/repos/acme/tool" {
			t.Fatalf("metadata requested %s, want repo metadata", path)
		}
	default:
		t.Fatal("metadata endpoint did not request GitHub repo metadata")
	}

	refreshResponse := httptest.NewRecorder()
	router.ServeHTTP(refreshResponse, httptest.NewRequest(http.MethodGet, "/api/release/acme/tool?refresh=refresh", nil))
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want %d", refreshResponse.Code, http.StatusOK)
	}
	if got, want := refreshResponse.Header().Get("Cache-Control"), "private, no-store"; got != want {
		t.Errorf("refresh Cache-Control = %q, want %q", got, want)
	}
}
