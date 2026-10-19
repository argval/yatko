// Package jev asks TypeSafe's Jev model (a System One decision model) to choose
// the release asset for a visitor. Code removes files that can never be
// installed or are proven incompatible; Jev chooses among the real files that
// remain (decide.go). It never invents a URL.
// Docs: https://docs.typesafe.ai/primitives/choice.md
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/argval/yatko/picker"
)

const (
	defaultEndpoint = "https://api.typesafe.ai/v1/systemone"
	// Model is the default, pinned, not "jev-latest": thresholds are tuned
	// against one version, and an alias can move under us. JEV_MODEL overrides
	// it to evaluate or adopt a new version.
	Model = "jev-1.14.0"
	// NoneOption lets Jev say no file fits instead of forcing a guess.
	NoneOption = "none of these files"
)

// Client calls the TypeSafe System One endpoint.
type Client struct {
	apiKey   string
	endpoint string
	model    string
	http     *http.Client
}

// ModelName is the versioned model this client sends.
func (c *Client) ModelName() string { return c.model }

// NewClientFromEnv returns nil when no API key is set, so callers can treat
// Jev as optional. TYPESAFE_API_KEY is the SDK name; TYPESAFE_API is accepted too.
func NewClientFromEnv() *Client {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		key = os.Getenv("TYPESAFE_API")
	}
	if key == "" {
		return nil
	}
	model := os.Getenv("JEV_MODEL")
	if model == "" {
		model = Model
	}
	return &Client{apiKey: key, endpoint: defaultEndpoint, model: model, http: &http.Client{Timeout: 10 * time.Second}}
}

// Usage is what one decision cost, for eval and logging.
type Usage struct {
	Model       string
	InputTokens int
}

// option is one Choice option; criteria must keep this order on the wire.
type option struct {
	name string
	desc any
}

// orderedCriteria marshals as a JSON object in slice order. encoding/json
// sorts map keys, which would destroy the forward/reverse order we rely on.
type orderedCriteria []option

func (o orderedCriteria) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, opt := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(opt.name)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(opt.desc)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

type question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]question `json:"questions"`
}

type answer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
	Noul          *float64           `json:"noul"` // nil when absent, so a missing answer is not "no"
}

type response struct {
	Model   string            `json:"model"`
	Answers map[string]answer `json:"answers"`
	Usage   struct {
		InputTokens int `json:"input_tokens"`
	} `json:"usage"`
}

var osNames = map[picker.Platform]string{
	picker.MacOS:   "macOS",
	picker.Windows: "Windows",
	picker.Linux:   "Linux",
	picker.Android: "Android",
	picker.IOS:     "iOS",
}

var cpuNames = map[picker.Arch]string{
	picker.AMD64: "64-bit Intel or AMD (filenames say x86_64, x64, amd64)",
	picker.ARM64: "64-bit ARM, e.g. Apple Silicon (filenames say arm64, aarch64)",
	picker.ARM:   "32-bit ARM (filenames say arm, armv6, armv7, armhf, armel)",
	picker.X86:   "32-bit Intel or AMD (filenames say 386, i386, i686, x86, win32)",
}

// post sends one request, retrying network errors and 429/529 with backoff as the
// API docs ask (https://docs.typesafe.ai/api.md#handling-rate-limits).
func (c *Client) post(ctx context.Context, req request) (*response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	backoff := 500 * time.Millisecond
	for attempt := 0; ; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")
		res, err := c.http.Do(httpReq)
		if err != nil {
			// Network errors are retried like 429/529, unless the caller gave up.
			if ctx.Err() == nil && attempt < 3 {
				backoff = c.wait(ctx, backoff)
				continue
			}
			return nil, fmt.Errorf("jev: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("jev: read body: %w", err)
		}
		if (res.StatusCode == http.StatusTooManyRequests || res.StatusCode == 529) && attempt < 3 {
			backoff = c.wait(ctx, backoff)
			continue
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("jev: HTTP %d: %s", res.StatusCode, bytes.TrimSpace(data))
		}
		var out response
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("jev: decode: %w", err)
		}
		return &out, nil
	}
}

// wait sleeps for backoff (or until ctx ends; the next request then fails
// fast) and returns the doubled backoff.
func (c *Client) wait(ctx context.Context, backoff time.Duration) time.Duration {
	select {
	case <-ctx.Done():
	case <-time.After(backoff):
	}
	return 2 * backoff
}
