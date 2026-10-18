package handlers

import (
	"context"
	"log"
	"sync"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

// pick is the /dl and /api/link decision. The rule picker always runs (it is
// the fallback and the baseline); with a Primary set, Jev's decision replaces
// it whenever Jev answers in time. Requests that pin a file with ?asset= keep
// the rules, which see only that one file.
func (h *RedirectHandler) pick(c *gin.Context, owner, repo string, assets []github.Asset) assetPick {
	p := pickReleaseAsset(c, assets)
	if h.Primary == nil || c.Query("asset") != "" {
		return p
	}
	opts := picker.PickOpts{Prefer: p.Prefer, Libc: p.Libc, UserAgent: p.UA}
	d, ok := h.Primary.Decide(c.Request.Context(), owner+"/"+repo, assets, p.Platform, p.Arch, opts)
	if !ok {
		return p
	}
	logPrimaryDisagreement(owner, repo, p, d)
	p.Decision = d
	return p
}

// logPrimaryDisagreement records where Jev and the rules differ, the data for
// judging the switch. Agreement is the common case and stays silent.
func logPrimaryDisagreement(owner, repo string, p assetPick, jev picker.AssetDecision) {
	rules, primary := assetName(p.Decision.Asset), assetName(jev.Asset)
	if rules == primary && p.Decision.ShouldAutoSelect() == jev.ShouldAutoSelect() {
		return
	}
	log.Printf("jev_primary_diff owner=%s repo=%s platform=%s arch=%s rules=%q rules_conf=%s jev=%q jev_conf=%s jev_auto=%v",
		owner, repo, p.Platform, p.Arch, rules, p.Decision.Confidence, primary, jev.Confidence, jev.ShouldAutoSelect())
}

// buildPicks fills the release page's per-target picks, using Jev where it
// answers within the shared wait and the rules everywhere else.
func (h *RedirectHandler) buildPicks(ctx context.Context, owner, repo string, assets []github.Asset) map[string]releasePick {
	if h.Primary == nil {
		return buildReleasePicks(assets)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	jevPicks := map[string]picker.AssetDecision{}
	for _, t := range releasePickTargets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if d, ok := h.Primary.Decide(ctx, owner+"/"+repo, assets, t.platform, t.arch, picker.PickOpts{}); ok {
				mu.Lock()
				jevPicks[releasePickKey(t.platform, t.arch)] = d
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return buildPicksWith(assets, func(platform picker.Platform, arch picker.Arch) picker.AssetDecision {
		if d, ok := jevPicks[releasePickKey(platform, arch)]; ok {
			return d
		}
		return picker.DecideAsset(assets, platform, arch, picker.PickOpts{})
	})
}
