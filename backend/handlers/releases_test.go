package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/gin-gonic/gin"
)

func TestReleasesHandlerSetsEdgeCacheControl(t *testing.T) {
	t.Setenv("REDIS_URL", "")
	t.Setenv("KV_URL", "")
	t.Setenv("UPSTASH_REDIS_URL", "")

	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/repos/acme/tool/releases" {
			t.Fatalf("unexpected GitHub request: %s", r.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`[]`)),
			Request:    r,
		}, nil
	})

	router := gin.New()
	router.GET("/api/releases/:owner/:repo", NewReleasesHandler(github.NewClient(), cache.New()).Handle)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/releases/acme/tool", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got, want := response.Header().Get("Cache-Control"), "public, max-age=0, s-maxage=3600, stale-while-revalidate=82800"; got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}
