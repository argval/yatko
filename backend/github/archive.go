package github

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"time"
)

const archiveTailLimit = 128 << 10

// ZIPPlatform reads only the ZIP directory at the end of a GitHub release
// asset. It never extracts or executes files. Missing/oversized directories,
// unsupported range responses, and network failures leave the platform unknown.
func (c *Client) ZIPPlatform(ctx context.Context, asset Asset) (string, error) {
	u, ok := releaseDownloadURL(asset)
	if !ok {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	n := min(asset.Size, archiveTailLimit)
	start := asset.Size - n
	tail, err := c.openAsset(ctx, u, asset.Size).fetch(start, n, n)
	if err != nil {
		return "", err
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
	r, err := boundedZIPReader(zipTail{bytes.NewReader(tail), start}, size)
	if err != nil {
		return ""
	}
	if macAppBundle(zipFileNames(r)) != "" {
		return "macos"
	}
	return ""
}

// zipFileNames lists regular entries, skipping directories and __MACOSX forks.
func zipFileNames(r *zip.Reader) []string {
	var names []string
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, "__MACOSX/") && !f.FileInfo().IsDir() {
			names = append(names, f.Name)
		}
	}
	return names
}

// macAppBundle returns the first bundle path ("dir/App.app") that has both an
// Info.plist and an executable under Contents/MacOS. __MACOSX resource forks
// by themselves are not evidence of a Mac app.
func macAppBundle(names []string) string {
	info, executable := map[string]bool{}, map[string]bool{}
	for _, name := range names {
		if strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		bundle, rest, ok := strings.Cut(name, ".app/Contents/")
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
			return bundle + ".app"
		}
	}
	return ""
}
