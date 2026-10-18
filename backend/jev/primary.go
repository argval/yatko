package jev

import (
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
// once, or waits briefly for a fresh one; ok is false when the caller should
// use the rule picker (timeout, error, breaker open). Decisions are cached in
// the existing Redis cache, so every visitor of the same release and visitor
// type gets the same answer.
type Primary struct {
	Client *Client
	Cache  *cache.Cache
	Wait   time.Duration // 0 means defaultWait

	inFlight  atomic.Int32
	fails     atomic.Int32
	openUntil atomic.Int64 // unix nanos
}

// record is the cached form of a decision: the asset is looked up by name in
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
// when a named asset is no longer present.
func (r record) decision(assets []github.Asset) (picker.AssetDecision, bool) {
	find := func(name string) *github.Asset {
		for i := range assets {
			if assets[i].Name == name {
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

// key identifies a decision: the model, the release's files and their observed
// evidence, and everything about the visitor that can change the answer.
func (p *Primary) key(repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) string {
	h := sha256.New()
	write := func(parts ...string) {
		h.Write([]byte(strings.Join(parts, "\x1f") + "\x1e"))
	}
	write(p.Client.ModelName(), repo, string(platform), string(arch), string(opts.Libc), opts.Prefer)
	if platform == picker.Linux {
		write(picker.ResolveLinuxPackagePrefer(opts.UserAgent)) // deb vs rpm is the only UA effect
	}
	names := make([]string, 0, len(assets))
	byName := map[string]github.Asset{}
	for _, a := range assets {
		names = append(names, a.Name)
		byName[a.Name] = a
	}
	slices.Sort(names)
	for _, n := range names {
		a := byName[n]
		evidence, _ := json.Marshal(a.Artifact)
		write(n, a.State, string(evidence))
	}
	return "jev:decision:v1:" + hex.EncodeToString(h.Sum(nil))
}

func (p *Primary) wait() time.Duration {
	if p.Wait > 0 {
		return p.Wait
	}
	return defaultWait
}

// Decide asks for Jev's decision for one visitor type.
func (p *Primary) Decide(ctx context.Context, repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, bool) {
	key := p.key(repo, assets, platform, arch, opts)
	if r, hit := cache.GetCached[record](ctx, p.Cache, key); hit {
		return r.decision(assets)
	}
	if time.Now().UnixNano() < p.openUntil.Load() || p.inFlight.Load() >= maxInFlight {
		return picker.AssetDecision{}, false
	}
	type result struct {
		r   record
		err error
	}
	done := make(chan result, 1)
	p.inFlight.Add(1)
	go func() {
		defer p.inFlight.Add(-1)
		// Detached from the request: a slow call still warms the cache.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		defer cancel()
		r, err := cache.FetchCachedWithTTL(fctx, p.Cache, key, decisionTTL, func(ctx context.Context, _ string) (record, string, bool, error) {
			d, _, err := p.Client.Decide(ctx, repo, assets, platform, arch, opts)
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
		return res.r.decision(assets)
	case <-timer.C:
	case <-ctx.Done():
	}
	return picker.AssetDecision{}, false
}
