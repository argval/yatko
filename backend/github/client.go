package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// APIError is returned when the GitHub API responds with a non-200 status.
// Handlers inspect it to forward the appropriate HTTP status to the client.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string { return e.Message }

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
	DownloadCount      int64  `json:"download_count"`
}

type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	HTMLURL     string  `json:"html_url"`
	Prerelease  bool    `json:"prerelease"`
	Assets      []Asset `json:"assets"`
}

// ReleaseSummary is a lightweight release for list endpoints (no assets, no body).
type ReleaseSummary struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
}

// Repo holds the subset of GitHub repo metadata Yatko needs.
type Repo struct {
	Description string `json:"description"`
	Owner       struct {
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
}

type Client struct {
	httpClient *http.Client
	token      string
	// remaining is the last-seen X-RateLimit-Remaining value, -1 until the
	// first response tells us. Read/written atomically since handlers hit
	// this concurrently.
	remaining int32
}

const (
	// maxREADMESize caps README fetches at 1 MB.
	maxREADMESize = 1 << 20
	// maxAPIResponseSize caps JSON API responses (releases, repo metadata) at 5 MB,
	// generous enough for releases with hundreds of assets.
	maxAPIResponseSize = 5 << 20
	// rateLimitReserve is the headroom kept below GitHub's rate limit. Once
	// remaining requests drop below this, new fetches are refused so that
	// repos already in cache.Cache keep serving their last known-good value
	// (via FetchCached's stale-while-error fallback) instead of every repo,
	// hot or cold, racing to fail against an exhausted quota.
	rateLimitReserve = 200
)

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		token:      os.Getenv("GITHUB_TOKEN"),
		remaining:  -1,
	}
}

// checkBudget refuses a new request once observed rate-limit headroom is
// below rateLimitReserve. Unknown budget (before the first response) is
// treated as fine.
func (c *Client) checkBudget() error {
	if remaining := atomic.LoadInt32(&c.remaining); remaining >= 0 && remaining < rateLimitReserve {
		return &APIError{StatusCode: http.StatusTooManyRequests, Message: "GitHub rate limit nearly exhausted, refusing new request"}
	}
	return nil
}

func (c *Client) recordRateLimit(h http.Header) {
	if v := h.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			atomic.StoreInt32(&c.remaining, int32(n))
		}
	}
}

func (c *Client) newRequest(ctx context.Context, url, accept, etag string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "Yatko/1.0")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	return req, nil
}

func (c *Client) checkStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return &APIError{StatusCode: resp.StatusCode, Message: string(body)}
}

// conditionalGet performs a GET with an optional If-None-Match header.
//
// GitHub does not count conditional requests that return 304 Not Modified
// against the caller's rate limit, so callers should always pass the last
// known ETag when revalidating a previously-cached value. When the origin
// reports 304, notModified is true and body is nil.
func (c *Client) conditionalGet(ctx context.Context, url, accept, etag string, maxSize int64) (body []byte, newETag string, notModified bool, err error) {
	if err := c.checkBudget(); err != nil {
		return nil, "", false, err
	}

	req, err := c.newRequest(ctx, url, accept, etag)
	if err != nil {
		return nil, "", false, fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", false, fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()
	c.recordRateLimit(resp.Header)

	if resp.StatusCode == http.StatusNotModified {
		return nil, etag, true, nil
	}
	if err := c.checkStatus(resp); err != nil {
		return nil, "", false, err
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return nil, "", false, fmt.Errorf("reading response: %w", err)
	}
	return data, resp.Header.Get("ETag"), false, nil
}

// GetLatestRelease fetches the latest release. Pass the previously-seen etag
// (empty string if none) to revalidate cheaply; when notModified is true, the
// returned Release is nil and the caller should keep using its cached value.
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo, etag string) (*Release, string, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	body, newETag, notModified, err := c.conditionalGet(ctx, url, "application/vnd.github.v3+json", etag, maxAPIResponseSize)
	if err != nil {
		return nil, "", false, err
	}
	if notModified {
		return nil, newETag, true, nil
	}
	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, "", false, fmt.Errorf("decoding release: %w", err)
	}
	return &release, newETag, false, nil
}

func (c *Client) GetReleaseByTag(ctx context.Context, owner, repo, tag, etag string) (*Release, string, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	body, newETag, notModified, err := c.conditionalGet(ctx, url, "application/vnd.github.v3+json", etag, maxAPIResponseSize)
	if err != nil {
		return nil, "", false, err
	}
	if notModified {
		return nil, newETag, true, nil
	}
	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, "", false, fmt.Errorf("decoding release: %w", err)
	}
	return &release, newETag, false, nil
}

// GetReleases fetches the first 30 releases (summary only, no assets/body).
func (c *Client) GetReleases(ctx context.Context, owner, repo, etag string) ([]ReleaseSummary, string, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=30", owner, repo)
	body, newETag, notModified, err := c.conditionalGet(ctx, url, "application/vnd.github.v3+json", etag, maxAPIResponseSize)
	if err != nil {
		return nil, "", false, err
	}
	if notModified {
		return nil, newETag, true, nil
	}
	var releases []ReleaseSummary
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, "", false, fmt.Errorf("decoding releases: %w", err)
	}
	return releases, newETag, false, nil
}

// GetRepo fetches basic repo metadata (description and owner avatar).
func (c *Client) GetRepo(ctx context.Context, owner, repo, etag string) (*Repo, string, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	body, newETag, notModified, err := c.conditionalGet(ctx, url, "application/vnd.github.v3+json", etag, maxAPIResponseSize)
	if err != nil {
		return nil, "", false, err
	}
	if notModified {
		return nil, newETag, true, nil
	}
	var r Repo
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "", false, fmt.Errorf("decoding repo: %w", err)
	}
	return &r, newETag, false, nil
}

// GetREADME fetches the raw README content for a repo, capped at maxREADMESize.
// A missing README (404) is not an error — it returns ("", "", false, nil).
func (c *Client) GetREADME(ctx context.Context, owner, repo, etag string) (string, string, bool, error) {
	if err := c.checkBudget(); err != nil {
		return "", "", false, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/readme", owner, repo)
	req, err := c.newRequest(ctx, url, "application/vnd.github.v3.raw", etag)
	if err != nil {
		return "", "", false, fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", false, fmt.Errorf("fetching readme: %w", err)
	}
	defer resp.Body.Close()
	c.recordRateLimit(resp.Header)

	if resp.StatusCode == http.StatusNotModified {
		return "", etag, true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", "", false, nil // no README is fine
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", false, nil // non-critical, skip
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxREADMESize))
	if err != nil {
		return "", "", false, fmt.Errorf("reading readme: %w", err)
	}
	return string(body), resp.Header.Get("ETag"), false, nil
}
