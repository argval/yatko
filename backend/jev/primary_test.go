package jev

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

var macAssets = assetsNamed("app-arm64.dmg", "app-x64.dmg")

// nativeMac picks the dmg named for the visitor's CPU in the request.
func nativeMac(t *testing.T, r wireRequest) fakeReply {
	if strings.Contains(string(r.State["visitor"]), "64-bit ARM") {
		return fakeReply{choice: "app-arm64.dmg", p: 0.95}
	}
	return fakeReply{choice: "app-x64.dmg", p: 0.95}
}

// The decision cache answers repeat contexts, including reordered assets,
// without a new Choice (the Describe cache alone cannot skip a Choice).
func TestPrimaryCachesDecisionPerContext(t *testing.T) {
	c, fs := fakeJev(t, picks("tool-linux-amd64.tar.gz", 0.95))
	p := &Primary{Client: c, Cache: cache.New()}
	for i := 0; i < 3; i++ {
		d, ok := p.Decide(context.Background(), "o/tool", linuxPair, picker.Linux, picker.AMD64, picker.PickOpts{})
		if !ok || name(d) != "tool-linux-amd64.tar.gz" || !d.ShouldAutoSelect() {
			t.Fatalf("run %d: ok=%v decision=%+v", i, ok, d)
		}
	}
	reversed := []github.Asset{linuxPair[1], linuxPair[0]}
	if _, ok := p.Decide(context.Background(), "o/tool", reversed, picker.Linux, picker.AMD64, picker.PickOpts{}); !ok {
		t.Fatal("reordered assets: not ok")
	}
	if fs.choices.Load() != 1 || fs.describes.Load() != 1 {
		t.Fatalf("choices=%d describes=%d, want 1 each (later requests are cache hits)", fs.choices.Load(), fs.describes.Load())
	}
	if d, ok := p.Decide(context.Background(), "o/tool", linuxPair, picker.Linux, picker.AMD64, picker.PickOpts{Prefer: "appimage"}); !ok || name(d) != "tool-linux-amd64.AppImage" {
		t.Fatalf("another context: ok=%v %+v", ok, d)
	}
}

// An abstention is a successful decision: the caller must not fall back to rules.
func TestPrimaryAbstentionIsOK(t *testing.T) {
	c, _ := fakeJev(t, picks(InsufficientOption, 0.9))
	p := &Primary{Client: c, Cache: cache.New()}
	d, ok := p.Decide(context.Background(), "o/app", macAssets, picker.MacOS, picker.UnknownArch, picker.PickOpts{})
	if !ok || d.ShouldAutoSelect() || d.Status != "unknown" {
		t.Fatalf("ok=%v %+v", ok, d)
	}
}

// A slow call falls back at once but still fills the cache for the next request.
func TestPrimarySlowCallFallsBackThenWarms(t *testing.T) {
	c, fs := fakeJev(t, func(t *testing.T, r wireRequest) fakeReply {
		rep := nativeMac(t, r)
		rep.latency = 300 * time.Millisecond
		return rep
	})
	p := &Primary{Client: c, Cache: cache.New(), Wait: 50 * time.Millisecond}
	if _, ok := p.Decide(context.Background(), "o/slow", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok {
		t.Fatal("cold slow call should fall back to the rules")
	}
	time.Sleep(900 * time.Millisecond)
	d, ok := p.Decide(context.Background(), "o/slow", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{})
	if !ok || name(d) != "app-arm64.dmg" || fs.calls() != 1 {
		t.Fatalf("warmed: ok=%v calls=%d %+v", ok, fs.calls(), d)
	}
}

func TestPrimaryErrorsFallBackAndTripBreaker(t *testing.T) {
	c, fs := fakeJev(t, func(*testing.T, wireRequest) fakeReply { return fakeReply{status: http.StatusInternalServerError} })
	p := &Primary{Client: c, Cache: cache.New()}
	for i := 0; i < breakerFails; i++ {
		if _, ok := p.Decide(context.Background(), "o/err"+string(rune('a'+i)), macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok {
			t.Fatal("API error must fall back to the rules")
		}
	}
	before := fs.calls()
	if _, ok := p.Decide(context.Background(), "o/err-after", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok || fs.calls() != before {
		t.Fatalf("open breaker still called the API: ok=%v calls %d -> %d", ok, before, fs.calls())
	}
}

func TestPrimaryMalformedFallsBack(t *testing.T) {
	c, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
		return fakeReply{tamper: func(a map[string]any) { delete(a, "os_0") }}
	})
	p := &Primary{Client: c, Cache: cache.New()}
	if _, ok := p.Decide(context.Background(), "o/bad", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok {
		t.Fatal("malformed answer must fall back to the rules")
	}
}

// A full in-flight pool falls back without calling the API.
func TestPrimaryInFlightBound(t *testing.T) {
	c, fs := fakeJev(t, nativeMac)
	p := &Primary{Client: c, Cache: cache.New()}
	p.inFlight.Store(maxInFlight)
	if _, ok := p.Decide(context.Background(), "o/busy", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok || fs.calls() != 0 {
		t.Fatalf("ok=%v calls=%d", ok, fs.calls())
	}
}

// A cached record never serves an asset that is gone or fails the current guard.
func TestRecordRevalidatesAsset(t *testing.T) {
	r := record{Status: "selected", Name: "app-arm64.dmg", Confidence: picker.ConfidenceHigh}
	if _, ok := r.decision(macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); !ok {
		t.Fatal("valid record rejected")
	}
	if _, ok := (record{Status: "selected", Name: "gone.dmg"}).decision(macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok {
		t.Fatal("decision accepted an asset that is not in the release")
	}
	failed := []github.Asset{{Name: "app-arm64.dmg", Artifact: &github.ArtifactMetadata{Status: "failed"}}}
	if _, ok := r.decision(failed, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok {
		t.Fatal("decision accepted an asset that now fails the guard")
	}
}

// Warm pays for Describe ahead of the first pick, which then needs at most its
// own Choice; a second client sharing the cache (another instance) pays nothing.
func TestWarmLetsFirstPickSkipDescribe(t *testing.T) {
	c, fs := fakeJev(t, nativeMac)
	shared := cache.New()
	c.Cache = shared
	p := &Primary{Client: c, Cache: shared}
	p.Warm(context.Background(), "o/app", "An app", macAssets)
	if fs.describes.Load() != 1 || fs.choices.Load() != 0 {
		t.Fatalf("warm: describes=%d choices=%d, want 1 and 0", fs.describes.Load(), fs.choices.Load())
	}
	d, ok := p.Decide(context.Background(), "o/app", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{ProductDescription: "An app"})
	if !ok || name(d) != "app-arm64.dmg" {
		t.Fatalf("pick after warm: ok=%v %+v", ok, d)
	}
	if fs.describes.Load() != 1 {
		t.Fatalf("pick after warm: describes=%d, want the warmed answer reused", fs.describes.Load())
	}
	other := &Client{apiKey: c.apiKey, endpoint: c.endpoint, model: c.model, http: c.http, Cache: shared}
	if _, _, err := other.Describe(context.Background(), "o/app", "An app", describableAssets(macAssets)); err != nil || fs.describes.Load() != 1 {
		t.Fatalf("second instance: err=%v describes=%d, want the shared answer", err, fs.describes.Load())
	}
}
