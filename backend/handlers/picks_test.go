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
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

func TestBuildReleasePicks(t *testing.T) {
	assets := []github.Asset{
		{Name: "tool-darwin-arm64.dmg", BrowserDownloadURL: "https://example.com/arm64.dmg", Size: 10},
		{Name: "tool-darwin-amd64.dmg", BrowserDownloadURL: "https://example.com/amd64.dmg", Size: 11},
		{Name: "tool-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux.tgz", Size: 12},
		{Name: "tool-windows-amd64.msi", BrowserDownloadURL: "https://example.com/win.msi", Size: 13},
	}
	picks := buildReleasePicks(assets)
	if picks == nil {
		t.Fatal("expected picks")
	}
	got := picks[releasePickKey(picker.MacOS, picker.ARM64)]
	if got.Filename != "tool-darwin-arm64.dmg" {
		t.Fatalf("macos/arm64 = %q, want tool-darwin-arm64.dmg", got.Filename)
	}
	if _, ok := picks[releasePickKey(picker.IOS, picker.ARM64)]; ok {
		t.Fatal("ios should abstain when no iOS asset exists")
	}
	if buildReleasePicks(nil) != nil {
		t.Fatal("empty assets should yield nil picks")
	}
}

func TestPageHandlerIncludesPicks(t *testing.T) {
	t.Setenv("REDIS_URL", "")
	t.Setenv("KV_URL", "")
	t.Setenv("UPSTASH_REDIS_URL", "")

	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"tag_name":"v1.0.0","published_at":"2026-01-01T00:00:00Z","html_url":"https://github.com/acme/tool/releases/tag/v1.0.0","assets":[{"name":"tool-darwin-arm64.dmg","browser_download_url":"https://example.com/a.dmg","size":42,"download_count":1}]}`
		if r.URL.Path != "/repos/acme/tool/releases/latest" {
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

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/release/acme/tool", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var body struct {
		Picks map[string]releasePick `json:"picks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := body.Picks["macos/arm64"]
	if !ok || got.Filename != "tool-darwin-arm64.dmg" {
		t.Fatalf("picks[macos/arm64] = %+v, want tool-darwin-arm64.dmg", got)
	}
}

func TestPageHandlerFallsBackToPrereleaseOnlyRepo(t *testing.T) {
	t.Setenv("REDIS_URL", "")
	t.Setenv("KV_URL", "")
	t.Setenv("UPSTASH_REDIS_URL", "")

	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/repos/acme/apkapp/releases/latest":
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
				Request:    r,
			}, nil
		case "/repos/acme/apkapp/releases":
			body := `[{"tag_name":"Beta","name":"v0.beta.0","prerelease":true,"draft":false,"published_at":"2026-09-07T13:12:10Z","html_url":"https://github.com/acme/apkapp/releases/tag/Beta","assets":[{"name":"app-debug.apk","browser_download_url":"https://example.com/app-debug.apk","size":100,"download_count":1}]}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    r,
			}, nil
		default:
			t.Fatalf("unexpected GitHub request: %s", r.URL.String())
			return nil, nil
		}
	})

	gh := github.NewClient()
	c := cache.New()
	page := NewPageHandler(NewRedirectHandler(gh, c), gh, c)
	router := gin.New()
	router.GET("/api/release/:owner/:repo", page.Handle)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/release/acme/apkapp", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}

	var body struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Picks      map[string]releasePick `json:"picks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TagName != "Beta" || !body.Prerelease {
		t.Fatalf("got tag=%q prerelease=%v, want Beta prerelease", body.TagName, body.Prerelease)
	}
	got, ok := body.Picks["android/arm64"]
	if !ok || got.Filename != "app-debug.apk" {
		t.Fatalf("picks[android/arm64] = %+v, want app-debug.apk", got)
	}
}
