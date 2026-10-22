package jev

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	replayOnly   bool
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
	if c.replayOnly {
		// A plain status, not a transport error, so post does not retry it.
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Request: req,
			Body: io.NopCloser(strings.NewReader("replay cache miss and no TYPESAFE_API_KEY"))}, nil
	}
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
// cache dir, or skips the test when the eval is not requested. Without
// TYPESAFE_API_KEY it replays only: a cache miss fails instead of paying.
func evalClient(t *testing.T) (*Client, *cachingTransport) {
	t.Helper()
	if os.Getenv("JEV_EVAL") != "1" {
		t.Skip("set JEV_EVAL=1 (and TYPESAFE_API_KEY for uncached calls) to run the Jev eval")
	}
	c := NewClientFromEnv()
	replayOnly := c == nil
	if replayOnly {
		model := cmp.Or(os.Getenv("JEV_MODEL"), Model)
		c = &Client{endpoint: defaultEndpoint, model: model}
	}
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	ct := &cachingTransport{dir: filepath.Join(base, "yatko-jev-eval"), replayOnly: replayOnly}
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
	releases, err := picker.LoadHarvest(harvestPath(t))
	if err != nil {
		t.Fatal(err)
	}
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
		// Cold = filenames only; warm = with inspection results.
		for _, warm := range []bool{false, true} {
			mode := map[bool]string{false: "cold", true: "warm"}[warm]
			var tokens, errs atomic.Int64
			decide := func(repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) picker.AssetDecision {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				d, usage, err := c.Decide(ctx, repo, assets, platform, arch, opts)
				tokens.Add(int64(usage.InputTokens))
				if err != nil {
					errs.Add(1)
					return picker.AssetDecision{Confidence: picker.ConfidenceLow, Reasons: []string{err.Error()}}
				}
				return d
			}
			score, lines := picker.ScoreHarvestWith(part, decide, 8, warm)
			t.Logf("jev=direct %s model=%s split=%s releases=%d %s errors=%d input_tokens=%d", mode, c.ModelName(), split, len(part), score, errs.Load(), tokens.Load())
			if err := harvestSafetyError(score, errs.Load()); err != nil {
				t.Errorf("%s split=%s: %v", mode, split, err)
			}
			if split == "tune" && testing.Verbose() && os.Getenv("JEV_SHOW") != "" {
				for _, l := range lines {
					if strings.Contains(l, os.Getenv("JEV_SHOW")) {
						t.Log(mode + " " + l)
					}
				}
			}
		}
	}
	t.Logf("cache hits=%d misses=%d replay_only=%v", ct.hits.Load(), ct.misses.Load(), ct.replayOnly)
}

func harvestSafetyError(s picker.HarvestScore, errors int64) error {
	if s.WrongPlatform > 0 || s.WrongArch > 0 || s.NoRunWrong > 0 || errors > 0 {
		return fmt.Errorf("unsafe: wrong_os=%d wrong_cpu=%d norun_wrong=%d errors=%d", s.WrongPlatform, s.WrongArch, s.NoRunWrong, errors)
	}
	return nil
}

func TestHarvestSafetyError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		score  picker.HarvestScore
		errors int64
		unsafe bool
	}{
		{"safe", picker.HarvestScore{}, 0, false},
		{"unscored uncertainty", picker.HarvestScore{Unverified: 1, FalseAbstain: 1, EmulatedNative: 1}, 0, false},
		{"wrong OS", picker.HarvestScore{WrongPlatform: 1}, 0, true},
		{"wrong CPU", picker.HarvestScore{WrongArch: 1}, 0, true},
		{"wrong when nothing runs", picker.HarvestScore{NoRunWrong: 1}, 0, true},
		{"service error or missing replay", picker.HarvestScore{}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := harvestSafetyError(tc.score, tc.errors); (err != nil) != tc.unsafe {
				t.Fatalf("error=%v, want unsafe=%v", err, tc.unsafe)
			}
		})
	}
}

// harvestPath skips the test when the harvest snapshot is absent. The data is
// kept out of the repo; restore shared/picker/harvest*.json* from commit
// b7a5b95 to rerun the evaluation.
func harvestPath(t *testing.T) string {
	t.Helper()
	path := sharedPath("harvest.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("harvest snapshot not in the repo; restore shared/picker/harvest*.json* from commit b7a5b95")
	}
	return path
}
