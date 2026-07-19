package handlers

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type PageHandler struct {
	redirect *RedirectHandler
	gh       *github.Client
	cache    *cache.Cache
}

func NewPageHandler(r *RedirectHandler, gh *github.Client, c *cache.Cache) *PageHandler {
	return &PageHandler{redirect: r, gh: gh, cache: c}
}

func (h *PageHandler) Handle(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), "")
}

// HandleVersioned serves /api/release/:owner/:repo/:version — metadata for a specific tag.
func (h *PageHandler) HandleVersioned(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), c.Param("version"))
}

func (h *PageHandler) handle(c *gin.Context, owner, repo, version string) {
	// Cache bust is gated on CACHE_REFRESH_SECRET so unauthenticated
	// ?refresh=1 can't force GitHub re-fetches. When the secret is unset,
	// refresh is ignored (no public escape hatch).
	if secret := os.Getenv("CACHE_REFRESH_SECRET"); secret != "" && c.Query("refresh") == secret {
		key := cache.ReleaseKey(owner, repo)
		if version != "" {
			key = cache.ReleaseTagKey(owner, repo, version)
		}
		_ = h.cache.Invalidate(c.Request.Context(), key)
		_ = h.cache.Invalidate(c.Request.Context(), cache.ReleasesKey(owner, repo))
	}

	// Release, README, and repo metadata are independent lookups (each its own
	// cache.FetchCached call) - run them concurrently instead of paying three
	// sequential GitHub round trips on a cold cache. c.Copy() is gin's documented
	// way to pass a *gin.Context into a goroutine.
	var release *github.Release
	var readme string
	var meta repoMeta
	var g errgroup.Group
	g.Go(func() error {
		var err error
		release, err = h.redirect.getRelease(c.Copy(), owner, repo, version)
		return err
	})
	g.Go(func() error {
		readme = h.getREADME(c.Copy(), owner, repo)
		return nil
	})
	g.Go(func() error {
		meta = h.getRepoMeta(c.Copy(), owner, repo)
		return nil
	})
	if err := g.Wait(); err != nil {
		log.Printf("page: error fetching release %q for %s/%s: %v", version, owner, repo, err)
		c.JSON(httpStatusFromError(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"owner":        owner,
		"repo":         repo,
		"description":  meta.Description,
		"avatar_url":   meta.AvatarURL,
		"tag_name":     release.TagName,
		"name":         release.Name,
		"body":         release.Body,
		"published_at": release.PublishedAt,
		"html_url":     release.HTMLURL,
		"prerelease":   release.Prerelease,
		"assets":       release.Assets,
		"readme":       readme,
	})
}

// repoMeta is the subset of repo metadata cached for the page/OG-image
// response: description and owner avatar.
type repoMeta struct {
	Description string `json:"description"`
	AvatarURL   string `json:"avatar_url"`
}

func (h *PageHandler) getRepoMeta(c *gin.Context, owner, repo string) repoMeta {
	key := cache.DescriptionKey(owner, repo)
	meta, err := cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (repoMeta, string, bool, error) {
		data, newETag, notModified, err := h.gh.GetRepo(ctx, owner, repo, etag)
		if err != nil {
			return repoMeta{}, "", false, err
		}
		if notModified {
			return repoMeta{}, newETag, true, nil
		}
		return repoMeta{Description: data.Description, AvatarURL: data.Owner.AvatarURL}, newETag, false, nil
	})
	if err != nil {
		log.Printf("repo fetch error for %s/%s: %v", owner, repo, err)
		return repoMeta{}
	}
	return meta
}

func (h *PageHandler) getREADME(c *gin.Context, owner, repo string) string {
	key := cache.ReadmeKey(owner, repo)
	content, err := cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (string, string, bool, error) {
		return h.gh.GetREADME(ctx, owner, repo, etag)
	})
	if err != nil {
		log.Printf("readme fetch error for %s/%s: %v", owner, repo, err)
		return ""
	}
	return content
}
