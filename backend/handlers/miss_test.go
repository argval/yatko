package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

func TestIsScriptUA(t *testing.T) {
	cases := []struct {
		ua   string
		want bool
	}{
		{"", true},
		{"curl/8.0.1", true},
		{"Wget/1.21", true},
		{"HTTPie/3.0", true},
		{"Go-http-client/2.0", true},
		{"python-requests/2.28.0", true},
		{"axios/1.0.0", true},
		{"Deno/2.0.0", true},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit/605.1.15 Safari/605.1.15", false},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0", false},
		{"Mozilla/5.0 (Windows NT; Windows NT 10.0; en-US) WindowsPowerShell/5.1", false},
	}
	for _, tc := range cases {
		if got := IsScriptUA(tc.ua); got != tc.want {
			t.Errorf("IsScriptUA(%q) = %v, want %v", tc.ua, got, tc.want)
		}
	}
}

func TestRespondDownloadMiss(t *testing.T) {
	gin.SetMode(gin.TestMode)
	release := &github.Release{
		HTMLURL: "https://github.com/cli/cli/releases/tag/v1.0.0",
		Assets:  []github.Asset{{Name: "only-windows.exe"}},
	}

	t.Run("script gets 404 json", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/dl/cli/cli", nil)
		c.Request.Header.Set("User-Agent", "curl/8.5.0")
		respondDownloadMiss(c, "cli", "cli", release, assetPick{
			Platform:       picker.Linux,
			Arch:           picker.AMD64,
			Prefer:         "deb",
			Libc:           picker.LibcMusl,
			PlatformSource: picker.SourceDefault,
			ArchSource:     picker.SourceDefault,
			Decision:       picker.AssetDecision{Confidence: picker.ConfidenceLow, Reasons: []string{"no matching installable asset"}},
		})
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("json: %v", err)
		}
		if body["url"] != release.HTMLURL {
			t.Fatalf("url = %v, want %s", body["url"], release.HTMLURL)
		}
		if body["platform_source"] != "default" || body["arch_source"] != "default" {
			t.Fatalf("sources = %v/%v, want default/default", body["platform_source"], body["arch_source"])
		}
	})

	t.Run("browser redirects to github html", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/dl/cli/cli", nil)
		c.Request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X) Safari/605.1.15")
		respondDownloadMiss(c, "cli", "cli", release, assetPick{
			Platform: picker.MacOS,
			Arch:     picker.ARM64,
			Decision: picker.AssetDecision{Confidence: picker.ConfidenceLow},
		})
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", w.Code)
		}
		if loc := w.Header().Get("Location"); loc != release.HTMLURL {
			t.Fatalf("Location = %q, want %q", loc, release.HTMLURL)
		}
	})
}

func TestPickReleaseAsset_ScriptDefaultsToLinuxAmd64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assets := []github.Asset{
		{Name: "tool-windows-amd64.zip", BrowserDownloadURL: "https://example.com/win.zip", Size: 11},
		{Name: "tool-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux.tgz", Size: 10},
		{Name: "tool-darwin-arm64.dmg", BrowserDownloadURL: "https://example.com/mac.dmg", Size: 12},
	}

	t.Run("curl without OS picks linux amd64", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/dl/acme/tool", nil)
		c.Request.Header.Set("User-Agent", "curl/8.5.0")
		p := pickReleaseAsset(c, assets)
		if p.Platform != picker.Linux || p.Arch != picker.AMD64 {
			t.Fatalf("got %s/%s, want linux/amd64", p.Platform, p.Arch)
		}
		if p.PlatformSource != picker.SourceDefault || p.ArchSource != picker.SourceDefault {
			t.Fatalf("sources = %s/%s, want default/default", p.PlatformSource, p.ArchSource)
		}
		if !p.Decision.ShouldAutoSelect() || p.Decision.Asset.Name != "tool-linux-amd64.tar.gz" {
			t.Fatalf("asset = %+v", p.Decision.Asset)
		}
	})

	t.Run("query overrides still win", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/dl/acme/tool?platform=macos&arch=arm64", nil)
		c.Request.Header.Set("User-Agent", "curl/8.5.0")
		p := pickReleaseAsset(c, assets)
		if p.Platform != picker.MacOS || p.Arch != picker.ARM64 {
			t.Fatalf("got %s/%s, want macos/arm64", p.Platform, p.Arch)
		}
		if p.PlatformSource != picker.SourceQuery || p.ArchSource != picker.SourceQuery {
			t.Fatalf("sources = %s/%s, want query/query", p.PlatformSource, p.ArchSource)
		}
		if !p.Decision.ShouldAutoSelect() || p.Decision.Asset.Name != "tool-darwin-arm64.dmg" {
			t.Fatalf("asset = %+v", p.Decision.Asset)
		}
	})

	t.Run("browser mac UA is unchanged", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/dl/acme/tool", nil)
		c.Request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15")
		p := pickReleaseAsset(c, assets)
		if p.Platform != picker.MacOS {
			t.Fatalf("platform = %s, want macos", p.Platform)
		}
		if p.PlatformSource != picker.SourceUA {
			t.Fatalf("platform source = %s, want ua", p.PlatformSource)
		}
	})
}

func TestPinnedDownloadKeepsDisplayedAssetAndRejectsIncompatible(t *testing.T) {
	assets := []github.Asset{{Name: "app-linux-amd64.tar.gz"}, {Name: "app-linux-amd64.AppImage"}, {Name: "app-linux-arm64.tar.gz"}, {Name: "app-linux-amd64-debuginfo.zip"}}
	for _, tc := range []struct {
		name     string
		selected bool
	}{
		{"app-linux-amd64.tar.gz", true}, {"app-linux-arm64.tar.gz", false},
		{"app-linux-amd64-debuginfo.zip", false}, {"https://attacker.example/app", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/dl/o/r?platform=linux&arch=amd64&asset="+url.QueryEscape(tc.name), nil)
			d := pickReleaseAsset(c, assets).Decision
			if d.ShouldAutoSelect() != tc.selected {
				t.Fatalf("%+v", d)
			}
			if tc.selected && d.Asset.Name != tc.name {
				t.Fatalf("switched from %s to %s", tc.name, d.Asset.Name)
			}
		})
	}
}
