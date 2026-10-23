package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

const optionalRepoMetaBudget = 100 * time.Millisecond

// warmRepoMetaBudget bounds the background repo description lookup for warmPick.
const warmRepoMetaBudget = 5 * time.Second

// pick asks Jev for the full release; only service failure invokes rule ranking.
// A previously offered pin is revalidated before asking for a new preference.
func (h *RedirectHandler) pick(c *gin.Context, owner, repo string, release *github.Release) assetPick {
	assets := release.Assets
	p := resolveAssetPick(c)
	if h.Primary != nil {
		if decision, ok := h.offeredDownload(c, owner, repo, release, p); ok {
			p.Decision = decision
			return p
		}
		p.Product = h.optionalProductDescription(c.Request.Context(), owner, repo)
		if decision, ok := h.Primary.Decide(c.Request.Context(), owner+"/"+repo, assets, p.Platform, p.Arch, p.opts()); ok {
			p.Decision = decision
			logPrimaryDisagreement(owner, repo, p, ruleDecision(c, assets, p))
			if name := c.Query("asset"); name != "" && decision.ShouldAutoSelect() {
				if decision.Asset.Name != name || !jev.ValidateSelection(*decision.Asset, p.Platform, p.Arch, p.opts()) {
					p.Decision = picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow,
						Reasons: []string{"pinned download could not be validated for this visitor"}}
				}
			}
			return p
		}
	}
	p.Decision = ruleDecision(c, assets, p)
	return p
}

// warmPick starts Jev's per-release Describe in the background when the release
// page loads, so a direct visit's first pick only waits for its own Choice. It
// fetches the repo description first and caches it, so the pick's
// optionalProductDescription reads the same text and the Describe keys match.
func (h *RedirectHandler) warmPick(parent context.Context, owner, repo string, assets []github.Asset) {
	if h.Primary == nil {
		return
	}
	h.warming.Add(1)
	go func() {
		defer h.warming.Done()
		ctx := context.WithoutCancel(parent)
		metaCtx, cancel := context.WithTimeout(ctx, warmRepoMetaBudget)
		meta, _ := h.getRepoMeta(metaCtx, owner, repo)
		cancel()
		h.Primary.Warm(ctx, owner+"/"+repo, strings.TrimSpace(meta.Description), assets)
	}()
}

// Optional context cannot share a short-lived cache fill with the page's required lookup.
func (h *RedirectHandler) optionalProductDescription(parent context.Context, owner, repo string) string {
	ctx, cancel := context.WithTimeout(parent, optionalRepoMetaBudget)
	defer cancel()
	key := cache.DescriptionKey(owner, repo)
	meta, ok := cache.GetCached[repoMeta](ctx, h.cache, key)
	if !ok {
		var err error
		meta, _, _, err = h.fetchRepoMeta(ctx, owner, repo, "")
		if err != nil {
			return ""
		}
		_ = cache.StoreCached(ctx, h.cache, key, meta)
	}
	return strings.TrimSpace(meta.Description)
}

// logPrimaryDisagreement records where Jev and the rules differ, with Jev's
// probabilities, as the production signal for tuning the gate thresholds.
// Agreement is the common case and stays silent; the rules cost ~0.1ms.
func logPrimaryDisagreement(owner, repo string, p assetPick, rules picker.AssetDecision) {
	jevName, ruleName := assetName(p.Decision.Asset), assetName(rules.Asset)
	if jevName == ruleName && p.Decision.ShouldAutoSelect() == rules.ShouldAutoSelect() {
		return
	}
	log.Printf("jev_primary_diff owner=%s repo=%s platform=%s arch=%s libc=%s prefer=%s rules=%q rules_conf=%s jev=%q jev_conf=%s jev_auto=%v jev_reasons=%q",
		owner, repo, p.Platform, p.Arch, p.Libc, p.Prefer, ruleName, rules.Confidence, jevName, p.Decision.Confidence,
		p.Decision.ShouldAutoSelect(), strings.Join(p.Decision.Reasons, "; "))
}

func assetName(a *github.Asset) string {
	if a == nil {
		return ""
	}
	return a.Name
}

// buildPicks avoids generic primary decisions without the visitor's real context.
// When Jev is disabled the existing deterministic table remains available.
func (h *RedirectHandler) buildPicks(_ context.Context, _, _ string, assets []github.Asset) map[string]releasePick {
	if h.Primary != nil {
		return nil
	}
	return buildReleasePicks(assets)
}
