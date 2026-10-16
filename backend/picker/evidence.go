package picker

import (
	"slices"
	"strings"

	"github.com/argval/yatko/github"
)

// FactsForAsset combines filename clues with observed release metadata.
// Contradictions remain explicit and prevent automatic downloads.
func FactsForAsset(asset github.Asset) ArtifactFacts {
	f := Classify(asset.Name)
	f.Role, f.Inspection = filenameRole(f), "unknown"
	if len(f.Platforms) > 1 && !androidWithLinuxHost(f) && (asset.Artifact == nil || asset.Artifact.Status != "inspected" || len(asset.Artifact.Targets) == 0) {
		f.Conflicts = append(f.Conflicts, "filename names multiple operating systems without corroborating evidence")
	}
	if f.FormatPlatform != "" && f.HasOtherPlatform(f.FormatPlatform) && !androidWithLinuxHost(f) {
		f.Conflicts = append(f.Conflicts, "filename OS conflicts with package format")
	}
	if asset.ArchivePlatform != "" {
		if len(f.Platforms) > 0 && !f.HasPlatform(Platform(asset.ArchivePlatform)) {
			f.Conflicts = append(f.Conflicts, "filename OS conflicts with archive contents")
		} else {
			f.Platforms = []Platform{Platform(asset.ArchivePlatform)}
			f.Inspection = "inspected"
			f.Evidence = appendUnique(f.Evidence, "macOS app bundle in ZIP")
			if !f.Source {
				f.Role = "application"
			}
		}
	}
	f.Targets = filenameTargets(f)
	if m := asset.Artifact; m != nil {
		if m.Status != "" {
			f.Inspection = m.Status
		}
		f.Conflicts = append(f.Conflicts, m.Conflicts...)
		if m.Role != "" && m.Role != "unknown" {
			if (f.Role == "source" || f.Role == "debug" || f.Role == "metadata") && m.Role != f.Role {
				f.Conflicts = append(f.Conflicts, "filename role conflicts with observed artifact role")
			}
			f.Role = m.Role
		}
		if len(m.Targets) > 0 {
			var targets []github.ArtifactTarget
			for _, t := range m.Targets {
				if t.Platform == "" && len(f.Targets) == 1 {
					t.Platform = f.Targets[0].Platform
				}
				if t.Arch == "" && len(f.Arches) == 1 {
					t.Arch = string(f.Arches[0])
				}
				if t.Libc == "" {
					t.Libc = string(f.Libc)
				}
				targets = append(targets, t)
			}
			if !targetsAgree(f.Targets, targets) {
				f.Conflicts = appendUnique(f.Conflicts, "filename targets conflict with observed targets")
			}
			f.Targets = specificTargets(targets)
		}
		for _, e := range m.Evidence {
			if e.Source != "filename" {
				f.Evidence = appendUnique(f.Evidence, e.Source+": "+e.Field+"="+e.Value)
			}
		}
	}
	f.Source = f.Role == "source"
	return f
}

// filenameRole checks sidecar suffixes first: source.tar.gz.sha256 is a
// checksum (metadata), not source code, whatever its base name says.
func filenameRole(f ArtifactFacts) string {
	for _, suffix := range []string{".sha256", ".sha512", ".md5", ".sig", ".asc", ".minisig", ".blockmap", ".yml", ".yaml", ".json"} {
		if strings.HasSuffix(f.Canonical, suffix) {
			return "metadata"
		}
	}
	if f.Source {
		return "source"
	}
	for _, tok := range []string{"symbols", "pdb", "pdbs", "dsym", "dsyms", "dbsym", "debuginfo", "debugsymbols", "debug-symbols"} {
		if hasBoundedKeyword(f.Canonical, tok) {
			return "debug"
		}
	}
	if f.Kind == KindInstaller {
		return "installer"
	}
	if f.Kind == KindPackage || f.Kind == KindExecutable || len(f.Platforms) > 0 {
		return "application"
	}
	return "unknown"
}

func filenameTargets(f ArtifactFacts) []github.ArtifactTarget {
	platforms := slices.Clone(f.Platforms)
	if androidWithLinuxHost(f) {
		platforms = []Platform{Android}
	}
	if len(platforms) == 0 && f.FormatPlatform != "" {
		platforms = []Platform{f.FormatPlatform}
	}
	if len(platforms) == 0 {
		platforms = []Platform{""}
	}
	arches := f.Arches
	if len(arches) == 0 {
		arches = []Arch{UnknownArch}
	}
	var out []github.ArtifactTarget
	for _, p := range platforms {
		for _, a := range arches {
			if p != "" || a != "" {
				out = append(out, github.ArtifactTarget{Platform: string(p), Arch: string(a), Libc: string(f.Libc)})
			}
		}
	}
	return out
}

func targetsAgree(a, b []github.ArtifactTarget) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	covers := func(from, to []github.ArtifactTarget) bool {
		for _, x := range from {
			matched := false
			for _, y := range to {
				if (x.Platform == "" || y.Platform == "" || x.Platform == y.Platform) &&
					(x.Arch == "" || y.Arch == "" || x.Arch == y.Arch) &&
					(x.Libc == "" || y.Libc == "" || x.Libc == y.Libc || x.Libc == "static" || y.Libc == "static") {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		return true
	}
	return covers(a, b) && covers(b, a)
}

// specificTargets removes broad structural facts once a header fills their gaps.
func specificTargets(targets []github.ArtifactTarget) []github.ArtifactTarget {
	var out []github.ArtifactTarget
	for i, t := range targets {
		subsumed := false
		for j, other := range targets {
			if i == j || t == other {
				continue
			}
			if (t.Platform == "" || t.Platform == other.Platform) && (t.Arch == "" || t.Arch == other.Arch) && (t.Libc == "" || t.Libc == other.Libc) {
				subsumed = true
				break
			}
		}
		if !subsumed && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// DescribeAsset is the shared record used by downloads, labels, and filters.
func DescribeAsset(asset github.Asset) github.ArtifactMetadata {
	f := FactsForAsset(asset)
	m := github.ArtifactMetadata{Targets: f.Targets, Role: f.Role, Status: f.Inspection, Conflicts: f.Conflicts, Evidence: []github.ArtifactEvidence{}}
	for _, clue := range Classify(asset.Name).Evidence {
		m.Evidence = append(m.Evidence, github.ArtifactEvidence{Source: "filename", Field: "token", Value: clue})
	}
	if asset.ArchivePlatform != "" {
		m.Evidence = append(m.Evidence, github.ArtifactEvidence{Source: "zip-directory", Field: "platform", Value: asset.ArchivePlatform})
	}
	if asset.Artifact != nil {
		for _, e := range asset.Artifact.Evidence {
			if e.Source != "filename" && !slices.Contains(m.Evidence, e) {
				m.Evidence = append(m.Evidence, e)
			}
		}
	}
	if m.Targets == nil {
		m.Targets = []github.ArtifactTarget{}
	}
	return m
}
