package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
)

// Manifest is the durable Redis record for a mirrored latest release.
type Manifest struct {
	Canonical string          `json:"canonical"`
	Source    string          `json:"source"`
	Tag       string          `json:"tag"`
	SyncedAt  time.Time       `json:"synced_at"`
	Assets    []ManifestAsset `json:"assets"`
}

// ManifestAsset is one mirrored file (release asset or source zip).
type ManifestAsset struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Pathname    string `json:"pathname"`
}

// RepoManifestKey is the Redis key for an alias's archive manifest.
func RepoManifestKey(owner, repo string) string {
	return fmt.Sprintf("archive:repo:%s/%s", owner, repo)
}

// LoadManifest reads the archive manifest for owner/repo, or nil if absent.
func LoadManifest(ctx context.Context, c *cache.Cache, owner, repo string) (*Manifest, error) {
	data, err := c.GetPermanent(ctx, RepoManifestKey(owner, repo))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decode archive manifest: %w", err)
	}
	return &m, nil
}

// SaveManifest writes the same manifest JSON under every alias in the group.
func SaveManifest(ctx context.Context, c *cache.Cache, g Group, m *Manifest) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	for _, a := range g.Aliases {
		owner, repo, err := SplitOwnerRepo(a)
		if err != nil {
			return err
		}
		if err := c.SetPermanent(ctx, RepoManifestKey(owner, repo), data); err != nil {
			return err
		}
	}
	return nil
}

// ToRelease builds a synthetic github.Release from a manifest for GitHub-down fallback.
func (m *Manifest) ToRelease(owner, repo string) *github.Release {
	assets := make([]github.Asset, 0, len(m.Assets))
	for _, a := range m.Assets {
		assets = append(assets, github.Asset{
			Name:               a.Name,
			BrowserDownloadURL: a.URL,
			Size:               a.Size,
			ContentType:        a.ContentType,
		})
	}
	tag := m.Tag
	return &github.Release{
		TagName:     tag,
		Name:        tag,
		PublishedAt: m.SyncedAt.UTC().Format(time.RFC3339),
		HTMLURL:     fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, tag),
		Assets:      assets,
	}
}

// Overlay rewrites BrowserDownloadURL on a clone of release to Blob URLs from
// the manifest. Injects the mirrored source zip when the release has no assets
// (or is missing that filename).
func Overlay(release *github.Release, m *Manifest) *github.Release {
	if release == nil || m == nil {
		return release
	}
	out := cloneRelease(release)
	byName := make(map[string]ManifestAsset, len(m.Assets))
	for _, a := range m.Assets {
		byName[a.Name] = a
	}
	for i := range out.Assets {
		if a, ok := byName[out.Assets[i].Name]; ok {
			out.Assets[i].BrowserDownloadURL = a.URL
			if a.Size > 0 {
				out.Assets[i].Size = a.Size
			}
			if a.ContentType != "" {
				out.Assets[i].ContentType = a.ContentType
			}
		}
	}
	present := map[string]bool{}
	for _, a := range out.Assets {
		present[a.Name] = true
	}
	for _, a := range m.Assets {
		if present[a.Name] {
			continue
		}
		out.Assets = append(out.Assets, github.Asset{
			Name:               a.Name,
			BrowserDownloadURL: a.URL,
			Size:               a.Size,
			ContentType:        a.ContentType,
		})
	}
	return out
}

func cloneRelease(r *github.Release) *github.Release {
	cp := *r
	if r.Assets != nil {
		cp.Assets = append([]github.Asset(nil), r.Assets...)
	}
	return &cp
}
