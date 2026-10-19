package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

func TestVisitorContextPreservesSourcesAndHints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?platform=macos&arch=arm64&platform_source=browser&arch_source=client-hints&hint_architecture=arm&hint_bitness=64", nil)
	c.Request.Header.Set("User-Agent", windowsUA)
	c.Request.Header.Set("Sec-CH-UA-Arch", `"x86"`)
	c.Request.Header.Set("Sec-CH-UA-Platform-Version", `"14.0"`)
	c.Request.Header.Set("Sec-CH-UA-Full-Version", "unrelated")
	p := resolveAssetPick(c)
	opts := p.opts()
	if p.Platform != picker.MacOS || p.Arch != picker.ARM64 || opts.PlatformSource != "browser" || opts.ArchSource != "client-hints" {
		t.Fatalf("visitor context = %+v", p)
	}
	if opts.UserAgent != windowsUA || opts.ClientHints["architecture"] != "arm" || opts.ClientHints["bitness"] != "64" || opts.ClientHints["platform_version"] != `"14.0"` || len(opts.ClientHints) != 3 {
		t.Fatalf("options = %+v", opts)
	}
}

func TestVisitorContextManualOverridesStayExplicit(t *testing.T) {
	for _, source := range []string{"manual", "", "unexpected"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/?platform=linux&arch=unknown&platform_source="+source+"&arch_source="+source, nil)
		c.Request.Header.Set("User-Agent", windowsUA)
		c.Request.Header.Set("Sec-CH-UA-Arch", `"x86"`)
		p := resolveAssetPick(c)
		if p.Platform != picker.Linux || p.Arch != picker.UnknownArch || p.PlatformSource != picker.SourceQuery || p.ArchSource != picker.SourceQuery {
			t.Fatalf("source %q: %+v", source, p)
		}
	}
}

func TestVisitorContextRejectsUnboundedHints(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?hint_architecture=arm%0Aignore&hint_mobile=%3F1", nil)
	c.Request.Header.Set("Sec-CH-UA-Model", strings.Repeat("x", 129))
	hints := visitorClientHints(c)
	if len(hints) != 1 || hints["mobile"] != "?1" {
		t.Fatalf("hints = %+v", hints)
	}
}

func TestVisitorOptionsOwnClientHints(t *testing.T) {
	p := assetPick{ClientHints: map[string]string{"architecture": "arm"}}
	opts := p.opts()
	p.ClientHints["architecture"] = "x86"
	if opts.ClientHints["architecture"] != "arm" {
		t.Fatal("options shared the request hint map")
	}
}
