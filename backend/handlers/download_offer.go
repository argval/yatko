package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

const downloadOfferBudget = 100 * time.Millisecond

type downloadOffer struct {
	Confidence picker.Confidence `json:"confidence"`
	Reasons    []string          `json:"reasons"`
	ExpiresAt  time.Time         `json:"expires_at"`
}

// downloadOfferKey binds an offer to the visitor, release and exact file.
// Inspection results can change; current compatibility is checked separately.
func downloadOfferKey(owner, repo string, release *github.Release, asset github.Asset, p assetPick) string {
	opts := p.opts()
	opts.ProductDescription = ""
	body, _ := json.Marshal(struct {
		Repo, Tag, Identity string
		Platform            picker.Platform
		Arch                picker.Arch
		Visitor             picker.PickOpts
	}{owner + "/" + repo, release.TagName, artifactIdentity(asset), p.Platform, p.Arch, opts})
	return fmt.Sprintf("download-offer:v1:%x", sha256.Sum256(body))
}

// rememberDownload records only successful API offers, in the existing cache.
// A later preference change must not invalidate a download already displayed.
func (h *RedirectHandler) rememberDownload(c *gin.Context, owner, repo string, release *github.Release, p assetPick) {
	if h.Primary == nil || c.Query("asset") != "" || !p.Decision.ShouldAutoSelect() {
		return
	}
	key := downloadOfferKey(owner, repo, release, *p.Decision.Asset, p)
	offer := downloadOffer{p.Decision.Confidence, p.Decision.Reasons, time.Now().Add(cache.HardTTL)}
	ctx, cancel := context.WithTimeout(c.Request.Context(), downloadOfferBudget)
	defer cancel()
	// StoreCached saves locally before the bounded shared write.
	err := cache.StoreCached(ctx, h.cache, key, offer)
	if err != nil {
		c.Error(err)
	}
}

// offeredDownload accepts only an issued pin with unchanged file identity and
// visitor context. New incompatible, failed or conflicting evidence rejects it.
func (h *RedirectHandler) offeredDownload(c *gin.Context, owner, repo string, release *github.Release, p assetPick) (picker.AssetDecision, bool) {
	name := c.Query("asset")
	if name == "" {
		return picker.AssetDecision{}, false
	}
	for i := range release.Assets {
		asset := &release.Assets[i]
		if asset.Name != name || !jev.ValidateSelection(*asset, p.Platform, p.Arch, p.opts()) {
			continue
		}
		key := downloadOfferKey(owner, repo, release, *asset, p)
		if offer, ok := cache.GetCached[downloadOffer](c.Request.Context(), h.cache, key); ok &&
			offer.ExpiresAt.After(time.Now()) &&
			(offer.Confidence == picker.ConfidenceMedium || offer.Confidence == picker.ConfidenceHigh) {
			return picker.AssetDecision{Status: "selected", Asset: asset, Confidence: offer.Confidence,
				Reasons: offer.Reasons, Facts: picker.FactsForAsset(*asset)}, true
		}
	}
	return picker.AssetDecision{}, false
}
