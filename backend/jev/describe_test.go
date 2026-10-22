package jev

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/picker"
)

var describeAssets = assetsNamed("tool-linux-amd64.tar.gz")

// corruptions break one question's answer in a way Describe must reject.
// Each gets the answer map; chosen is the offered choice, other any other option.
var corruptions = map[string]func(a map[string]any, probs map[string]float64, chosen, other string){
	"wrong answer type":      func(a map[string]any, _ map[string]float64, _, _ string) { a["type"] = "noul" },
	"missing probabilities":  func(a map[string]any, _ map[string]float64, _, _ string) { delete(a, "probabilities") },
	"unknown option key":     func(_ map[string]any, p map[string]float64, _, _ string) { p["not an option"] = 0 },
	"missing option key":     func(_ map[string]any, p map[string]float64, _, o string) { delete(p, o) },
	"negative probability":   func(_ map[string]any, p map[string]float64, c, o string) { p[c], p[o] = 1.5, -0.5 },
	"probability above one":  func(_ map[string]any, p map[string]float64, _, o string) { p[o] = 1.2 },
	"sum is not one":         func(_ map[string]any, p map[string]float64, _, o string) { p[o] = 0.5 },
	"choice not offered":     func(a map[string]any, _ map[string]float64, _, _ string) { a["choice"] = "Plan 9" },
	"choice is not the mode": func(_ map[string]any, p map[string]float64, c, o string) { p[c], p[o] = 0.2, 0.8 },
}

// tamperFirst corrupts question id on the first Describe request only.
func tamperFirst(id string, corrupt func(a map[string]any, probs map[string]float64, chosen, other string)) func(*testing.T, wireRequest) fakeReply {
	var n atomic.Int32
	return func(*testing.T, wireRequest) fakeReply {
		if n.Add(1) > 1 {
			return fakeReply{}
		}
		return fakeReply{tamper: func(answers map[string]any) {
			a := answers[id].(map[string]any)
			probs, _ := a["probabilities"].(map[string]float64)
			chosen, _ := a["choice"].(string)
			other := ""
			for k := range probs {
				if k != chosen {
					other = k
					break
				}
			}
			corrupt(a, probs, chosen, other)
		}}
	}
}

// A malformed answer to any of the three questions fails Describe, is not
// cached, and the next valid reply recovers.
func TestDescribeRejectsMalformedAnswers(t *testing.T) {
	for _, id := range []string{"os_0", "cpu_0", "role_0"} {
		for name, corrupt := range corruptions {
			t.Run(id+"/"+name, func(t *testing.T) {
				c, fs := fakeJev(t, tamperFirst(id, corrupt))
				if _, _, err := c.Describe(context.Background(), "o/r", "", describeAssets); err == nil {
					t.Fatal("malformed answer accepted")
				}
				attrs, _, err := c.Describe(context.Background(), "o/r", "", describeAssets)
				if err != nil || len(attrs) != 1 || attrs[0].OS[osLinux] != 1 {
					t.Fatalf("next call did not recover: %+v, %v", attrs, err)
				}
				if fs.describes.Load() != 2 {
					t.Fatalf("describes=%d, want 2: the malformed reply must not be cached", fs.describes.Load())
				}
				if _, _, err := c.Describe(context.Background(), "o/r", "", describeAssets); err != nil || fs.describes.Load() != 2 {
					t.Fatalf("recovered result was not cached: err=%v describes=%d", err, fs.describes.Load())
				}
			})
		}
	}
}

// The fake answers every question with the full option set, and a file the
// role rule marks as a companion is a companion's argmax (roleMain .05).
func TestDescribeFakeAnswersAreCoherent(t *testing.T) {
	c, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply { return fakeReply{} })
	attrs, _, err := c.Describe(context.Background(), "o/r", "", assetsNamed("tool-linux-amd64.tar.gz", "tool-cli-linux-amd64.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if attrs[0].Main != 0.95 || attrs[1].Main != 0.05 {
		t.Fatalf("Main = %v, %v, want 0.95, 0.05", attrs[0].Main, attrs[1].Main)
	}
}

// Primary falls back to the rules (ok=false) when Describe's reply is invalid.
func TestPrimaryMalformedDescribeFallsBack(t *testing.T) {
	c, _ := fakeJev(t, tamperFirst("role_0", corruptions["choice is not the mode"]))
	p := &Primary{Client: c, Cache: cache.New()}
	if _, ok := p.Decide(context.Background(), "o/bad", macAssets, picker.MacOS, picker.ARM64, picker.PickOpts{}); ok {
		t.Fatal("malformed Describe reply must fall back to the rules")
	}
}
