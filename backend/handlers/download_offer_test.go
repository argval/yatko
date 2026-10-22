package handlers

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

// A prior offer never authorizes another file, visitor or incompatible target.
func TestDownloadOfferRevalidatesCurrentAsset(t *testing.T) {
	for _, key := range []string{"REDIS_URL", "KV_URL", "UPSTASH_REDIS_URL"} {
		t.Setenv(key, "")
	}
	h := &RedirectHandler{cache: cache.New(), Primary: &jev.Primary{}}
	asset := github.Asset{ID: 1, Name: "tool-windows-amd64.zip", State: "uploaded", Size: 10,
		BrowserDownloadURL: "https://example.com/v1/tool-windows-amd64.zip", Digest: "sha256:original"}
	release := &github.Release{TagName: "v1", Assets: []github.Asset{asset}}
	contextFor := func(query, ua string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		c.Request.Header.Set("User-Agent", ua)
		return c
	}
	c := contextFor("platform=windows&arch=amd64", windowsUA)
	p := resolveAssetPick(c)
	p.Decision = picker.DecideAsset(release.Assets, p.Platform, p.Arch, p.opts())
	if !p.Decision.ShouldAutoSelect() {
		t.Fatal("precondition: rules must offer the file")
	}
	h.rememberDownload(c, "acme", "tool", release, p)
	for _, tc := range []struct {
		name   string
		change func(*github.Asset)
		want   bool
	}{
		{"same file", func(*github.Asset) {}, true},
		{"new compatible inspection", func(a *github.Asset) {
			a.Artifact = &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "amd64"}}}
		}, true},
		{"wrong CPU", func(a *github.Asset) {
			a.Artifact = &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "arm64"}}}
		}, false},
		{"wrong OS", func(a *github.Asset) {
			a.Artifact = &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64"}}}
		}, false},
		{"failed inspection", func(a *github.Asset) { a.Artifact = &github.ArtifactMetadata{Status: "failed"} }, false},
		{"source role", func(a *github.Asset) { a.Artifact = &github.ArtifactMetadata{Status: "inspected", Role: "source"} }, false},
		{"not uploaded", func(a *github.Asset) { a.State = "starter" }, false},
		{"replacement asset", func(a *github.Asset) { a.ID++ }, false},
		{"changed contents", func(a *github.Asset) { a.Digest = "sha256:replacement" }, false},
		{"changed URL", func(a *github.Asset) { a.BrowserDownloadURL += "-other" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := asset
			tc.change(&current)
			r := &github.Release{TagName: "v1", Assets: []github.Asset{current}}
			c := contextFor("platform=windows&arch=amd64&asset="+asset.Name, windowsUA)
			d, ok := h.offeredDownload(c, "acme", "tool", r, resolveAssetPick(c))
			if ok != tc.want || (ok && (!d.ShouldAutoSelect() || d.Asset.BrowserDownloadURL != current.BrowserDownloadURL)) {
				t.Fatalf("accepted=%v want=%v decision=%+v", ok, tc.want, d)
			}
		})
	}
	for _, query := range []string{
		"platform=windows&arch=arm64", "platform=linux&arch=amd64", "platform=windows&arch=amd64&prefer=zip",
	} {
		c := contextFor(query+"&asset="+asset.Name, windowsUA)
		if _, ok := h.offeredDownload(c, "acme", "tool", release, resolveAssetPick(c)); ok {
			t.Errorf("offer crossed visitor context: %s", query)
		}
	}
	c = contextFor("platform=windows&arch=amd64&asset="+asset.Name, "curl/8.5.0")
	if _, ok := h.offeredDownload(c, "acme", "tool", release, resolveAssetPick(c)); ok {
		t.Error("offer crossed user agents")
	}
	c = contextFor("platform=windows&arch=amd64&asset="+asset.Name, windowsUA)
	if _, ok := h.offeredDownload(c, "acme", "other", release, resolveAssetPick(c)); ok {
		t.Error("offer crossed repositories")
	}
	for _, r := range []*github.Release{{TagName: "v2", Assets: []github.Asset{asset}}, {TagName: "v1"}} {
		if _, ok := h.offeredDownload(c, "acme", "tool", r, resolveAssetPick(c)); ok {
			t.Error("offer crossed releases or accepted a missing asset")
		}
	}
	key := downloadOfferKey("acme", "tool", release, asset, p)
	if err := h.cache.Invalidate(c.Request.Context(), key); err != nil {
		t.Fatal(err)
	}
	_, err := cache.FetchCached(c.Request.Context(), h.cache, key, func(context.Context, string) (downloadOffer, string, bool, error) {
		return downloadOffer{Confidence: picker.ConfidenceHigh, ExpiresAt: time.Now().Add(-time.Second)}, "", false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.offeredDownload(c, "acme", "tool", release, resolveAssetPick(c)); ok {
		t.Error("expired offer was accepted from the process cache")
	}
	h.rememberDownload(contextFor("platform=windows&arch=amd64", windowsUA), "acme", "tool", release, p)
	if _, ok := h.offeredDownload(c, "acme", "tool", release, resolveAssetPick(c)); !ok {
		t.Error("a fresh successful selection did not renew the expired offer")
	}
}
