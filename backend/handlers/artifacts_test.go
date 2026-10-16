package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

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
		bad := request(path + "?platform=windows&arch=386") // 32-bit Windows cannot run x64
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

func TestFailedContentInspectionBlocksPublisherPick(t *testing.T) {
	publisher := github.ArtifactMetadata{Role: "application", Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "android", Arch: "arm64"}}}
	observed := mergeObservations(publisher, github.ArtifactMetadata{Role: "unknown", Status: "failed"})
	if observed.Status != "failed" {
		t.Fatalf("failure discarded: %+v", observed)
	}
	asset := github.Asset{Name: "tool-android-arm64.apk", Artifact: &observed}
	if pick := buildReleasePicks([]github.Asset{asset})["android/arm64"]; pick.Filename != "" || pick.Status != "unknown" {
		t.Fatalf("failed APK became a download: %+v", pick)
	}
}

// shellcheck v0.11.0: the only OS-unknown asset (a Windows zip) sorts last and
// must still win the probe budget over assets that only lack a CPU.
func TestInspectionRankProbesUnknownOSFirst(t *testing.T) {
	assets := []github.Asset{
		{Name: "shellcheck-v0.11.0.darwin.tar.xz"},
		{Name: "shellcheck-v0.11.0.linux.tar.xz"},
		{Name: "App-android-arm64.apk"},
		{Name: "shellcheck-v0.11.0.zip"},
		{Name: "shellcheck-v0.11.0.linux.x86_64.tar.xz"},
		{Name: "shellcheck-v0.11.0-src.tar.gz"},
	}
	want := map[string]int{"shellcheck-v0.11.0.zip": 0, "App-android-arm64.apk": 1, "shellcheck-v0.11.0.darwin.tar.xz": 2, "shellcheck-v0.11.0.linux.tar.xz": 2}
	for _, a := range assets {
		rank, ok := inspectionRank(a)
		if w, expected := want[a.Name]; ok != expected || rank != w {
			t.Errorf("%s: rank=%d ok=%v, want rank=%d ok=%v", a.Name, rank, ok, w, expected)
		}
	}
}

// A probe cut off by the request deadline says nothing about the file, so it
// must not mark the asset failed and hide a filename-valid download.
func TestInspectionTimeoutKeepsFilenameFacts(t *testing.T) {
	t.Setenv("REDIS_URL", "")
	t.Setenv("KV_URL", "")
	t.Setenv("UPSTASH_REDIS_URL", "")
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	h := NewRedirectHandler(github.NewClient(), cache.New())
	assets := []github.Asset{{Name: "tool-android-arm64.apk", State: "uploaded", Size: 1 << 20,
		BrowserDownloadURL: "https://github.com/acme/tool/releases/download/v1/tool-android-arm64.apk"}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	h.enrichArtifacts(ctx, assets)
	if assets[0].Artifact.Status == "failed" {
		t.Fatalf("timeout recorded as failed: %+v", assets[0].Artifact)
	}
	if pick := buildReleasePicks(assets)["android/arm64"]; pick.Filename != "tool-android-arm64.apk" {
		t.Fatalf("timed-out APK hidden: %+v", pick)
	}
}

// Cache hits must not spend the live-probe budget; otherwise the same first
// four uncertain assets win on every request and later ones are never inspected.
func TestCachedInspectionsDoNotSpendProbeBudget(t *testing.T) {
	t.Setenv("REDIS_URL", "") // the in-process L1 cache is enough here
	t.Setenv("KV_URL", "")
	t.Setenv("UPSTASH_REDIS_URL", "")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if w, err := zw.Create("README.txt"); err != nil {
		t.Fatal(err)
	} else {
		w.Write([]byte("docs"))
	}
	zw.Close()
	payload := buf.Bytes()
	probed := map[string]bool{}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		probed[path.Base(r.URL.Path)] = true
		start, end := 0, len(payload)-1
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		end = min(end, len(payload)-1)
		return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))}},
			Body: io.NopCloser(bytes.NewReader(payload[start : end+1])), ContentLength: int64(end - start + 1), Request: r}, nil
	})
	h := NewRedirectHandler(github.NewClient(), cache.New())
	newAssets := func() []github.Asset {
		var out []github.Asset
		for i := range 6 {
			name := fmt.Sprintf("bundle%d.zip", i)
			out = append(out, github.Asset{ID: int64(i + 1), Name: name, State: "uploaded", Size: int64(len(payload)),
				BrowserDownloadURL: "https://github.com/acme/tool/releases/download/v1/" + name})
		}
		return out
	}
	h.enrichArtifacts(context.Background(), newAssets())
	if len(probed) != 4 {
		t.Fatalf("first request probed %d assets, want 4", len(probed))
	}
	h.enrichArtifacts(context.Background(), newAssets())
	if len(probed) != 6 {
		t.Fatalf("second request reached %d of 6 assets: %v", len(probed), probed)
	}
}
