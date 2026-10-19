package handlers

import (
	"context"
	"log"
	"strings"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

// pick asks Jev for the full release; only service failure invokes rule ranking.
// A pinned filename must remain Jev's validated choice for the same visitor.
func (h *RedirectHandler) pick(c *gin.Context, owner, repo string, assets []github.Asset) assetPick {
	p := resolveAssetPick(c)
	if h.Primary != nil {
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

// buildPicks avoids generic primary decisions without the visitor's real context.
// When Jev is disabled the existing deterministic table remains available.
func (h *RedirectHandler) buildPicks(_ context.Context, _, _ string, assets []github.Asset) map[string]releasePick {
	if h.Primary != nil {
		return nil
	}
	return buildReleasePicks(assets)
}
