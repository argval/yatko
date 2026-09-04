package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

type RedirectHandler struct {
	gh    *github.Client
	cache *cache.Cache
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

	ua := c.GetHeader("User-Agent")
	// Query params mirror /api/link so scripts can pin OS/arch/format/libc.
	platform := picker.ResolvePlatform(c.Query("platform"), ua)
	arch := picker.ResolveArch(c.Query("arch"), ua)
	prefer := picker.ResolvePrefer(c.Query("prefer"))
	libc := picker.ResolveLibc(c.Query("libc"))
	decision := picker.DecideAsset(release.Assets, platform, arch, picker.PickOpts{
		Prefer:    prefer,
		Libc:      libc,
		UserAgent: ua,
	})
	if !decision.ShouldAutoSelect() {
		respondDownloadMiss(c, owner, repo, release, platform, arch, prefer, libc, decision)
		return
	}
	if decision.Confidence != picker.ConfidenceHigh {
		log.Printf(
			"picker_shadow owner=%s repo=%s platform=%s arch=%s confidence=%s file=%q unknown=%q reasons=%q",
			owner, repo, platform, arch, decision.Confidence, decision.Asset.Name,
			picker.UnknownTokens(decision.Asset.Name), decision.Reasons,
		)
	}

	c.Redirect(http.StatusFound, decision.Asset.BrowserDownloadURL)
}

// getRelease returns the release for owner/repo — the latest when version is
// empty, otherwise the given tag — transparently caching and revalidating via
// conditional GitHub requests (see cache.FetchCached).
func (h *RedirectHandler) getRelease(c *gin.Context, owner, repo, version string) (*github.Release, error) {
	if version == "" {
		key := cache.ReleaseKey(owner, repo)
		return cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
			return h.gh.GetLatestRelease(ctx, owner, repo, etag)
		})
	}
	key := cache.ReleaseTagKey(owner, repo, version)
	return cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
		return h.gh.GetReleaseByTag(ctx, owner, repo, version, etag)
	})
}
