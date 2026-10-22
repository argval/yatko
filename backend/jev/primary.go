package jev

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

const (
	// defaultWait is how long one request waits for a cold Jev decision before
	// the rule picker answers. The call keeps running, so the next request hits cache.
	defaultWait = time.Second
	// fetchTimeout bounds the background Jev call itself.
	fetchTimeout = 15 * time.Second
	// decisionTTL keeps a decision in the shared cache; the cache's own hard TTL (24h) still applies.
	decisionTTL = 24 * time.Hour
	// maxInFlight bounds concurrent background Jev decisions per process.
	maxInFlight = 16
	// breakerFails consecutive failures open the breaker for breakerPause.
	breakerFails = 3
	breakerPause = 30 * time.Second
)

// Primary makes Jev the main asset decider. Decide returns a cached decision at
// once, or waits briefly for a fresh one; ok is false only when the caller
// should use the rule picker (service error, timeout, breaker open, full
// in-flight pool). "No compatible file" and "not enough information" are
// answers, so they return ok. Decisions are cached in the shared Redis cache.
type Primary struct {
	Client *Client
	Cache  *cache.Cache
	Wait   time.Duration // 0 means defaultWait

	inFlight  atomic.Int32
	fails     atomic.Int32
	openUntil atomic.Int64 // unix nanos
}

// record is the cached form of a decision: assets are looked up by name in
// the live release, so a stale record can never invent a download URL.
type record struct {
	Status       string            `json:"status"`
	Name         string            `json:"name,omitempty"`
	Confidence   picker.Confidence `json:"confidence"`
	Reasons      []string          `json:"reasons,omitempty"`
	Alternatives []string          `json:"alternatives,omitempty"`
}

func toRecord(d picker.AssetDecision) record {
	r := record{Status: d.Status, Confidence: d.Confidence, Reasons: d.Reasons}
	if d.Asset != nil {
		r.Name = d.Asset.Name
	}
	for _, a := range d.Alternatives {
		r.Alternatives = append(r.Alternatives, a.Asset.Name)
	}
	return r
}

// decision rebuilds an AssetDecision against the current assets; ok is false
// when the named asset is gone or no longer passes ValidateSelection.
func (r record) decision(assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, bool) {
	find := func(name string) *github.Asset {
		for i := range assets {
			if assets[i].Name == name && ValidateSelection(assets[i], platform, arch, opts) {
				return &assets[i]
			}
		}
		return nil
	}
	d := picker.AssetDecision{Status: r.Status, Confidence: r.Confidence, Reasons: r.Reasons}
	if r.Name != "" {
		if d.Asset = find(r.Name); d.Asset == nil {
			return d, false
		}
		d.Facts = picker.FactsForAsset(*d.Asset)
	}
	for _, n := range r.Alternatives {
		if a := find(n); a != nil {
			d.Alternatives = append(d.Alternatives, picker.RankedAsset{Asset: *a})
		}
	}
	return d, true
}

// assetIdentity is what makes a release file "the same file" for the cache;
// download counts are left out because they change on every download.
type assetIdentity struct {
	ID              int64                    `json:"id"`
	Name            string                   `json:"name"`
	Label           string                   `json:"label"`
	State           string                   `json:"state"`
	Digest          string                   `json:"digest"`
	UpdatedAt       string                   `json:"updated_at"`
	URL             string                   `json:"url"`
	Size            int64                    `json:"size"`
	ArchivePlatform string                   `json:"archive_platform"`
	Artifact        *github.ArtifactMetadata `json:"artifact"`
}

// key hashes everything that can change the answer: prompt and policy
// version, the model, the visitor state Jev sees, the candidates in canonical
// order and the identity and evidence of every release file, including the
// ones code filtered out before the request.
func key(p prepared, model string, assets []github.Asset) string {
	h := sha256.New()
	names := make([]string, len(p.candidates))
	for i, a := range p.candidates {
		names[i] = a.Name
	}
	body, _ := json.Marshal(map[string]any{"model": model, "state": p.state, "candidates": names})
	h.Write([]byte(promptVersion + "\x1e"))
	h.Write(body)
	ids := make([]assetIdentity, len(assets))
	for i, a := range assets {
		ids[i] = assetIdentity{a.ID, a.Name, a.Label, a.State, a.Digest, a.UpdatedAt, a.BrowserDownloadURL, a.Size, a.ArchivePlatform, a.Artifact}
	}
	slices.SortFunc(ids, func(x, y assetIdentity) int { return cmp.Or(strings.Compare(x.Name, y.Name), cmp.Compare(x.ID, y.ID)) })
	idBody, _ := json.Marshal(ids)
	h.Write([]byte("\x1e"))
	h.Write(idBody)
	return "jev:decision:v3:" + hex.EncodeToString(h.Sum(nil))
}

func (p *Primary) wait() time.Duration {
	if p.Wait > 0 {
		return p.Wait
	}
	return defaultWait
}

// acquire reserves an in-flight slot; CAS keeps the bound exact under races.
func (p *Primary) acquire() bool {
	for {
		n := p.inFlight.Load()
		if n >= maxInFlight {
			return false
		}
		if p.inFlight.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// Decide asks for Jev's decision for one visitor context.
func (p *Primary) Decide(ctx context.Context, repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, bool) {
	prep, err := p.Client.prepare(repo, assets, platform, arch, opts)
	if err != nil {
		return picker.AssetDecision{}, false
	}
	if prep.immediate != nil {
		return *prep.immediate, true
	}
	key := key(prep, p.Client.model, assets)
	if r, hit := cache.GetCached[record](ctx, p.Cache, key); hit {
		return r.decision(assets, platform, arch, opts)
	}
	if time.Now().UnixNano() < p.openUntil.Load() || !p.acquire() {
		return picker.AssetDecision{}, false
	}
	type result struct {
		r   record
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer p.inFlight.Add(-1)
		// Detached from the request: a slow call still warms the cache.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		defer cancel()
		r, err := cache.FetchCachedWithTTL(fctx, p.Cache, key, decisionTTL, func(ctx context.Context, _ string) (record, string, bool, error) {
			d, _, err := p.Client.run(ctx, prep, platform, arch, opts)
			if err != nil {
				return record{}, "", false, err
			}
			return toRecord(d), "", false, nil
		})
		if err != nil {
			if p.fails.Add(1) >= breakerFails {
				p.openUntil.Store(time.Now().Add(breakerPause).UnixNano())
				p.fails.Store(0)
			}
		} else {
			p.fails.Store(0)
		}
		done <- result{r, err}
	}()
	timer := time.NewTimer(p.wait())
	defer timer.Stop()
	select {
	case res := <-done:
		if res.err != nil {
			return picker.AssetDecision{}, false
		}
		return res.r.decision(assets, platform, arch, opts)
	case <-timer.C:
	case <-ctx.Done():
	}
	return picker.AssetDecision{}, false
}
