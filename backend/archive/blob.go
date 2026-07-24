package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const blobAPIVersion = "12"
const blobAPIBase = "https://vercel.com/api/blob"

// BlobClient uploads and deletes objects in Vercel Blob via the REST API.
type BlobClient struct {
	token      string
	httpClient *http.Client
}

// NewBlobClientFromEnv returns a Blob client when BLOB_READ_WRITE_TOKEN is set.
func NewBlobClientFromEnv() *BlobClient {
	token := os.Getenv("BLOB_READ_WRITE_TOKEN")
	if token == "" {
		return nil
	}
	return &BlobClient{
		token: token,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

type putResponse struct {
	URL      string `json:"url"`
	Pathname string `json:"pathname"`
}

type listResponse struct {
	Blobs []struct {
		URL      string `json:"url"`
		Pathname string `json:"pathname"`
	} `json:"blobs"`
	Cursor  string `json:"cursor"`
	HasMore bool   `json:"hasMore"`
}

// Put uploads body at pathname (public, overwrite, no random suffix).
func (b *BlobClient) Put(ctx context.Context, pathname string, body []byte, contentType string) (urlStr, storedPath string, err error) {
	if b == nil || b.token == "" {
		return "", "", fmt.Errorf("blob token not configured")
	}
	q := url.Values{"pathname": {pathname}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, blobAPIBase+"?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("x-api-version", blobAPIVersion)
	req.Header.Set("x-vercel-blob-access", "public")
	req.Header.Set("x-add-random-suffix", "0")
	req.Header.Set("x-allow-overwrite", "1")
	if contentType != "" {
		req.Header.Set("x-content-type", contentType)
	}
	req.Header.Set("x-content-length", fmt.Sprintf("%d", len(body)))

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("blob put %s: %s: %s", pathname, resp.Status, truncate(string(data), 300))
	}
	var out putResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", "", fmt.Errorf("blob put decode: %w", err)
	}
	if out.URL == "" {
		return "", "", fmt.Errorf("blob put: empty url in response")
	}
	path := out.Pathname
	if path == "" {
		path = pathname
	}
	return out.URL, path, nil
}

// DeletePrefix lists and deletes all blobs under prefix.
func (b *BlobClient) DeletePrefix(ctx context.Context, prefix string) error {
	if b == nil || b.token == "" {
		return fmt.Errorf("blob token not configured")
	}
	var urls []string
	cursor := ""
	for {
		q := url.Values{"prefix": {prefix}, "limit": {"1000"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobAPIBase+"?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+b.token)
		req.Header.Set("x-api-version", blobAPIVersion)
		resp, err := b.httpClient.Do(req)
		if err != nil {
			return err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("blob list %s: %s: %s", prefix, resp.Status, truncate(string(data), 300))
		}
		var list listResponse
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		for _, blob := range list.Blobs {
			if blob.URL != "" {
				urls = append(urls, blob.URL)
			}
		}
		if !list.HasMore || list.Cursor == "" {
			break
		}
		cursor = list.Cursor
	}
	if len(urls) == 0 {
		return nil
	}
	return b.deleteURLs(ctx, urls)
}

func (b *BlobClient) deleteURLs(ctx context.Context, urls []string) error {
	body, err := json.Marshal(map[string]any{"urls": urls})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, blobAPIBase+"/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("x-api-version", blobAPIVersion)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("blob delete: %s: %s", resp.Status, truncate(string(data), 300))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
