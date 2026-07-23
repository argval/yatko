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

// HandleREADME serves /api/readme/:owner/:repo — raw README for install-command
// extraction and the About section. Kept off the critical /api/release path so
// the download CTA can paint without waiting on a potentially large README.
func (h *PageHandler) HandleREADME(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")
	if secret := os.Getenv("CACHE_REFRESH_SECRET"); secret != "" && c.Query("refresh") == secret {
		_ = h.cache.Invalidate(c.Request.Context(), cache.ReadmeKey(owner, repo))
	}
	c.JSON(http.StatusOK, gin.H{"readme": h.getREADME(c, owner, repo)})
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
		_ = h.cache.Invalidate(c.Request.Context(), cache.DescriptionKey(owner, repo))
	}

	// Release, repo metadata, and the version list are independent lookups —
	// run them concurrently. README is served from /api/readme instead so it
	// does not block this response. c.Copy() is gin's documented way to pass a
	// *gin.Context into a goroutine.
	var release *github.Release
	var meta repoMeta
	var repoFound bool
	var releases []github.ReleaseSummary
	var g errgroup.Group
	g.Go(func() error {
		var err error
		release, err = h.redirect.getRelease(c.Copy(), owner, repo, version)
		return err
	})
	g.Go(func() error {
		meta, repoFound = h.getRepoMeta(c.Copy(), owner, repo)
		return nil
	})
	g.Go(func() error {
		releases = h.getReleases(c.Copy(), owner, repo)
		return nil
	})
	if err := g.Wait(); err != nil {
		log.Printf("page: error fetching release %q for %s/%s: %v", version, owner, repo, err)
		status := httpStatusFromError(err)
		body := gin.H{"error": publicErrorMessage(err)}
		// GitHub's releases/latest 404s both for a repo that doesn't exist and
		// for a real repo with zero releases; repoFound (from the separately
		// fetched repo metadata) disambiguates the two for the frontend.
		if status == http.StatusNotFound && repoFound {
			body["reason"] = "no_releases"
		}
		c.JSON(status, body)
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
		"releases":     releases,
	})
}

// repoMeta is the subset of repo metadata cached for the page/OG-image
// response: description and owner avatar.
type repoMeta struct {
	Description string `json:"description"`
	AvatarURL   string `json:"avatar_url"`
}

// getRepoMeta also reports whether the repo itself was confirmed to exist
// (used to disambiguate a "repo not found" 404 from a "no releases yet" 404).
func (h *PageHandler) getRepoMeta(c *gin.Context, owner, repo string) (repoMeta, bool) {
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
		return repoMeta{}, false
	}
	return meta, true
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

func (h *PageHandler) getReleases(c *gin.Context, owner, repo string) []github.ReleaseSummary {
	key := cache.ReleasesKey(owner, repo)
	releases, err := cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) ([]github.ReleaseSummary, string, bool, error) {
		return h.gh.GetReleases(ctx, owner, repo, etag)
	})
	if err != nil {
		log.Printf("releases fetch error for %s/%s: %v", owner, repo, err)
		return nil
	}
	return releases
}
