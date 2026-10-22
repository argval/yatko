package picker

import (
	"github.com/argval/yatko/github"
	"os"
	"path/filepath"
	"testing"
)

func TestAppImageNeedsLauncherVerification(t *testing.T) {
	for _, metadata := range []*github.ArtifactMetadata{nil, {Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "arm64"}}}} {
		a := github.Asset{Name: "tool-linux-arm64.AppImage", Artifact: metadata}
		d := DecideAsset([]github.Asset{a}, Linux, ARM64, PickOpts{})
		if d.ShouldAutoSelect() || d.Status != "unknown" {
			t.Fatalf("unchecked launcher selected: %+v", d)
		}
	}
	a := github.Asset{Name: "tool-linux-arm64.AppImage", Artifact: &github.ArtifactMetadata{Status: "inspected", LauncherArch: "amd64", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64"}}}}
	for _, arch := range []Arch{ARM64, AMD64} {
		if d := DecideAsset([]github.Asset{a}, Linux, arch, PickOpts{}); d.ShouldAutoSelect() {
			t.Fatalf("mismatched launcher selected for %s: %+v", arch, d)
		}
	}
	a.Artifact.LauncherArch = "arm64"
	a.Artifact.Targets[0].Arch = "arm64"
	if d := DecideAsset([]github.Asset{a}, Linux, ARM64, PickOpts{}); !d.ShouldAutoSelect() {
		t.Fatalf("verified launcher rejected: %+v", d)
	}
}

func TestPartialTargetsDoNotExcludeUnseenCPU(t *testing.T) {
	a := github.Asset{Name: "sdk-windows.zip", Artifact: &github.ArtifactMetadata{Status: "inspected", Role: "application", TargetsPartial: true, Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "arm64"}}}}
	d := DecideAsset([]github.Asset{a}, Windows, AMD64, PickOpts{})
	if d.ShouldAutoSelect() || d.Status != "unknown" {
		t.Fatalf("missing sample treated as incompatible or runnable: %+v", d)
	}
	a.Artifact.Targets = append(a.Artifact.Targets, github.ArtifactTarget{Platform: "windows", Arch: "amd64"})
	if d := DecideAsset([]github.Asset{a}, Windows, AMD64, PickOpts{}); !d.ShouldAutoSelect() {
		t.Fatalf("matching sample rejected: %+v", d)
	}
	m := DescribeAsset(a)
	if !m.TargetsPartial {
		t.Fatal("shared record dropped partial coverage")
	}
	a.Artifact.Targets = []github.ArtifactTarget{{Platform: "windows", Arch: "arm64"}}
	a.Name = "sdk-windows-amd64.zip"
	if f := FactsForAsset(a); len(f.Conflicts) > 0 {
		t.Fatalf("sample absence created filename conflict: %+v", f)
	}
	if d := DecideAsset([]github.Asset{a}, Windows, AMD64, PickOpts{}); d.ShouldAutoSelect() {
		t.Fatal("filename replaced missing CPU proof")
	}
}

func TestEmptyPartialInspectionDoesNotVerifyFilenameTargets(t *testing.T) {
	for _, status := range []string{"inspected", "unsupported", "unknown"} {
		t.Run(status, func(t *testing.T) {
			a := github.Asset{Name: "sdk-windows-amd64.zip", Artifact: &github.ArtifactMetadata{Status: status, TargetsPartial: true}}
			for range 2 {
				if f := FactsForAsset(a); !NeedsVerification(f, Windows, AMD64, LibcAny) || len(f.Targets) != 0 {
					t.Fatalf("filename became observed proof: %+v", f)
				}
				if d := DecideAsset([]github.Asset{a}, Windows, AMD64, PickOpts{}); d.ShouldAutoSelect() || d.Status != "unknown" {
					t.Fatalf("empty partial inspection selected: %+v", d)
				}
				if d := DecideAsset([]github.Asset{a}, MacOS, AMD64, PickOpts{}); d.Status != "incompatible" {
					t.Fatalf("known wrong OS became uncertain: %+v", d)
				}
				m := DescribeAsset(a)
				a.Artifact = &m
			}
		})
	}
}

func TestHarvestPartialCPUIsUnverified(t *testing.T) {
	a := HarvestAsset{Name: "sdk-windows.zip", Status: "inspected", TargetsPartial: true, Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "arm64"}}}
	if f, otherOS := fit(a, Windows, AMD64); f != fitUnproven || otherOS {
		t.Fatalf("missing CPU scored incompatible: %d %v", f, otherOS)
	}
	if f, _ := fit(a, Windows, ARM64); f != 2 {
		t.Fatalf("matching sample lost: %d", f)
	}
}

func TestAppImageMultiCPUPayloadStillNeedsMatchingLauncher(t *testing.T) {
	a := github.Asset{Name: "tool.AppImage", Artifact: &github.ArtifactMetadata{Status: "inspected", LauncherArch: "amd64", Role: "application", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64"}, {Platform: "linux", Arch: "arm64"}}}}
	if d := DecideAsset([]github.Asset{a}, Linux, ARM64, PickOpts{}); d.ShouldAutoSelect() {
		t.Fatalf("foreign launcher selected: %+v", d)
	}
	if d := DecideAsset([]github.Asset{a}, Linux, AMD64, PickOpts{}); !d.ShouldAutoSelect() {
		t.Fatalf("matching launcher rejected: %+v", d)
	}
}

func TestVerificationDoesNotObscureKnownWrongOS(t *testing.T) {
	for _, a := range []github.Asset{
		{Name: "tool.AppImage"},
		{Name: "sdk-windows.zip", Artifact: &github.ArtifactMetadata{Status: "inspected", TargetsPartial: true, Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "arm64"}}}},
	} {
		if d := DecideAsset([]github.Asset{a}, MacOS, AMD64, PickOpts{}); d.Status != "incompatible" {
			t.Fatalf("known wrong OS became unknown: %+v", d)
		}
	}
}

func TestLegacyHarvestSamplesRemainPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harvest.jsonl")
	data := `{"repo":"test/sdk","tag":"v1","name":"legacy-windows.zip","status":"inspected","targets":[{"platform":"windows","arch":"arm64"}]}` + "\n" +
		`{"repo":"test/sdk","tag":"v1","name":"complete-windows.zip","status":"inspected","targets_partial":false,"targets":[{"platform":"windows","arch":"arm64"}]}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	releases, err := LoadHarvest(path)
	if err != nil {
		t.Fatal(err)
	}
	if !releases[0].Assets[0].TargetsPartial || releases[0].Assets[1].TargetsPartial {
		t.Fatalf("legacy migration: %+v", releases)
	}
}
