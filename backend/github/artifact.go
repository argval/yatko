package github

import "slices"

// ArtifactTarget keeps OS, CPU, and runtime requirements together for one build.
type ArtifactTarget struct {
	Platform string `json:"platform"`
	Arch     string `json:"arch,omitempty"`
	Libc     string `json:"libc,omitempty"`
}

// ArtifactEvidence identifies the source of a fact without inventing asset URLs.
type ArtifactEvidence struct {
	Source string `json:"source"`
	Field  string `json:"field"`
	Value  string `json:"value"`
}

// ArtifactMetadata holds observed facts; absent targets remain unknown.
type ArtifactMetadata struct {
	Targets        []ArtifactTarget   `json:"targets"`
	Role           string             `json:"role"`
	Evidence       []ArtifactEvidence `json:"evidence"`
	Status         string             `json:"status"`
	Conflicts      []string           `json:"conflicts,omitempty"`
	TargetsPartial bool               `json:"targets_partial,omitempty"` // Other targets may exist outside the inspected sample.
	LauncherArch   string             `json:"launcher_arch,omitempty"`   // AppImage ELF runtime, independent of its payload.
}

// MergeObservations combines publisher facts (a, from manifests) with content
// inspection (b): it intersects corroborating targets and preserves contradictions.
func MergeObservations(a, b ArtifactMetadata) ArtifactMetadata {
	out := a
	out.Evidence = slices.Clone(a.Evidence)
	out.Conflicts = slices.Clone(a.Conflicts)
	for _, e := range b.Evidence {
		if !slices.Contains(out.Evidence, e) {
			out.Evidence = append(out.Evidence, e)
		}
	}
	out.Conflicts = append(out.Conflicts, b.Conflicts...)
	if b.LauncherArch != "" {
		out.LauncherArch = b.LauncherArch
	}
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
		out.TargetsPartial = b.TargetsPartial
		if len(b.Targets) == 0 {
			out.TargetsPartial = a.TargetsPartial || b.TargetsPartial
		}
	} else if len(b.Targets) > 0 {
		// A sample cannot narrow a complete list or contradict an absent CPU.
		if a.TargetsPartial || b.TargetsPartial {
			switch {
			case !b.TargetsPartial:
				out.Targets, out.TargetsPartial = slices.Clone(b.Targets), false
			case a.TargetsPartial:
				out.Targets = slices.Clone(a.Targets)
				for _, t := range b.Targets {
					addTarget(&out, t)
				}
			}
		} else {
			var combined []ArtifactTarget
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
	}
	if b.Status == "failed" || b.Status == "inspected" || out.Status == "" || out.Status == "unknown" {
		out.Status = b.Status
	}
	return out
}
