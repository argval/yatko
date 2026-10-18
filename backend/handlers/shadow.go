package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

// ShadowDecider is a second asset picker run only for comparison logs (Jev in
// production). Its result never reaches the visitor; nil disables shadow mode.
type ShadowDecider func(ctx context.Context, repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, jev.Usage, error)

const (
	maxShadowBuckets  = 10_000
	maxShadowInFlight = 4
	shadowTimeout     = 5 * time.Second
)

// startShadow runs the shadow picker in the background, at most once per
// owner/repo/tag/platform/arch/prefer/libc bucket per process. It never blocks:
// when all in-flight slots are busy, the call is skipped.
func (h *RedirectHandler) startShadow(c *gin.Context, owner, repo string, release *github.Release, p assetPick) {
	// Pinned ?asset= requests compare a one-file list against the full list; skip them.
	if h.Shadow == nil || c.Query("asset") != "" {
		return
	}
	bucket := strings.Join([]string{owner, repo, release.TagName, string(p.Platform), string(p.Arch), p.Prefer, string(p.Libc)}, "|")

	h.shadowMu.Lock()
	if _, seen := h.shadowSeen[bucket]; seen {
		h.shadowMu.Unlock()
		return
	}
	select {
	case h.shadowSlots <- struct{}{}:
	default:
		h.shadowMu.Unlock()
		return
	}
	// ponytail: forget every bucket at 10k entries, so a few repeat Jev calls
	// follow each reset; switch to an LRU if that cost shows up.
	if len(h.shadowSeen) >= maxShadowBuckets {
		clear(h.shadowSeen)
	}
	h.shadowSeen[bucket] = struct{}{}
	h.shadowMu.Unlock()

	h.shadowWG.Add(1)
	go func() {
		defer h.shadowWG.Done()
		defer func() { <-h.shadowSlots }()
		// A shadow-only bug must never crash the server that serves visitors.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("jev_shadow_error owner=%s repo=%s err=%q", owner, repo, "panic")
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), shadowTimeout)
		defer cancel()
		start := time.Now()
		opts := picker.PickOpts{Prefer: p.Prefer, Libc: p.Libc, UserAgent: p.UA}
		d, usage, err := h.Shadow(ctx, owner+"/"+repo, release.Assets, p.Platform, p.Arch, opts)
		if err != nil {
			log.Printf("jev_shadow_error owner=%s repo=%s err=%q", owner, repo, err.Error())
			return
		}
		logJevShadow(owner, repo, release.TagName, p, d, usage, time.Since(start))
	}()
}

func logJevShadow(owner, repo, tag string, p assetPick, d picker.AssetDecision, usage jev.Usage, latency time.Duration) {
	rules, shadow := assetName(p.Decision.Asset), assetName(d.Asset)
	agree := rules == shadow && p.Decision.ShouldAutoSelect() == d.ShouldAutoSelect()
	log.Printf(
		"jev_shadow owner=%s repo=%s tag=%s platform=%s arch=%s prefer=%s libc=%s rules=%q rules_conf=%s rules_auto=%v jev=%q jev_conf=%s jev_auto=%v agree=%v consistent=%v tokens=%d latency_ms=%d model=%s",
		owner, repo, tag, p.Platform, p.Arch, p.Prefer, p.Libc,
		rules, p.Decision.Confidence, p.Decision.ShouldAutoSelect(),
		shadow, d.Confidence, d.ShouldAutoSelect(),
		agree, usage.Consistent, usage.InputTokens, latency.Milliseconds(), usage.Model,
	)
}

func assetName(a *github.Asset) string {
	if a == nil {
		return ""
	}
	return a.Name
}
