package github

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// download reuses only a complete file under the same GitHub asset identity.
// Its client shares the range reader's redirect policy and never sends an API token.
func (d *DeepInspector) download(ctx context.Context, asset Asset) (string, func(), error) {
	rawURL, ok := releaseDownloadURL(asset)
	if !ok || asset.Size > deepMaxDownload {
		return "", func() {}, errors.New("invalid public release asset or size")
	}
	dir := d.DownloadDir
	ephemeral := dir == ""
	if ephemeral {
		var err error
		dir, err = os.MkdirTemp("", "yatko-download-")
		if err != nil {
			return "", func() {}, err
		}
	} else if err := os.MkdirAll(dir, 0700); err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		if ephemeral {
			os.RemoveAll(dir)
		}
	}
	identity, _ := json.Marshal(struct {
		ID      int64
		URL     string
		Name    string
		Size    int64
		Updated string
		Digest  string
	}{asset.ID, rawURL, asset.Name, asset.Size, asset.UpdatedAt, asset.Digest})
	filename := filepath.Join(dir, fmt.Sprintf("%x.asset", sha256.Sum256(identity)))
	if info, err := os.Lstat(filename); err == nil && info.Mode().IsRegular() && info.Size() == asset.Size {
		return filename, cleanup, nil
	}
	f, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	c := d.client
	if c == nil {
		c = &Client{httpClient: &http.Client{}}
	}
	hc := c.openAsset(ctx, rawURL, asset.Size).client
	hc.Timeout = 10 * time.Minute
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		cleanup()
		return "", func() {}, errors.New("invalid artifact download request")
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := hc.Do(req)
	if err != nil {
		cleanup()
		if ctx.Err() != nil {
			return "", func() {}, ctx.Err()
		}
		// URL errors may include a signed redirect URL. Keep it out of persisted errors.
		return "", func() {}, errors.New("artifact download failed (network or refused redirect)")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || (resp.ContentLength >= 0 && resp.ContentLength != asset.Size) ||
		(resp.Header.Get("Content-Encoding") != "" && resp.Header.Get("Content-Encoding") != "identity") {
		cleanup()
		return "", func() {}, fmt.Errorf("artifact download returned invalid status, length, or encoding (HTTP %d)", resp.StatusCode)
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, asset.Size+1))
	if err != nil || n != asset.Size {
		cleanup()
		return "", func() {}, errors.New("artifact download incomplete or exceeded declared size")
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := os.Rename(f.Name(), filename); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return filename, cleanup, nil
}
