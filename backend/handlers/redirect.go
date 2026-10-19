package handlers

import (
	"context"
	"log"
	"net/http"
	"slices"

	"github.com/argval/yatko/artifactscan"
	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/gin-gonic/gin"
)

type RedirectHandler struct {
	gh    *github.Client
	cache *cache.Cache
	// ScanStore adds durable full-file evidence; nil keeps inspection disabled.
	ScanStore *artifactscan.Store
	// Primary makes Jev the main decider, with the rule picker as fallback; nil keeps the rules.
	Primary *jev.Primary
}

func NewRedirectHandler(gh *github.Client, c *cache.Cache) *RedirectHandler {
	return &RedirectHandler{gh: gh, cache: c}
}

func (h *RedirectHandler) Handle(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), "")
}

// HandleVersioned handles /dl/:owner/:repo/:version — download a specific release tag.
func (h *RedirectHandler) HandleVersioned(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), c.Param("version"))
}

func (h *RedirectHandler) handle(c *gin.Context, owner, repo, version string) {
	release, err := h.getRelease(c, owner, repo, version)
	if err != nil {
		log.Printf("error fetching release %q for %s/%s: %v", version, owner, repo, err)
		c.JSON(httpStatusFromError(err), gin.H{"error": publicErrorMessage(err)})
		return
	}

	p := h.pick(c, owner, repo, release.Assets)
	if !p.Decision.ShouldAutoSelect() {
		respondDownloadMiss(c, owner, repo, release, p)
		return
	}
	logPickerShadow(owner, repo, p)

	c.Redirect(http.StatusFound, p.Decision.Asset.BrowserDownloadURL)
}

// getRelease returns the release for owner/repo — the latest when version is
// empty, otherwise the given tag — transparently caching and revalidating via
// conditional GitHub requests (see cache.FetchCached).
func (h *RedirectHandler) getRelease(c *gin.Context, owner, repo, version string) (*github.Release, error) {
	var release *github.Release
	var err error
	if version == "" {
		key := cache.ReleaseKey(owner, repo)
		release, err = cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
			return h.gh.GetLatestRelease(ctx, owner, repo, etag)
		})
	} else {
		key := cache.ReleaseTagKey(owner, repo, version)
		release, err = cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
			return h.gh.GetReleaseByTag(ctx, owner, repo, version, etag)
		})
	}
	if err != nil {
		return nil, err
	}
	// Copy before enrichment: the release can be shared by concurrent cache hits.
	result := *release
	result.Assets = slices.Clone(release.Assets)
	h.enrichArtifacts(c.Request.Context(), result.Assets)
	return &result, nil
}
