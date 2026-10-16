package picker

import (
	"testing"

	"github.com/argval/yatko/github"
)

func TestObservedTargetsResolveOpaqueArchive(t *testing.T) {
	a := github.Asset{Name: "App-x64.zip", Artifact: &github.ArtifactMetadata{
		Targets: []github.ArtifactTarget{{Platform: "macos", Arch: "amd64"}}, Role: "application", Status: "inspected",
		Evidence: []github.ArtifactEvidence{{Source: "macho", Field: "cpu", Value: "amd64"}},
	}}
	for _, tc := range []struct {
		platform Platform
		arch     Arch
		want     bool
	}{{MacOS, AMD64, true}, {Windows, AMD64, false}, {Linux, AMD64, false},
		{MacOS, ARM64, true}} { // Intel-only Mac app runs on Apple Silicon via Rosetta
		if d := DecideAsset([]github.Asset{a}, tc.platform, tc.arch, PickOpts{}); d.ShouldAutoSelect() != tc.want {
			t.Fatalf("%s/%s: %+v", tc.platform, tc.arch, d)
		}
	}
	m := DescribeAsset(a)
	if len(m.Targets) != 1 || m.Targets[0].Platform != "macos" || len(m.Evidence) < 2 {
		t.Fatalf("missing shared evidence: %+v", m)
	}
}

func TestTargetTuplesDoNotInventCrossPlatformArchitectures(t *testing.T) {
	a := github.Asset{Name: "release.zip", Artifact: &github.ArtifactMetadata{Targets: []github.ArtifactTarget{
		{Platform: "windows", Arch: "386"}, {Platform: "macos", Arch: "arm64"},
	}, Role: "application", Status: "inspected"}}
	for _, tc := range []struct {
		platform Platform
		arch     Arch
		want     bool
	}{{Windows, X86, true}, {MacOS, ARM64, true}, {MacOS, AMD64, false}, {MacOS, X86, false},
		{Windows, ARM64, true}} { // windows/386 runs under ARM64 Windows' x86 emulation, not via macos/arm64
		if d := DecideAsset([]github.Asset{a}, tc.platform, tc.arch, PickOpts{}); d.ShouldAutoSelect() != tc.want {
			t.Fatalf("%s/%s: %+v", tc.platform, tc.arch, d)
		}
	}
}

func TestContradictionsAndInspectionFailuresStayUnresolved(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata github.ArtifactMetadata
		status   string
	}{
		{"App-windows-x64.zip", github.ArtifactMetadata{Targets: []github.ArtifactTarget{{Platform: "macos", Arch: "amd64"}}, Status: "inspected", Role: "application"}, "ambiguous"},
		{"App-linux-arm64.zip", github.ArtifactMetadata{Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64"}}, Status: "inspected", Role: "application"}, "ambiguous"},
		{"App-linux.zip", github.ArtifactMetadata{Status: "failed", Role: "unknown"}, "unknown"},
		{"App-linux-amd64.zip", github.ArtifactMetadata{Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64"}, {Platform: "windows", Arch: "arm64"}}, Role: "application", Status: "inspected"}, "ambiguous"},
	} {
		d := DecideAsset([]github.Asset{{Name: tc.name, Artifact: &tc.metadata}}, Linux, AMD64, PickOpts{})
		if d.ShouldAutoSelect() || d.Status != tc.status {
			t.Fatalf("%s: %+v", tc.name, d)
		}
	}
}

func TestObservedCPURefinesStructuralTarget(t *testing.T) {
	a := github.Asset{Name: "App.zip", Artifact: &github.ArtifactMetadata{Targets: []github.ArtifactTarget{{Platform: "macos"}, {Platform: "macos", Arch: "amd64"}}, Role: "application", Status: "inspected"}}
	// Structural OS evidence must not erase a known, usable CPU target.
	other := github.Asset{Name: "Other.dmg"}
	if d := DecideAsset([]github.Asset{a, other}, MacOS, ARM64, PickOpts{}); !d.ShouldAutoSelect() || d.Asset.Name != "App.zip" {
		t.Fatalf("structural target overrode known CPU: %+v", d)
	}
	if d := DecideAsset([]github.Asset{a}, MacOS, AMD64, PickOpts{}); !d.ShouldAutoSelect() {
		t.Fatalf("known CPU not usable: %+v", d)
	}
}

func TestNonApplicationRolesNeverAutoSelect(t *testing.T) {
	for _, role := range []string{"source", "debug", "metadata"} {
		a := github.Asset{Name: "tool-linux-amd64.zip", Artifact: &github.ArtifactMetadata{Role: role, Status: "inspected"}}
		if d := DecideAsset([]github.Asset{a}, Linux, AMD64, PickOpts{}); d.ShouldAutoSelect() {
			t.Fatalf("selected %s: %+v", role, d)
		}
	}
	for _, name := range []string{"tool-linux-amd64-symbols.zip", "tool-linux-amd64.sha256"} {
		a := github.Asset{Name: name, Artifact: &github.ArtifactMetadata{Role: "application", Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64"}}}}
		if d := DecideAsset([]github.Asset{a}, Linux, AMD64, PickOpts{}); d.ShouldAutoSelect() {
			t.Fatalf("role contradiction selected: %+v", d)
		}
	}
	if d := DecideAsset([]github.Asset{{Name: "tool-windows-x64-pdbs.zip"}}, Windows, AMD64, PickOpts{}); d.ShouldAutoSelect() {
		t.Fatal("selected debug symbols")
	}
}

func TestInspectedWithoutTargetsDoesNotResolveAmbiguousFilename(t *testing.T) {
	a := github.Asset{Name: "tool-windows-linux-x64.zip", Artifact: &github.ArtifactMetadata{Status: "inspected", Role: "unknown"}}
	if d := DecideAsset([]github.Asset{a}, Windows, AMD64, PickOpts{}); d.ShouldAutoSelect() {
		t.Fatalf("inspection status is not target evidence: %+v", d)
	}
}

func TestExplicitUnknownArchNeverRedetectsVisitor(t *testing.T) {
	if got := ResolveArch("unknown", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"); got != UnknownArch {
		t.Fatalf("explicit unknown arch became %q", got)
	}
}

// astral-sh/uv 0.13.1: cargo-dist marks source.tar.gz.sha256 as a checksum.
// The "source" base name must not turn that into a role conflict.
func TestChecksumOfSourceIsMetadataNotConflict(t *testing.T) {
	a := github.Asset{Name: "source.tar.gz.sha256", Artifact: &github.ArtifactMetadata{Role: "metadata", Status: "inspected"}}
	if f := FactsForAsset(a); f.Role != "metadata" || len(f.Conflicts) > 0 {
		t.Fatalf("%+v", f)
	}
	if f := FactsForAsset(github.Asset{Name: "source.tar.gz"}); f.Role != "source" {
		t.Fatalf("source archive lost its role: %+v", f)
	}
}
