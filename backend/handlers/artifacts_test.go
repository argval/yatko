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

// Publisher facts must drive the page, link API, and redirect in the same way.
func TestManifestDecisionAcrossEndpoints(t *testing.T) {
	for _, key := range []string{"REDIS_URL", "KV_URL", "UPSTASH_REDIS_URL", "GITHUB_TOKEN"} {
		t.Setenv(key, "")
	}
	const download = "https://github.com/acme/tool/releases/download/v1/opaque.zip"
	const manifestURL = "https://github.com/acme/tool/releases/download/v1/dist-manifest.json"
	manifest := `{"dist_version":"0.30.0","artifacts":{"app":{"name":"opaque.zip","kind":"executable-zip","target_triples":["x86_64-pc-windows-msvc"]}}}`
	release, err := json.Marshal(github.Release{TagName: "v1", HTMLURL: "https://github.com/acme/tool/releases/tag/v1", Assets: []github.Asset{
		{Name: "opaque.zip", State: "uploaded", Size: 100, BrowserDownloadURL: download},
		{Name: "dist-manifest.json", State: "uploaded", Size: int64(len(manifest)), BrowserDownloadURL: manifestURL},
	}})
	if err != nil {
		t.Fatal(err)
	}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := string(release)
		if r.URL.String() == manifestURL {
			body = manifest
		} else if r.URL.Host != "api.github.com" || r.URL.Path != "/repos/acme/tool/releases/latest" {
			t.Fatalf("unexpected fetch: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	gh, c := github.NewClient(), cache.New()
	h := NewRedirectHandler(gh, c)
	router := gin.New()
	router.GET("/api/release/:owner/:repo", NewPageHandler(h, gh, c).Handle)
	router.GET("/api/link/:owner/:repo", NewLinkHandler(h).Handle)
	router.GET("/dl/:owner/:repo", h.Handle)
	request := func(path string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("User-Agent", "curl/8.0")
		router.ServeHTTP(res, req)
		return res
	}
	page := request("/api/release/acme/tool")
	var payload struct {
		Assets []github.Asset         `json:"assets"`
		Picks  map[string]releasePick `json:"picks"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if got := payload.Picks["windows/amd64"]; page.Code != 200 || got.URL != download || got.Status != "selected" {
		t.Fatalf("page: %d %+v", page.Code, got)
	}
	if len(payload.Assets) == 0 || payload.Assets[0].Artifact == nil || payload.Assets[0].Artifact.Status != "inspected" {
		t.Fatalf("missing shared facts: %s", page.Body)
	}
	for _, path := range []string{"/api/link/acme/tool", "/dl/acme/tool"} {
		ok := request(path + "?platform=windows&arch=amd64")
		if path == "/dl/acme/tool" {
			if ok.Code != 302 || ok.Header().Get("Location") != download {
				t.Fatalf("redirect: %d %s", ok.Code, ok.Body)
			}
		} else {
			var link LinkResponse
			if err := json.Unmarshal(ok.Body.Bytes(), &link); err != nil || ok.Code != 200 || link.URL != download {
				t.Fatalf("link: %d %s", ok.Code, ok.Body)
			}
		}
		bad := request(path + "?platform=windows&arch=arm64")
		if bad.Code != 404 || strings.Contains(bad.Body.String(), download) {
			t.Fatalf("wrong CPU selected: %d %s", bad.Code, bad.Body)
		}
	}
}

func TestArtifactIdentityInvalidatesReplacedAssets(t *testing.T) {
	a := github.Asset{ID: 1, Name: "tool.zip", Size: 100, UpdatedAt: "yesterday", Digest: "sha256:a", BrowserDownloadURL: "https://github.com/acme/tool/releases/download/v1/tool.zip"}
	for _, change := range []func(*github.Asset){func(b *github.Asset) { b.ID++ }, func(b *github.Asset) { b.UpdatedAt = "today" }, func(b *github.Asset) { b.Digest = "sha256:b" }} {
		b := a
		change(&b)
		if artifactIdentity(a) == artifactIdentity(b) {
			t.Fatal("replaced asset reused observation identity")
		}
	}
}

func TestObservationContradictionsPreventAutomaticSelection(t *testing.T) {
	a := github.ArtifactMetadata{Role: "installer", Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "arm64"}}}
	b := github.ArtifactMetadata{Role: "application", Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "amd64"}}}
	if m := mergeObservations(a, b); len(m.Conflicts) == 0 {
		t.Fatalf("CPU contradiction discarded: %+v", m)
	}
	b.Targets[0].Arch = "arm64"
	b.Role = "metadata"
	if m := mergeObservations(a, b); len(m.Conflicts) == 0 {
		t.Fatalf("non-runnable role discarded: %+v", m)
	}
}
