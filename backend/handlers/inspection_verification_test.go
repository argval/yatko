package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

const (
	elfX86_64  = 0x3e
	elfAArch64 = 0xb7
)

// fakeELF is only a 4 KiB header-sized byte slice; nothing here can be executed.
func fakeELF(machine uint16) []byte {
	b := make([]byte, 4096)
	copy(b, "\x7fELF")
	b[4], b[5], b[6] = 2, 1, 1 // 64-bit, little endian, version 1
	binary.LittleEndian.PutUint16(b[16:], 2)
	binary.LittleEndian.PutUint16(b[18:], machine)
	return b
}

func appImageAsset(id int64, name string) github.Asset {
	return github.Asset{ID: id, Name: name, State: "uploaded", Size: 4096, UpdatedAt: "2026-10-17T12:00:00Z",
		BrowserDownloadURL: "https://github.com/acme/tool/releases/download/v1/" + name}
}

// downloadFake serves the release JSON plus ranged fake ELF headers, and
// records every asset request so tests can prove probe counts and safety.
type downloadFake struct {
	mu     sync.Mutex
	bodies map[string][]byte // asset name -> bytes
	fail   map[string]string // asset name -> "error" | "hang"
	calls  map[string]int
	noRng  []string // download requests that lacked a Range header
}

func (f *downloadFake) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[name]
}

func (f *downloadFake) probed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var names []string
	for n, c := range f.calls {
		if c > 0 {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}

func newDownloadFake(t *testing.T, assets []github.Asset, bodies map[string][]byte) *downloadFake {
	t.Helper()
	for _, key := range []string{"REDIS_URL", "KV_URL", "UPSTASH_REDIS_URL", "GITHUB_TOKEN"} {
		t.Setenv(key, "")
	}
	release, err := json.Marshal(github.Release{TagName: "v1", HTMLURL: "https://github.com/acme/tool/releases/tag/v1", Assets: assets})
	if err != nil {
		t.Fatal(err)
	}
	f := &downloadFake{bodies: bodies, fail: map[string]string{}, calls: map[string]int{}}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.github.com" && r.URL.Path == "/repos/acme/tool/releases/latest" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(release)), Request: r}, nil
		}
		name := path.Base(r.URL.Path)
		payload, ok := f.bodies[name]
		if r.URL.Host != "github.com" || !ok {
			t.Errorf("unexpected request: %s", r.URL)
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		f.mu.Lock()
		f.calls[name]++
		if r.Header.Get("Range") == "" {
			f.noRng = append(f.noRng, name)
		}
		mode := f.fail[name]
		f.mu.Unlock()
		switch mode {
		case "error":
			return nil, fmt.Errorf("connection reset")
		case "hang":
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		start, end := 0, len(payload)-1
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		end = min(end, len(payload)-1)
		return &http.Response{StatusCode: http.StatusPartialContent, Request: r, ContentLength: int64(end - start + 1),
			Header: http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))}},
			Body:   io.NopCloser(bytes.NewReader(payload[start : end+1]))}, nil
	})
	return f
}

func appImageRouter(h *RedirectHandler) *gin.Engine {
	router := gin.New()
	router.GET("/dl/:owner/:repo", h.Handle)
	return router
}

func curlDL(router *gin.Engine, query string) *httptest.ResponseRecorder {
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dl/acme/tool?"+query, nil)
	req.Header.Set("User-Agent", "curl/8.0")
	router.ServeHTTP(res, req)
	return res
}

// A launcher header decides an AppImage even when the filename names a CPU:
// a mismatch is a hard conflict for every visitor, and only ranged header
// bytes are fetched (the file is never run).
func TestAppImageLauncherIsCheckedEvenWhenFilenameNamesCPU(t *testing.T) {
	for _, tc := range []struct {
		name               string
		machine            uint16
		wantArm64, wantAmd bool
	}{
		{"Tool-1.0-arm64.AppImage", elfX86_64, false, false}, // lying filename: refuse both CPUs
		{"Tool-1.0-x86_64.AppImage", elfAArch64, false, false},
		{"Tool-1.0-arm64.AppImage", elfAArch64, true, false}, // honest file is still auto-offered
		{"Tool-1.0-x86_64.AppImage", elfX86_64, false, true},
	} {
		t.Run(fmt.Sprintf("%s/machine=%#x", tc.name, tc.machine), func(t *testing.T) {
			asset := appImageAsset(1, tc.name)
			fake := newDownloadFake(t, []github.Asset{asset}, map[string][]byte{tc.name: fakeELF(tc.machine)})
			router := appImageRouter(NewRedirectHandler(github.NewClient(), cache.New()))
			for arch, want := range map[string]bool{"arm64": tc.wantArm64, "amd64": tc.wantAmd} {
				res := curlDL(router, "platform=linux&arch="+arch)
				if got := res.Code == http.StatusFound && res.Header().Get("Location") == asset.BrowserDownloadURL; got != want {
					t.Errorf("linux/%s: code=%d location=%q, want auto=%v", arch, res.Code, res.Header().Get("Location"), want)
				}
			}
			if fake.count(tc.name) == 0 {
				t.Error("named-CPU AppImage was never content-checked")
			}
			if len(fake.noRng) != 0 {
				t.Errorf("download requested without a Range header: %v", fake.noRng)
			}
		})
	}
}

