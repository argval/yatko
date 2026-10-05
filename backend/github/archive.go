package github

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const archiveTailLimit = 128 << 10

// ZIPPlatform reads only the ZIP directory at the end of a GitHub release
// asset. It never extracts or executes files. Missing/oversized directories,
// unsupported range responses, and network failures leave the platform unknown.
func (c *Client) ZIPPlatform(ctx context.Context, asset Asset) (string, error) {
	u, err := url.Parse(asset.BrowserDownloadURL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || !strings.Contains(u.Path, "/releases/download/") || asset.Size <= 0 {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	start := max(int64(0), asset.Size-archiveTailLimit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, asset.Size-1))
	// Download requests must not carry the GitHub API token.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPartialContent:
		if resp.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, asset.Size-1, asset.Size) {
			return "", nil
		}
	case http.StatusOK:
		if start != 0 {
			return "", nil
		}
	default:
		return "", fmt.Errorf("ZIP range request: HTTP %d", resp.StatusCode)
	}
	tail, err := io.ReadAll(io.LimitReader(resp.Body, archiveTailLimit+1))
	if err != nil {
		return "", err
	}
	if int64(len(tail)) != asset.Size-start {
		return "", io.ErrUnexpectedEOF
	}
	return zipPlatform(tail, start, asset.Size), nil
}

type zipTail struct {
	*bytes.Reader
	start int64
}

func (r zipTail) ReadAt(p []byte, off int64) (int, error) {
	if off < r.start {
		return 0, io.EOF
	}
	return r.Reader.ReadAt(p, off-r.start)
}

func zipPlatform(tail []byte, start, size int64) string {
	r, err := zip.NewReader(zipTail{bytes.NewReader(tail), start}, size)
	if err != nil {
		return ""
	}
	// Require both an Info.plist and an executable in the same app bundle.
	// __MACOSX resource forks by themselves are not evidence of a Mac app.
	info, executable := map[string]bool{}, map[string]bool{}
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "__MACOSX/") || f.FileInfo().IsDir() {
			continue
		}
		bundle, rest, ok := strings.Cut(f.Name, ".app/Contents/")
		if !ok {
			continue
		}
		if rest == "Info.plist" {
			info[bundle] = true
		}
		if strings.HasPrefix(rest, "MacOS/") && len(strings.TrimPrefix(rest, "MacOS/")) > 0 {
			executable[bundle] = true
		}
		if info[bundle] && executable[bundle] {
			return "macos"
		}
	}
	return ""
}
