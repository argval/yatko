package jev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// cachingTransport replays identical Jev requests from disk, so rerunning an
// eval after a code-only change is free and returns the exact same answers.
type cachingTransport struct {
	dir          string
	hits, misses atomic.Int64
}

func (c *cachingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	path := filepath.Join(c.dir, hex.EncodeToString(sum[:])+".json")
	if data, err := os.ReadFile(path); err == nil {
		c.hits.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}, Request: req}, nil
	}
	c.misses.Add(1)
	req.Body = io.NopCloser(bytes.NewReader(body))
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil || res.StatusCode != http.StatusOK {
		return res, err
	}
	data, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	res.Body = io.NopCloser(bytes.NewReader(data))
	return res, nil
}

// evalClient returns a Jev client whose responses are cached under the user
// cache dir, or skips the test when the paid eval is not requested.
func evalClient(t *testing.T) (*Client, *cachingTransport) {
	t.Helper()
	c := NewClientFromEnv()
	if os.Getenv("JEV_EVAL") != "1" || c == nil {
		t.Skip("set JEV_EVAL=1 and TYPESAFE_API_KEY to run the paid Jev eval")
	}
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	ct := &cachingTransport{dir: filepath.Join(base, "yatko-jev-eval")}
	if err := os.MkdirAll(ct.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	c.http = &http.Client{Timeout: 30 * time.Second, Transport: ct}
	return c, ct
}

func sharedPath(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "shared", "picker", name)
}

// TestJevHarvest scores Jev on filename-only harvest cases against inspected
// targets. Tune prints per-case misses for error analysis; holdout prints
// totals only, so prompts are never fitted to it. JEV_SPLIT picks one split.
//
//	set -a; source ../.env; set +a; JEV_EVAL=1 go test ./jev -run TestJevHarvest -v -timeout 60m
func TestJevHarvest(t *testing.T) {
	c, ct := evalClient(t)
	releases, err := picker.LoadHarvest(sharedPath("harvest.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	type decideFunc func(context.Context, string, []github.Asset, picker.Platform, picker.Arch, picker.PickOpts) (picker.AssetDecision, Usage, error)
	for _, split := range []string{"tune", "holdout"} {
		if s := os.Getenv("JEV_SPLIT"); s != "" && s != split {
			continue
		}
		var part []picker.HarvestRelease
		for _, r := range releases {
			if picker.HarvestSplit(r.Repo) == split {
				part = append(part, r)
			}
		}
		for _, v := range []struct {
			label  string
			decide decideFunc
		}{{"attrs", c.Decide}, {"fit", c.DecideFit}, {"choice", c.DecideChoice}} {
			var tokens, errs atomic.Int64
			decide := func(repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) picker.AssetDecision {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				d, usage, err := v.decide(ctx, repo, assets, platform, arch, opts)
				tokens.Add(int64(usage.InputTokens))
				if err != nil {
					errs.Add(1)
					return picker.AssetDecision{Confidence: picker.ConfidenceLow, Reasons: []string{err.Error()}}
				}
				return d
			}
			score, lines := picker.ScoreHarvestWith(part, decide, 8)
			t.Logf("jev=%s split=%s releases=%d %s errors=%d input_tokens=%d", v.label, split, len(part), score, errs.Load(), tokens.Load())
			if split == "tune" && testing.Verbose() && os.Getenv("JEV_SHOW") != "" {
				for _, l := range lines {
					if strings.Contains(l, os.Getenv("JEV_SHOW")) {
						t.Log(l)
					}
				}
			}
		}
	}
	t.Logf("cache hits=%d misses=%d (misses are paid calls)", ct.hits.Load(), ct.misses.Load())
}
