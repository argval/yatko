package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// fakeAPI answers Describe for any file with a single OS/CPU read from its
// name ("-arm64" or "-x64", macOS) and counts requests.
func fakeAPI(t *testing.T, delay time.Duration, status int) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(delay)
		if status != http.StatusOK {
			http.Error(w, "boom", status)
			return
		}
		var req struct {
			State     struct{ Files []string } `json:"state"`
			Questions map[string]struct {
				Instructions map[string]string `json:"instructions"`
			} `json:"questions"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		answers := map[string]any{}
		for id, q := range req.Questions {
			file := q.Instructions["file"]
			arm := len(file) >= 5 && file[len(file)-9:len(file)-4] == "arm64"
			switch id[:2] {
			case "os":
				answers[id] = map[string]any{"type": "choice", "choice": osMac, "probabilities": map[string]float64{osMac: 1}}
			case "cp":
				cpu := cpuAMD64
				if arm {
					cpu = cpuARM64
				}
				answers[id] = map[string]any{"type": "choice", "choice": cpu, "probabilities": map[string]float64{cpu: 1}}
			default:
				answers[id] = map[string]any{"type": "choice", "choice": roleMain, "probabilities": map[string]float64{roleMain: 1}}
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "jev-test", "answers": answers, "usage": map[string]int{"input_tokens": 10}})
	}))
	t.Cleanup(srv.Close)
	return &Client{apiKey: "k", endpoint: srv.URL, model: "jev-test", http: srv.Client()}, &calls
}

var macAssets = []github.Asset{{Name: "app-arm64.dmg"}, {Name: "app-x64.dmg"}}

func TestPrimaryCachesDecisionAcrossVisitors(t *testing.T) {
	c, calls := fakeAPI(t, 0, http.StatusOK)
	p := &Primary{Client: c, Cache: cache.New()}
	for i := 0; i < 3; i++ {
		d, ok := p.Decide(context.Background(), "o/app", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{})
		if !ok || d.Asset == nil || d.Asset.Name != "app-arm64.dmg" || !d.ShouldAutoSelect() {
			t.Fatalf("run %d: ok=%v decision=%+v", i, ok, d)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("API calls = %d, want 1 (later requests are cache hits)", n)
	}
	// Another visitor type shares the per-release file reading but is its own decision.
	if d, ok := p.Decide(context.Background(), "o/app", macAssets, picker.MacOS, picker.AMD64, picker.PickOpts{}); !ok || d.Asset.Name != "app-x64.dmg" {
		t.Fatalf("amd64: ok=%v %+v", ok, d)
	}
}

// A slow call falls back at once but still fills the cache for the next request.
func TestPrimarySlowCallFallsBackThenWarms(t *testing.T) {
	c, calls := fakeAPI(t, 300*time.Millisecond, http.StatusOK)
	p := &Primary{Client: c, Cache: cache.New(), Wait: 50 * time.Millisecond}
	if _, ok := p.Decide(context.Background(), "o/slow", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok {
		t.Fatal("cold slow call should fall back to the rules")
	}
	time.Sleep(900 * time.Millisecond)
	d, ok := p.Decide(context.Background(), "o/slow", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{})
	if !ok || d.Asset.Name != "app-arm64.dmg" || calls.Load() != 1 {
		t.Fatalf("warmed: ok=%v calls=%d %+v", ok, calls.Load(), d)
	}
}

func TestPrimaryErrorsFallBackAndTripBreaker(t *testing.T) {
	c, calls := fakeAPI(t, 0, http.StatusInternalServerError)
	p := &Primary{Client: c, Cache: cache.New()}
	for i := 0; i < breakerFails; i++ {
		if _, ok := p.Decide(context.Background(), "o/err"+string(rune('a'+i)), macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok {
			t.Fatal("API error must fall back to the rules")
		}
	}
	before := calls.Load()
	if _, ok := p.Decide(context.Background(), "o/err-after", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok || calls.Load() != before {
		t.Fatalf("open breaker still called the API: ok=%v calls %d -> %d", ok, before, calls.Load())
	}
}

// A cached record never invents an asset: a name missing from the release means "use the rules".
func TestRecordRejectsMissingAsset(t *testing.T) {
	r := record{Status: "selected", Name: "gone.dmg", Confidence: picker.ConfidenceHigh}
	if _, ok := r.decision(macAssets); ok {
		t.Fatal("decision accepted an asset that is not in the release")
	}
}
