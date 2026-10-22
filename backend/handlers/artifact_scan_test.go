package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/argval/yatko/artifactscan"
	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func scanAsset(name string) github.Asset {
	return github.Asset{ID: 10, Name: name, Size: 1024, State: "uploaded", UpdatedAt: "2026-10-17T12:00:00Z",
		BrowserDownloadURL: "https://github.com/acme/tool/releases/download/v1/" + name}
}

func scanMetadata(arch string) github.ArtifactMetadata {
	return github.ArtifactMetadata{Status: "inspected", Role: "application", Targets: []github.ArtifactTarget{{Platform: "windows", Arch: arch}},
		Evidence: []github.ArtifactEvidence{{Source: "deep-inspection", Field: "binary.entry", Value: "app.exe"}}}
}

// scanRouter uses only fixture GitHub responses; asset payload downloads fail the test.
func scanRouter(t *testing.T, assets []github.Asset, manifests map[string]string) (*RedirectHandler, *gin.Engine, *cache.Cache) {
	t.Helper()
	for _, key := range []string{"REDIS_URL", "KV_URL", "UPSTASH_REDIS_URL", "GITHUB_TOKEN"} {
		t.Setenv(key, "")
	}
	release, err := json.Marshal(github.Release{TagName: "v1", HTMLURL: "https://github.com/acme/tool/releases/tag/v1", Assets: assets})
	if err != nil {
		t.Fatal(err)
	}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := string(release)
		if manifest, found := manifests[request.URL.String()]; found {
			body = manifest
		} else if request.URL.Host != "api.github.com" || request.URL.Path != "/repos/acme/tool/releases/latest" {
			t.Fatalf("unexpected asset download on HTTP path: %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	gh, c := github.NewClient(), cache.New()
	h := NewRedirectHandler(gh, c)
	router := gin.New()
	router.GET("/api/release/:owner/:repo", NewPageHandler(h, gh, c).Handle)
	router.GET("/api/link/:owner/:repo", NewLinkHandler(h).Handle)
	router.GET("/dl/:owner/:repo", h.Handle)
	return h, router, c
}

func scanRequest(router *gin.Engine, path string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("User-Agent", "curl/8.0")
	router.ServeHTTP(response, request)
	return response
}

func completeScan(t *testing.T, store *artifactscan.Store, asset github.Asset, metadata github.ArtifactMetadata, inspectionError error) {
	t.Helper()
	if _, err := store.Enqueue(context.Background(), asset); err != nil {
		t.Fatal(err)
	}
	worked, err := store.ProcessNext(context.Background(), func(context.Context, github.Asset) (github.ArtifactMetadata, error) {
		return metadata, inspectionError
	})
	if !worked || err != nil {
		t.Fatalf("complete fixture scan: worked=%v err=%v", worked, err)
	}
}

func shallowScan(t *testing.T, c *cache.Cache, asset github.Asset, metadata github.ArtifactMetadata) {
	t.Helper()
	key := fmt.Sprintf("artifact-content:v4:%x", sha256.Sum256([]byte(artifactIdentity(asset))))
	_, err := cache.FetchCached(context.Background(), c, key, func(context.Context, string) (github.ArtifactMetadata, string, bool, error) {
		return metadata, "", false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeepPayloadEvidenceSharedAcrossEndpointsAndOverridesShallowStub(t *testing.T) {
	asset := scanAsset("Setup.exe")
	h, router, c := scanRouter(t, []github.Asset{asset}, nil)
	store, err := artifactscan.NewLocal(t.TempDir(), "inspector-1")
	if err != nil {
		t.Fatal(err)
	}
	h.ScanStore = store
	completeScan(t, store, asset, scanMetadata("arm64"), nil)
	stub := scanMetadata("386")
	stub.Evidence = []github.ArtifactEvidence{{Source: "inspection", Field: "stub.cpu", Value: "386"}}
	shallowScan(t, c, asset, stub)
	page := scanRequest(router, "/api/release/acme/tool")
	var payload struct {
		Assets []github.Asset         `json:"assets"`
		Picks  map[string]releasePick `json:"picks"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &payload); err != nil || page.Code != http.StatusOK {
		t.Fatalf("page: %d %s err=%v", page.Code, page.Body, err)
	}
	if len(payload.Assets) != 1 || payload.Assets[0].Artifact == nil {
		t.Fatalf("page omitted deep artifact metadata: %s", page.Body)
	}
	observed := payload.Assets[0].Artifact
	if len(observed.Conflicts) != 0 || !reflectTargets(observed.Targets, "windows", "arm64") ||
		slices.ContainsFunc(observed.Evidence, func(e github.ArtifactEvidence) bool { return e.Field == "stub.cpu" }) {
		t.Fatalf("shallow launcher overrode payload proof: %+v", observed)
	}
	if pick := payload.Picks["windows/arm64"]; pick.Status != "selected" || pick.URL != asset.BrowserDownloadURL {
		t.Fatalf("page did not use payload CPU: %+v", pick)
	}
	for _, path := range []string{"/api/link/acme/tool", "/dl/acme/tool"} {
		good := scanRequest(router, path+"?platform=windows&arch=arm64")
		if path == "/dl/acme/tool" {
			if good.Code != http.StatusFound || good.Header().Get("Location") != asset.BrowserDownloadURL {
				t.Fatalf("redirect: %d %s", good.Code, good.Body)
			}
		} else {
			var link LinkResponse
			if err := json.Unmarshal(good.Body.Bytes(), &link); err != nil || good.Code != http.StatusOK || link.URL != asset.BrowserDownloadURL {
				t.Fatalf("link: %d %s", good.Code, good.Body)
			}
		}
		bad := scanRequest(router, path+"?platform=windows&arch=amd64")
		if bad.Code != http.StatusNotFound {
			t.Fatalf("wrong CPU accepted by %s: %d %s", path, bad.Code, bad.Body)
		}
	}
}

func reflectTargets(targets []github.ArtifactTarget, platform, arch string) bool {
	return len(targets) == 1 && targets[0].Platform == platform && targets[0].Arch == arch
}

func TestDeepUnknownRetainsFastFactsAndDoesNotRequeue(t *testing.T) {
	asset := scanAsset("Setup.exe")
	h, router, c := scanRouter(t, []github.Asset{asset}, nil)
	store, err := artifactscan.NewLocal(t.TempDir(), "inspector-1")
	if err != nil {
		t.Fatal(err)
	}
	h.ScanStore = store
	completeScan(t, store, asset, github.ArtifactMetadata{Status: "unsupported", Role: "unknown", Evidence: []github.ArtifactEvidence{
		{Source: "deep-inspection", Field: "inspection.skipped", Value: "Inno payload version 7.1 cannot be extracted"},
	}}, nil)
	fast := scanMetadata("amd64")
	fast.Evidence = []github.ArtifactEvidence{{Source: "inspection", Field: "PE.machine", Value: "amd64"}}
	shallowScan(t, c, asset, fast)
	page := scanRequest(router, "/api/release/acme/tool")
	var payload struct {
		Assets []github.Asset `json:"assets"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &payload); err != nil || page.Code != http.StatusOK || len(payload.Assets) != 1 {
		t.Fatalf("page: %d %s err=%v", page.Code, page.Body, err)
	}
	metadata := payload.Assets[0].Artifact
	if metadata == nil || metadata.Status != "inspected" || !reflectTargets(metadata.Targets, "windows", "amd64") ||
		!slices.ContainsFunc(metadata.Evidence, func(e github.ArtifactEvidence) bool { return strings.Contains(e.Value, "Inno payload version 7.1") }) {
		t.Fatalf("terminal unknown replaced fast facts or lost reason: %+v", metadata)
	}
	for _, path := range []string{"/api/link/acme/tool", "/dl/acme/tool"} {
		response := scanRequest(router, path+"?platform=windows&arch=amd64")
		if response.Code != http.StatusOK && response.Code != http.StatusFound {
			t.Fatalf("fast fallback hidden by deep unknown: %d %s", response.Code, response.Body)
		}
	}
	if worked, err := store.ProcessNext(context.Background(), completedInspectorForHandler); worked || err != nil {
		t.Fatalf("terminal unknown was requeued: worked=%v err=%v", worked, err)
	}
}

func completedInspectorForHandler(context.Context, github.Asset) (github.ArtifactMetadata, error) {
	return scanMetadata("amd64"), nil
}

func TestDeepMalformedBlocksFileAndReplacementQueuesFreshInspection(t *testing.T) {
	original := scanAsset("Tool-windows-arm64.exe")
	store, err := artifactscan.NewLocal(t.TempDir(), "inspector-1")
	if err != nil {
		t.Fatal(err)
	}
	completeScan(t, store, original, github.ArtifactMetadata{}, fmt.Errorf("%w: invalid payload header", github.ErrMalformedArtifact))
	t.Run("malformed", func(t *testing.T) {
		h, router, _ := scanRouter(t, []github.Asset{original}, nil)
		h.ScanStore = store
		for _, path := range []string{"/api/link/acme/tool", "/dl/acme/tool"} {
			response := scanRequest(router, path+"?platform=windows&arch=arm64")
			if response.Code != http.StatusNotFound {
				t.Fatalf("malformed payload selected: %d %s", response.Code, response.Body)
			}
		}
	})
	t.Run("replacement", func(t *testing.T) {
		replaced := original
		replaced.ID++
		replaced.UpdatedAt = "2026-10-18T12:00:00Z"
		h, router, _ := scanRouter(t, []github.Asset{replaced}, nil)
		h.ScanStore = store
		response := scanRequest(router, "/dl/acme/tool?platform=windows&arch=arm64")
		if response.Code != http.StatusFound || response.Header().Get("Location") != replaced.BrowserDownloadURL {
			t.Fatalf("replacement reused failed proof: %d %s", response.Code, response.Body)
		}
		record, found, err := store.Lookup(context.Background(), replaced)
		if err != nil || !found || record.State != artifactscan.StateQueued || record.Attempts != 0 {
			t.Fatalf("named replacement not queued: %+v found=%v err=%v", record, found, err)
		}
	})
}

func TestDeepPublisherConflictBlocksAutomaticDownload(t *testing.T) {
	asset := scanAsset("Setup.exe")
	manifest := `{"dist_version":"0.30.0","artifacts":{"app":{"name":"Setup.exe","kind":"executable-zip","target_triples":["x86_64-pc-windows-msvc"]}}}`
	manifestAsset := scanAsset("dist-manifest.json")
	manifestAsset.ID++
	manifestAsset.Size = int64(len(manifest))
	h, router, _ := scanRouter(t, []github.Asset{asset, manifestAsset}, map[string]string{manifestAsset.BrowserDownloadURL: manifest})
	store, err := artifactscan.NewLocal(t.TempDir(), "inspector-1")
	if err != nil {
		t.Fatal(err)
	}
	h.ScanStore = store
	completeScan(t, store, asset, scanMetadata("arm64"), nil)
	response := scanRequest(router, "/dl/acme/tool?platform=windows&arch=arm64")
	if response.Code != http.StatusNotFound {
		t.Fatalf("publisher/content contradiction was discarded: %d %s", response.Code, response.Body)
	}
}

func TestDeepNonRunnableRoleBlocksFilenameValidFile(t *testing.T) {
	asset := scanAsset("Tool-windows-amd64.exe")
	h, router, _ := scanRouter(t, []github.Asset{asset}, nil)
	store, err := artifactscan.NewLocal(t.TempDir(), "inspector-1")
	if err != nil {
		t.Fatal(err)
	}
	h.ScanStore = store
	completeScan(t, store, asset, github.ArtifactMetadata{Status: "inspected", Role: "source",
		Evidence: []github.ArtifactEvidence{{Source: "deep-inspection", Field: "payload.role", Value: "source files only"}},
	}, nil)
	response := scanRequest(router, "/dl/acme/tool?platform=windows&arch=amd64")
	if response.Code != http.StatusNotFound {
		t.Fatalf("deep source role was discarded: %d %s", response.Code, response.Body)
	}
}

func TestNamedInstallersAndUnprovenArchivesQueueButSidecarsDoNot(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{"Tool-windows-x64.exe", true}, {"Tool-macos-arm64.dmg", true}, {"Tool-windows-x64.msi", true},
		{"Tool-macos-arm64.pkg", true}, {"Tool-windows-x64.msix", true}, {"Tool-windows-x64.msixbundle", true},
		{"Tool-linux-amd64.flatpak", true}, {"Tool-linux-amd64.tar.xz", true}, {"opaque.zip", true},
		{"tool-source.zip", false}, {"Tool-windows-x64.exe.sha256", false}, {"Tool-windows-x64-symbols.zip", false},
		{"Tool-wasm32.zip", false}, {"firmware-android-arm64.zip", false}, {"lineage-23.0.zip", false},
		{"Tool-freebsd-arm64.tar.xz", false}, {"Tool-android-arm64.apk", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := needsDeepInspection(scanAsset(test.name)); got != test.want {
				t.Fatalf("needsDeepInspection=%v, want %v", got, test.want)
			}
		})
	}
	asset := scanAsset("Tool-linux-amd64.tar.xz")
	asset.Artifact = &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64"}},
		Evidence: []github.ArtifactEvidence{{Source: "filename", Field: "token", Value: "amd64"}}}
	if !needsDeepInspection(asset) {
		t.Fatal("filename-filled targets were treated as completed inspection")
	}
	asset.Artifact.Evidence[0].Source = "cargo-dist"
	if needsDeepInspection(asset) {
		t.Fatal("archive with publisher-proven OS and CPU was queued again")
	}
	asset.Name = "Tool-linux-amd64.exe"
	if !needsDeepInspection(asset) {
		t.Fatal("named installer was skipped because its filename or manifest had a CPU")
	}
}

func TestScanLookupAndQueueFailuresKeepFastResponses(t *testing.T) {
	t.Run("queue failure", func(t *testing.T) {
		asset := scanAsset("Tool-windows-amd64.exe")
		h, router, _ := scanRouter(t, []github.Asset{asset}, nil)
		dir := t.TempDir()
		store, err := artifactscan.NewLocal(dir, "inspector-1")
		if err != nil {
			t.Fatal(err)
		}
		h.ScanStore = store
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		response := scanRequest(router, "/dl/acme/tool?platform=windows&arch=amd64")
		if response.Code != http.StatusFound || response.Header().Get("Location") != asset.BrowserDownloadURL {
			t.Fatalf("queue failure changed fast response: %d %s", response.Code, response.Body)
		}
	})
	t.Run("lookup timeout", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		connections := make(chan net.Conn, 8)
		go func() {
			defer close(connections)
			for {
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				connections <- connection
			}
		}()
		t.Cleanup(func() {
			_ = listener.Close()
			for connection := range connections {
				_ = connection.Close()
			}
		})
		client := redis.NewClient(&redis.Options{Addr: listener.Addr().String(), DialTimeout: time.Second, ReadTimeout: time.Second,
			WriteTimeout: time.Second, MaxRetries: -1, ContextTimeoutEnabled: true})
		t.Cleanup(func() { _ = client.Close() })
		store, err := artifactscan.NewRedis(client, "inspector-1")
		if err != nil {
			t.Fatal(err)
		}
		asset := scanAsset("Tool-windows-amd64.exe")
		h, router, _ := scanRouter(t, []github.Asset{asset}, nil)
		h.ScanStore = store
		started := time.Now()
		response := scanRequest(router, "/dl/acme/tool?platform=windows&arch=amd64")
		if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
			t.Errorf("scan lookup exceeded its request budget: %s", elapsed)
		}
		if response.Code != http.StatusFound || response.Header().Get("Location") != asset.BrowserDownloadURL {
			t.Fatalf("lookup timeout changed fast response: %d %s", response.Code, response.Body)
		}
	})
}

func TestShallowTransportFailureKeepsFilenameValidDownload(t *testing.T) {
	for _, key := range []string{"REDIS_URL", "KV_URL", "UPSTASH_REDIS_URL", "GITHUB_TOKEN"} {
		t.Setenv(key, "")
	}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	h := NewRedirectHandler(github.NewClient(), cache.New())
	assets := []github.Asset{scanAsset("Tool-android-arm64.apk")}
	h.enrichArtifacts(context.Background(), assets)
	if assets[0].Artifact.Status == "failed" {
		t.Fatalf("transient transport failure became malformed-file proof: %+v", assets[0].Artifact)
	}
	if pick := buildReleasePicks(assets)["android/arm64"]; pick.Filename != assets[0].Name || pick.Status != "selected" {
		t.Fatalf("transport failure hid valid fast fallback: %+v", pick)
	}
}