// A failed or timed-out probe leaves no launcher proof, so the AppImage must
// not become an automatic download; filename facts alone are not enough.
func TestAppImageProbeFailureLeavesFileNonAutomatic(t *testing.T) {
	for _, mode := range []string{"error", "hang"} {
		t.Run(mode, func(t *testing.T) {
			asset := appImageAsset(1, "Tool-1.0-arm64.AppImage")
			fake := newDownloadFake(t, []github.Asset{asset}, map[string][]byte{asset.Name: fakeELF(elfAArch64)})
			fake.fail[asset.Name] = mode
			h := NewRedirectHandler(github.NewClient(), cache.New())
			assets := []github.Asset{asset}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			h.enrichArtifacts(ctx, assets)
			if fake.count(asset.Name) == 0 {
				t.Fatal("precondition: the probe must have been attempted")
			}
			if m := assets[0].Artifact; m == nil || m.LauncherArch != "" || m.Status == "failed" {
				t.Fatalf("transport failure produced wrong metadata: %+v", m)
			}
			if pick := buildReleasePicks(assets)["linux/arm64"]; pick.Status == "selected" {
				t.Fatalf("unverified AppImage was selected: %+v", pick)
			}
		})
	}
}

// Only 4 live probes run per request; the rest stay non-automatic until cached
// probes (free) let later requests reach them, without re-downloading the first four.
func TestAppImageProbeCapLeavesRestUncheckedThenFills(t *testing.T) {
	var assets []github.Asset
	bodies := map[string][]byte{}
	for i := range 6 {
		a := appImageAsset(int64(i+1), fmt.Sprintf("App%d-1.0-arm64.AppImage", i))
		assets = append(assets, a)
		bodies[a.Name] = fakeELF(elfAArch64)
	}
	fake := newDownloadFake(t, assets, bodies)
	h := NewRedirectHandler(github.NewClient(), cache.New())
	verified := func(batch []github.Asset) (done, pending []string) {
		for _, a := range batch {
			facts := picker.FactsForAsset(a)
			if a.Artifact != nil && a.Artifact.LauncherArch == "arm64" && !picker.NeedsVerification(facts, picker.Linux, picker.ARM64, picker.LibcAny) {
				done = append(done, a.Name)
			} else if picker.NeedsVerification(facts, picker.Linux, picker.ARM64, picker.LibcAny) {
				pending = append(pending, a.Name)
			}
		}
		return
	}

	first := slices.Clone(assets)
	h.enrichArtifacts(context.Background(), first)
	done, pending := verified(first)
	if len(done) != 4 || len(pending) != 2 || len(fake.probed()) != 4 {
		t.Fatalf("first request: verified=%v unchecked=%v probed=%v", done, pending, fake.probed())
	}
	before := map[string]int{}
	for _, n := range done {
		before[n] = fake.count(n)
	}

	second := slices.Clone(assets)
	h.enrichArtifacts(context.Background(), second)
	if done, pending = verified(second); len(done) != 6 || len(pending) != 0 || len(fake.probed()) != 6 {
		t.Fatalf("second request: verified=%v unchecked=%v probed=%v", done, pending, fake.probed())
	}
	for n, c := range before {
		if fake.count(n) != c {
			t.Errorf("cached probe for %s was downloaded again", n)
		}
	}
}

// A pre-v4 cached observation (no proven launcher) must not stand in for the
// fresh check: here the stale entry claims arm64 but the real header is x86-64.
func TestOldContentCacheVersionDoesNotSkipVerification(t *testing.T) {
	asset := appImageAsset(1, "Tool-1.0-arm64.AppImage")
	fake := newDownloadFake(t, []github.Asset{asset}, map[string][]byte{asset.Name: fakeELF(elfX86_64)})
	h := NewRedirectHandler(github.NewClient(), cache.New())
	key := fmt.Sprintf("artifact-content:v3:%x", sha256.Sum256([]byte(artifactIdentity(asset))))
	stale := github.ArtifactMetadata{Status: "inspected", Role: "application", LauncherArch: "arm64",
		Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "arm64"}}}
	if _, err := cache.FetchCached(context.Background(), h.cache, key, func(context.Context, string) (github.ArtifactMetadata, string, bool, error) {
		return stale, "", false, nil
	}); err != nil {
		t.Fatal(err)
	}
	assets := []github.Asset{asset}
	h.enrichArtifacts(context.Background(), assets)
	if fake.count(asset.Name) == 0 {
		t.Fatal("v3 cache entry suppressed the fresh content probe")
	}
	m := assets[0].Artifact
	if m == nil || m.LauncherArch != "amd64" {
		t.Fatalf("fresh launcher not recorded: %+v", m)
	}
	if pick := buildReleasePicks(assets)["linux/arm64"]; pick.Status == "selected" {
		t.Fatalf("stale v3 arm64 claim was trusted: %+v", pick)
	}
}

