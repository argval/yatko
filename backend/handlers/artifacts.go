package handlers

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

func artifactIdentity(a github.Asset) string {
	return fmt.Sprintf("%d:%s:%s:%s:%s:%d", a.ID, a.Name, a.UpdatedAt, a.Digest, a.BrowserDownloadURL, a.Size)
}

// enrichArtifacts uses bounded, cached observations before publishing shared facts.
func (h *RedirectHandler) enrichArtifacts(parent context.Context, assets []github.Asset) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	identities := make([]string, len(assets))
	for i, a := range assets {
		identities[i] = artifactIdentity(a)
	}
	slices.Sort(identities)
	key := fmt.Sprintf("artifact-manifests:v2:%x", sha256.Sum256([]byte(strings.Join(identities, "\n"))))
	metadata, _ := cache.FetchCached(ctx, h.cache, key, func(ctx context.Context, _ string) (map[string]github.ArtifactMetadata, string, bool, error) {
		m, err := h.gh.ReleaseMetadata(ctx, assets)
		if len(m) > 0 {
			err = nil
		} // Keep valid observations when another manifest fails.
		return m, "", false, err
	})
	for i := range assets {
		if m, ok := metadata[assets[i].Name]; ok {
			assets[i].Artifact = &m
		}
	}
	scans := h.scanArtifacts(parent, assets)
	authoritative := make([]bool, len(assets))
	type probe struct{ i, rank int }
	var queue []probe
	for i := range assets {
		if record, ok := scans[assets[i].BrowserDownloadURL]; ok && artifactIdentity(record.Asset) == artifactIdentity(assets[i]) && record.Ready() {
			observed := *record.Metadata
			if authoritativeScanMetadata(observed) {
				if assets[i].Artifact != nil {
					observed = github.MergeObservations(*assets[i].Artifact, observed)
				}
				assets[i].Artifact = &observed
				authoritative[i] = true
				continue
			}
		}
		if rank, ok := inspectionRank(assets[i]); ok {
			queue = append(queue, probe{i, rank})
		}
	}
	// Cached observations are free; only live downloads spend the budget, so
	// repeated requests fill in every uncertain asset a few at a time.
	// ponytail: 4 live probes per request; background-fill if cold pages need more.
	slices.SortStableFunc(queue, func(a, b probe) int { return a.rank - b.rank })
	probes := 0
	for _, p := range queue {
		if ctx.Err() != nil {
			break
		}
		a := assets[p.i]
		k := fmt.Sprintf("artifact-content:v3:%x", sha256.Sum256([]byte(artifactIdentity(a))))
		observed, hit := cache.GetCached[github.ArtifactMetadata](ctx, h.cache, k)
		if !hit {
			if probes == 4 {
				continue
			}
			probes++
			var err error
			observed, err = cache.FetchCached(ctx, h.cache, k, func(ctx context.Context, _ string) (github.ArtifactMetadata, string, bool, error) {
				m, err := h.gh.InspectArtifact(ctx, a)
				return m, "", false, err
			})
			if err != nil && (!errors.Is(err, github.ErrMalformedArtifact) || ctx.Err() != nil) {
				continue // A transport failure says nothing about the file; keep fast facts.
			}
			if err != nil {
				observed = github.ArtifactMetadata{Status: "failed", Role: "unknown"}
			}
		}
		if a.Artifact != nil {
			observed = github.MergeObservations(*a.Artifact, observed)
		}
		assets[p.i].Artifact = &observed
	}
	for i := range assets {
		if record, ok := scans[assets[i].BrowserDownloadURL]; ok && artifactIdentity(record.Asset) == artifactIdentity(assets[i]) && record.Ready() && !authoritative[i] {
			appendScanEvidence(&assets[i], *record.Metadata)
		}
		m := picker.DescribeAsset(assets[i])
		assets[i].Artifact = &m
	}
}

// inspectionRank orders content probes: unknown OS first (the asset cannot be
// offered at all), then .apk (Android vs Alpine), then a missing CPU.
func inspectionRank(a github.Asset) (int, bool) {
	facts := picker.FactsForAsset(a)
	if facts.Role == "source" || facts.Role == "metadata" || facts.Role == "debug" || facts.NonNative {
		return 0, false
	}
	if len(facts.Targets) == 0 || slices.ContainsFunc(facts.Targets, func(t github.ArtifactTarget) bool { return t.Platform == "" }) {
		return 0, true
	}
	if strings.HasSuffix(strings.ToLower(a.Name), ".apk") {
		return 1, true
	}
	if slices.ContainsFunc(facts.Targets, func(t github.ArtifactTarget) bool { return t.Arch == "" }) {
		return 2, true
	}
	return 0, false
}
