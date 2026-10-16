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
	type probe struct{ i, rank int }
	var queue []probe
	for i := range assets {
		if m, ok := metadata[assets[i].Name]; ok {
			assets[i].Artifact = &m
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
		k := fmt.Sprintf("artifact-content:v2:%x", sha256.Sum256([]byte(artifactIdentity(a))))
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
			if err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || ctx.Err() != nil) {
				continue // A timeout says nothing about the file; keep filename/manifest facts.
			}
			if err != nil {
				observed = github.ArtifactMetadata{Status: "failed", Role: "unknown"}
			}
		}
		if a.Artifact != nil {
			observed = mergeObservations(*a.Artifact, observed)
		}
		assets[p.i].Artifact = &observed
	}
	for i := range assets {
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

// mergeObservations intersects corroborating targets and preserves contradictions.
func mergeObservations(a, b github.ArtifactMetadata) github.ArtifactMetadata {
	out := a
	out.Evidence = slices.Clone(a.Evidence)
	out.Conflicts = slices.Clone(a.Conflicts)
	for _, e := range b.Evidence {
		if !slices.Contains(out.Evidence, e) {
			out.Evidence = append(out.Evidence, e)
		}
	}
	out.Conflicts = append(out.Conflicts, b.Conflicts...)
	if out.Role == "" || out.Role == "unknown" {
		out.Role = b.Role
	} else if b.Role != "" && b.Role != "unknown" && out.Role != b.Role {
		// Installer/application describe the same runnable payload at different layers.
		if !((out.Role == "installer" && b.Role == "application") || (out.Role == "application" && b.Role == "installer")) {
			out.Conflicts = append(out.Conflicts, "publisher role conflicts with file contents")
		}
	}
	if len(out.Targets) == 0 {
		out.Targets = b.Targets
	} else if len(b.Targets) > 0 {
		var combined []github.ArtifactTarget
		for _, x := range out.Targets {
			for _, y := range b.Targets {
				if x.Platform != "" && y.Platform != "" && x.Platform != y.Platform {
					continue
				}
				if x.Arch != "" && y.Arch != "" && x.Arch != y.Arch {
					continue
				}
				if x.Libc != "" && y.Libc != "" && x.Libc != y.Libc && x.Libc != "static" && y.Libc != "static" {
					continue
				}
				merged := x
				if merged.Platform == "" {
					merged.Platform = y.Platform
				}
				if merged.Arch == "" {
					merged.Arch = y.Arch
				}
				if merged.Libc == "" {
					merged.Libc = y.Libc
				}
				if !slices.Contains(combined, merged) {
					combined = append(combined, merged)
				}
			}
		}
		if len(combined) == 0 {
			out.Conflicts = append(out.Conflicts, "publisher targets conflict with file contents")
		} else {
			out.Targets = combined
		}
	}
	if b.Status == "failed" || b.Status == "inspected" || out.Status == "" || out.Status == "unknown" {
		out.Status = b.Status
	}
	return out
}
