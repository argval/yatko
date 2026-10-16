package handlers

import (
	"context"
	"crypto/sha256"
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
	key := fmt.Sprintf("artifact-manifests:v1:%x", sha256.Sum256([]byte(strings.Join(identities, "\n"))))
	metadata, _ := cache.FetchCached(ctx, h.cache, key, func(ctx context.Context, _ string) (map[string]github.ArtifactMetadata, string, bool, error) {
		m, err := h.gh.ReleaseMetadata(ctx, assets)
		if len(m) > 0 {
			err = nil
		} // Keep valid observations when another manifest fails.
		return m, "", false, err
	})
	probes := 0
	for i, a := range assets {
		if m, ok := metadata[a.Name]; ok {
			a.Artifact = &m
		}
		facts := picker.FactsForAsset(a)
		needsInspection := len(facts.Targets) == 0
		for _, target := range facts.Targets {
			needsInspection = needsInspection || target.Platform == "" || target.Arch == ""
		}
		if needsInspection && facts.Role != "source" && facts.Role != "metadata" && facts.Role != "debug" && !facts.NonNative && probes < 4 && ctx.Err() == nil {
			probes++
			k := fmt.Sprintf("artifact-content:v1:%x", sha256.Sum256([]byte(artifactIdentity(a))))
			observed, err := cache.FetchCached(ctx, h.cache, k, func(ctx context.Context, _ string) (github.ArtifactMetadata, string, bool, error) {
				m, err := h.gh.InspectArtifact(ctx, a)
				return m, "", false, err
			})
			if err != nil {
				observed = github.ArtifactMetadata{Status: "failed", Role: "unknown"}
			}
			if a.Artifact != nil {
				observed = mergeObservations(*a.Artifact, observed)
			}
			a.Artifact = &observed
		}
		m := picker.DescribeAsset(a)
		a.Artifact = &m
		assets[i] = a
	}
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
	if b.Status == "inspected" || out.Status == "" || out.Status == "unknown" {
		out.Status = b.Status
	}
	return out
}