// An offer issued while a file looked fine is void once the current file has
// no launcher proof, a launcher/filename conflict, or a partial archive
// without the visitor's CPU.
func TestIssuedOfferRejectsCurrentlyUnverifiedAssets(t *testing.T) {
	for _, key := range []string{"REDIS_URL", "KV_URL", "UPSTASH_REDIS_URL"} {
		t.Setenv(key, "")
	}
	h := &RedirectHandler{cache: cache.New(), Primary: &jev.Primary{}}
	contextFor := func(query string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		c.Request.Header.Set("User-Agent", "curl/8.0")
		return c
	}
	linuxTarget := func(arch string) []github.ArtifactTarget {
		return []github.ArtifactTarget{{Platform: "linux", Arch: arch}}
	}
	for _, tc := range []struct {
		name, file, arch string
		offered          github.ArtifactMetadata
		current          *github.ArtifactMetadata
		want             bool
	}{
		{"AppImage verified again", "Tool-1.0-arm64.AppImage", "arm64",
			github.ArtifactMetadata{Status: "inspected", LauncherArch: "arm64", Targets: linuxTarget("arm64")},
			&github.ArtifactMetadata{Status: "inspected", LauncherArch: "arm64", Targets: linuxTarget("arm64")}, true},
		{"AppImage now unverified", "Tool-1.0-arm64.AppImage", "arm64",
			github.ArtifactMetadata{Status: "inspected", LauncherArch: "arm64", Targets: linuxTarget("arm64")}, nil, false},
		{"AppImage launcher mismatches filename", "Tool-1.0-arm64.AppImage", "arm64",
			github.ArtifactMetadata{Status: "inspected", LauncherArch: "arm64", Targets: linuxTarget("arm64")},
			&github.ArtifactMetadata{Status: "inspected", LauncherArch: "amd64", Targets: linuxTarget("amd64")}, false},
		{"partial archive lost visitor CPU", "tool-linux.tar.gz", "arm64",
			github.ArtifactMetadata{Status: "inspected", Targets: linuxTarget("arm64")},
			&github.ArtifactMetadata{Status: "inspected", TargetsPartial: true, Targets: linuxTarget("amd64")}, false},
		{"empty partial archive cannot use filename CPU", "tool-linux-arm64.tar.gz", "arm64",
			github.ArtifactMetadata{Status: "inspected", Targets: linuxTarget("arm64")},
			&github.ArtifactMetadata{Status: "unsupported", TargetsPartial: true}, false},
		{"partial archive keeps visitor CPU", "tool-linux.tar.gz", "arm64",
			github.ArtifactMetadata{Status: "inspected", Targets: linuxTarget("arm64")},
			&github.ArtifactMetadata{Status: "inspected", TargetsPartial: true, Targets: linuxTarget("arm64")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asset := appImageAsset(1, tc.file)
			offeredAsset := asset
			offeredAsset.Artifact = &tc.offered
			c := contextFor("platform=linux&arch=" + tc.arch)
			p := resolveAssetPick(c)
			p.Decision = picker.DecideAsset([]github.Asset{offeredAsset}, p.Platform, p.Arch, p.opts())
			if !p.Decision.ShouldAutoSelect() {
				t.Fatalf("precondition: offered file must be auto-selectable: %+v", p.Decision)
			}
			h.rememberDownload(c, "acme", "tool", &github.Release{TagName: "v1", Assets: []github.Asset{offeredAsset}}, p)

			current := asset
			current.Artifact = tc.current
			c = contextFor("platform=linux&arch=" + tc.arch + "&asset=" + tc.file)
			d, ok := h.offeredDownload(c, "acme", "tool", &github.Release{TagName: "v1", Assets: []github.Asset{current}}, resolveAssetPick(c))
			if ok != tc.want || (ok && !strings.HasSuffix(d.Asset.BrowserDownloadURL, tc.file)) {
				t.Fatalf("accepted=%v want=%v decision=%+v", ok, tc.want, d)
			}
		})
	}
}
