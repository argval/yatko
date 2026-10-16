package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/gin-gonic/gin"
)

const visitorReleaseJSON = `{"tag_name":"v1.0.0","published_at":"2026-01-01T00:00:00Z","html_url":"https://github.com/acme/tool/releases/tag/v1.0.0","assets":[{"name":"tool-linux-amd64.tar.gz","browser_download_url":"https://example.com/linux.tgz","size":10,"download_count":1},{"name":"tool-windows-amd64.zip","browser_download_url":"https://example.com/win.zip","size":11,"download_count":1},{"name":"tool-darwin-arm64.dmg","browser_download_url":"https://example.com/mac.dmg","size":12,"download_count":1}]}`

func visitorRouter(t *testing.T) *gin.Engine {
	t.Helper()
	t.Setenv("REDIS_URL", "")
	t.Setenv("KV_URL", "")
	t.Setenv("UPSTASH_REDIS_URL", "")

	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/repos/acme/tool/releases/latest" {
			t.Fatalf("unexpected GitHub request: %s", r.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(visitorReleaseJSON)),
			Request:    r,
		}, nil
	})

	gh := github.NewClient()
	c := cache.New()
	redirect := NewRedirectHandler(gh, c)
	router := gin.New()
	router.GET("/dl/:owner/:repo", redirect.Handle)
	router.GET("/api/link/:owner/:repo", NewLinkHandler(redirect).Handle)
	return router
}

func TestDownloadAndLink_CurlFallsBackToLinuxAmd64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := visitorRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/dl/acme/tool", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusFound {
		t.Fatalf("/dl status = %d, want 302", response.Code)
	}
	if loc := response.Header().Get("Location"); loc != "https://example.com/linux.tgz" {
		t.Fatalf("/dl Location = %q, want linux asset", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/link/acme/tool", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("/api/link status = %d body = %s", response.Code, response.Body.String())
	}
	var link LinkResponse
	if err := json.Unmarshal(response.Body.Bytes(), &link); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if link.Filename != "tool-linux-amd64.tar.gz" || link.Platform != "linux" || link.Arch != "amd64" {
		t.Fatalf("link = %+v, want linux/amd64 tarball", link)
	}
	if link.PlatformSource != "default" || link.ArchSource != "default" {
		t.Fatalf("sources = %s/%s, want default/default", link.PlatformSource, link.ArchSource)
	}
	if link.Error != "" {
		t.Fatalf("unexpected error field: %q", link.Error)
	}
}

func TestDownloadAndLink_QueryOverridesCurlDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := visitorRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/dl/acme/tool?platform=macos&arch=arm64", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "https://example.com/mac.dmg" {
		t.Fatalf("/dl override = %d %q", response.Code, response.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/link/acme/tool?platform=windows&arch=amd64", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	var link LinkResponse
	if err := json.Unmarshal(response.Body.Bytes(), &link); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || link.Filename != "tool-windows-amd64.zip" {
		t.Fatalf("link override = %d %+v", response.Code, link)
	}
	if link.PlatformSource != "query" || link.ArchSource != "query" {
		t.Fatalf("sources = %s/%s, want query/query", link.PlatformSource, link.ArchSource)
	}
}

func TestDownloadAndLink_CurlMissRedirectsToGitHub(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("REDIS_URL", "")
	t.Setenv("KV_URL", "")
	t.Setenv("UPSTASH_REDIS_URL", "")

	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"tag_name":"v1.0.0","html_url":"https://github.com/acme/iosapp/releases/tag/v1.0.0","assets":[{"name":"app.ipa","browser_download_url":"https://example.com/app.ipa","size":1}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})

	gh := github.NewClient()
	c := cache.New()
	redirect := NewRedirectHandler(gh, c)
	router := gin.New()
	router.GET("/dl/:owner/:repo", redirect.Handle)
	router.GET("/api/link/:owner/:repo", NewLinkHandler(redirect).Handle)

	req := httptest.NewRequest(http.MethodGet, "/dl/acme/iosapp", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusFound {
		t.Fatalf("/dl miss status = %d, want 302", response.Code)
	}
	if loc := response.Header().Get("Location"); loc != "https://github.com/acme/iosapp/releases/tag/v1.0.0" {
		t.Fatalf("/dl miss Location = %q", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/link/acme/iosapp", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("/api/link miss status = %d body = %s", response.Code, response.Body.String())
	}
	var link LinkResponse
	if err := json.Unmarshal(response.Body.Bytes(), &link); err != nil {
		t.Fatal(err)
	}
	if link.Error == "" || link.URL != "https://github.com/acme/iosapp/releases/tag/v1.0.0" {
		t.Fatalf("miss payload = %+v", link)
	}
	if link.Platform != "linux" || link.Arch != "amd64" || link.PlatformSource != "default" {
		t.Fatalf("miss still reports chosen platform: %+v", link)
	}
	if link.Filename != "" {
		t.Fatalf("miss should not claim a filename, got %q", link.Filename)
	}
}
