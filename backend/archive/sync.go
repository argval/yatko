package archive

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
)

const (
	maxAssetBytes = 200 << 20 // 200 MiB per asset
	sourceSuffix  = "-source.zip"
)

// Service mirrors allowlisted latest releases into Vercel Blob and overlays
// those URLs when serving /dl and landing pages.
type Service struct {
	groups []Group
	gh     *github.Client
	cache  *cache.Cache
	blob   *BlobClient
	http   *http.Client
	token  string
}

// New constructs an archive Service. groups may be empty (feature off).
// blob may be nil (serve-from-manifest still works; sync will fail).
func New(groups []Group, gh *github.Client, c *cache.Cache, blob *BlobClient) *Service {
	return &Service{
		groups: groups,
		gh:     gh,
		cache:  c,
		blob:   blob,
		http:   &http.Client{Timeout: 10 * time.Minute},
		token:  os.Getenv("GITHUB_TOKEN"),
	}
}

// NewFromEnv loads groups + blob from the environment.
func NewFromEnv(gh *github.Client, c *cache.Cache) (*Service, error) {
	groups, err := GroupsFromEnv()
	if err != nil {
		return nil, err
	}
	return New(groups, gh, c, NewBlobClientFromEnv()), nil
}

// Enabled reports whether any archive groups are configured.
func (s *Service) Enabled() bool {
	return s != nil && len(s.groups) > 0
}

// FindGroup returns the group containing owner/repo, or nil.
func (s *Service) FindGroup(owner, repo string) *Group {
	if s == nil {
		return nil
	}
	for i := range s.groups {
		if s.groups[i].Contains(owner, repo) {
			return &s.groups[i]
		}
	}
	return nil
}

// Groups returns configured groups (for sync-all).
func (s *Service) Groups() []Group {
	if s == nil {
		return nil
	}
	return s.groups
}

// SyncResult is the outcome of syncing one group.
type SyncResult struct {
	Canonical string `json:"canonical"`
	Source    string `json:"source"`
	Tag       string `json:"tag"`
	Assets    int    `json:"assets"`
}

// SyncGroup downloads the latest release (preferred alias first) into Blob
// and writes durable manifests for every alias.
func (s *Service) SyncGroup(ctx context.Context, g Group) (*SyncResult, error) {
	if s == nil || !s.Enabled() {
		return nil, fmt.Errorf("archive not configured")
	}
	if s.blob == nil {
		return nil, fmt.Errorf("BLOB_READ_WRITE_TOKEN not set")
	}
	canonical := g.Canonical()
	if canonical == "" {
		return nil, fmt.Errorf("empty archive group")
	}

	release, source, err := s.fetchLatest(ctx, g)
	if err != nil {
		return nil, err
	}
	owner, repo, err := SplitOwnerRepo(source)
	if err != nil {
		return nil, err
	}
	tag := release.TagName
	if tag == "" {
		return nil, fmt.Errorf("release missing tag_name from %s", source)
	}

	prevOwner, prevRepo, err := SplitOwnerRepo(canonical)
	if err != nil {
		return nil, err
	}
	prev, _ := LoadManifest(ctx, s.cache, prevOwner, prevRepo)

	var assets []ManifestAsset
	for _, a := range release.Assets {
		if a.BrowserDownloadURL == "" || a.Name == "" {
			continue
		}
		body, ctype, err := s.download(ctx, a.BrowserDownloadURL, a.ContentType)
		if err != nil {
			return nil, fmt.Errorf("download asset %s: %w", a.Name, err)
		}
		if ctype == "" {
			ctype = a.ContentType
		}
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		pathname := blobPath(canonical, tag, a.Name)
		urlStr, stored, err := s.blob.Put(ctx, pathname, body, ctype)
		if err != nil {
			return nil, fmt.Errorf("upload asset %s: %w", a.Name, err)
		}
		assets = append(assets, ManifestAsset{
			Name:        a.Name,
			URL:         urlStr,
			Size:        int64(len(body)),
			ContentType: ctype,
			Pathname:    stored,
		})
	}

	sourceName := sourceZipName(repo, tag)
	zipURL := release.ZipballURL
	if zipURL == "" {
		zipURL = fmt.Sprintf(
			"https://api.github.com/repos/%s/%s/zipball/%s",
			url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tag),
		)
	}
	zipBody, _, err := s.download(ctx, zipURL, "application/zip")
	if err != nil {
		return nil, fmt.Errorf("download source zip: %w", err)
	}
	zipPath := blobPath(canonical, tag, sourceName)
	zipURLOut, zipStored, err := s.blob.Put(ctx, zipPath, zipBody, "application/zip")
	if err != nil {
		return nil, fmt.Errorf("upload source zip: %w", err)
	}
	assets = append(assets, ManifestAsset{
		Name:        sourceName,
		URL:         zipURLOut,
		Size:        int64(len(zipBody)),
		ContentType: "application/zip",
		Pathname:    zipStored,
	})

	m := &Manifest{
		Canonical: canonical,
		Source:    source,
		Tag:       tag,
		SyncedAt:  time.Now().UTC(),
		Assets:    assets,
	}
	if err := SaveManifest(ctx, s.cache, g, m); err != nil {
		return nil, err
	}

	// Latest-only: drop previous tag's blobs after the new manifest is durable.
	if prev != nil && prev.Tag != "" && prev.Tag != tag {
		prefix := fmt.Sprintf("archive/%s/%s/", canonical, sanitizePath(prev.Tag))
		if err := s.blob.DeletePrefix(ctx, prefix); err != nil {
			log.Printf("archive: cleanup old tag %s/%s: %v", canonical, prev.Tag, err)
		}
	}

	return &SyncResult{
		Canonical: canonical,
		Source:    source,
		Tag:       tag,
		Assets:    len(assets),
	}, nil
}

