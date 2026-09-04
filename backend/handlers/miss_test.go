package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit/605.1.15 Safari/605.1.15", false},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0", false},
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
			Platform: picker.Linux,
			Arch:     picker.AMD64,
			Prefer:   "deb",
			Libc:     picker.LibcMusl,
			Decision: picker.AssetDecision{Confidence: picker.ConfidenceLow, Reasons: []string{"no matching installable asset"}},
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