// SyncAll syncs every configured group; returns per-group results (and combined error).
func (s *Service) SyncAll(ctx context.Context) ([]SyncResult, error) {
	var results []SyncResult
	var errs []string
	for _, g := range s.Groups() {
		res, err := s.SyncGroup(ctx, g)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", g.Canonical(), err))
			continue
		}
		results = append(results, *res)
	}
	if len(errs) > 0 {
		return results, fmt.Errorf("archive sync failures: %s", strings.Join(errs, "; "))
	}
	return results, nil
}

func (s *Service) fetchLatest(ctx context.Context, g Group) (*github.Release, string, error) {
	var errs []string
	for _, a := range g.Aliases {
		owner, repo, err := SplitOwnerRepo(a)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		rel, _, _, err := s.gh.GetLatestRelease(ctx, owner, repo, "")
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", a, err))
			log.Printf("archive: latest release %s failed: %v", a, err)
			continue
		}
		if rel != nil && rel.TagName != "" {
			return rel, a, nil
		}
	}
	// Last resort: source zip of default branch from the preferred alias.
	owner, repo, err := SplitOwnerRepo(g.Canonical())
	if err != nil {
		return nil, "", err
	}
	repoMeta, _, _, err := s.gh.GetRepo(ctx, owner, repo, "")
	branch := "main"
	if err == nil && repoMeta != nil && repoMeta.DefaultBranch != "" {
		branch = repoMeta.DefaultBranch
	} else if err != nil {
		errs = append(errs, fmt.Sprintf("repo %s: %v", g.Canonical(), err))
	}
	if len(errs) > 0 {
		log.Printf("archive: falling back to default-branch zipball for %s after: %s", g.Canonical(), strings.Join(errs, "; "))
	}
	return &github.Release{
		TagName:    branch,
		Name:       branch,
		ZipballURL: fmt.Sprintf("https://api.github.com/repos/%s/%s/zipball/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(branch)),
		HTMLURL:    fmt.Sprintf("https://github.com/%s/%s", owner, repo),
	}, g.Canonical(), nil
}

func (s *Service) download(ctx context.Context, rawURL, hintType string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Yatko/1.0")
	req.Header.Set("Accept", "application/octet-stream")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("%s: %s", resp.Status, truncate(string(body), 200))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxAssetBytes {
		return nil, "", fmt.Errorf("asset exceeds %d byte limit", maxAssetBytes)
	}
	ctype := resp.Header.Get("Content-Type")
	if ctype == "" {
		ctype = hintType
	}
	return data, ctype, nil
}

func blobPath(canonical, tag, filename string) string {
	return path.Join("archive", canonical, sanitizePath(tag), path.Base(filename))
}

func sanitizePath(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "..", ".")
	return s
}

func sourceZipName(repo, tag string) string {
	return fmt.Sprintf("%s-%s%s", repo, sanitizePath(tag), sourceSuffix)
}
