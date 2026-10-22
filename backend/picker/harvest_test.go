package picker

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/argval/yatko/github"
)

// TestHarvestRules is the free baseline: the rule picker on filename-only
// harvest cases, scored against inspected targets. Holdout prints totals only.
func TestHarvestRules(t *testing.T) {
	releases, err := LoadHarvest(harvestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, split := range []string{"tune", "holdout"} {
		var part []HarvestRelease
		for _, r := range releases {
			if HarvestSplit(r.Repo) == split {
				part = append(part, r)
			}
		}
		s, lines := ScoreHarvestWith(part, ruleDecider, 1, false)
		t.Logf("rules split=%s releases=%d %s", split, len(part), s)
		warm, _ := ScoreHarvestWith(part, ruleDecider, 1, true)
		t.Logf("rules warm split=%s releases=%d %s", split, len(part), warm)
		if split == "tune" && testing.Verbose() {
			show := os.Getenv("HARVEST_SHOW") // extra substring to print, e.g. "abstained"
			for _, l := range lines {
				if strings.Contains(l, "WRONG") || (show != "" && strings.Contains(l, show)) {
					t.Log(l)
				}
			}
		}
	}
}

func TestHarvestManifestConflictKeepsContentLabels(t *testing.T) {
	a := HarvestAsset{Name: "app.zip", Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "macos", Arch: "arm64"}},
		Manifest: &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "amd64"}}}}
	obs, ok := a.Observed()
	if !ok || len(obs.Conflicts) == 0 || !slices.Equal(a.labelTargets(), a.Targets) {
		t.Fatalf("conflict lost: observed=%+v labels=%+v", obs, a.labelTargets())
	}
	if fit, _ := fit(a, MacOS, ARM64); fit != 2 {
		t.Fatalf("proven macOS content no longer fits: %d", fit)
	}
	a.Status = "failed"
	if len(a.labelTargets()) != 0 {
		t.Fatal("failed inspection supplied trusted labels")
	}
}

func TestUnknownInstallerPayloadDoesNotProveFilenameCPU(t *testing.T) {
	a := HarvestAsset{Name: "app-arm64-setup.exe", Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "windows"}}}
	if fit, _ := fit(a, Windows, ARM64); fit != fitUnproven {
		t.Fatalf("filename counted as inspected CPU: %d", fit)
	}
}

func TestLoadHarvestManifestSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "harvest.jsonl")
	if err := os.WriteFile(path, []byte(`{"repo":"acme/app","tag":"v1","name":"Setup.exe","status":"inspected","targets":[{"platform":"windows"}]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(dir, "harvest_manifests.json")
	if err := os.WriteFile(snapshot, []byte(`{"acme/app@v1":{"Setup.exe":{"status":"inspected","role":"installer","targets":[{"platform":"windows","arch":"arm64"}]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	releases, err := LoadHarvest(path)
	if err != nil || len(releases) != 1 || len(releases[0].Assets) != 1 {
		t.Fatalf("load snapshot: %+v %v", releases, err)
	}
	a := releases[0].Assets[0]
	if fit, _ := fit(a, Windows, ARM64); fit != 2 {
		t.Fatalf("publisher CPU did not fill unknown content CPU: %+v", a)
	}
	if err := os.WriteFile(snapshot, []byte("invalid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHarvest(path); err == nil {
		t.Fatal("ignored invalid manifest snapshot")
	}
}

// A proven full-file result replaces the setup-stub label; an unknown one
// keeps the content facts, as /dl does.
func TestHarvestDeepEvidencePrecedence(t *testing.T) {
	stub := []github.ArtifactTarget{{Platform: "windows", Arch: "386"}}
	a := HarvestAsset{Name: "App-Setup.exe", Status: "inspected", Targets: stub,
		Deep: &github.ArtifactMetadata{Status: "inspected", Role: "application", Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "amd64"}, {Platform: "windows", Arch: "arm64"}}}}
	if fit, _ := fit(a, Windows, ARM64); fit != 2 {
		t.Fatalf("proven deep arm64 payload does not fit arm64: %d", fit)
	}
	a.Deep = &github.ArtifactMetadata{Status: "unknown", Role: "unknown"}
	if !slices.Equal(a.labelTargets(), stub) {
		t.Fatalf("unknown deep result erased content facts: %+v", a.labelTargets())
	}
}

// The setup-stub rule is for shallow reads; a proven 32-bit payload stays 32-bit.
func TestCorrectedTargetsTrustsPayloadCPU(t *testing.T) {
	x86 := []github.ArtifactTarget{{Platform: "windows", Arch: "386"}}
	if got := CorrectedTargets("cmder.zip", github.ArtifactMetadata{Targets: x86}, false); got[0].Arch != string(ArchUnproven) {
		t.Fatalf("shallow x86 label should be unproven, got %+v", got)
	}
	if got := CorrectedTargets("cmder.zip", github.ArtifactMetadata{Targets: x86}, true); got[0].Arch != string(X86) {
		t.Fatalf("payload x86 label should stay 386, got %+v", got)
	}
}

func TestCorrectedTargetsPreservesUnknownOS(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"tool", ""}, {"tool-1.0.tar.gz", ""}, {"tool-linux-amd64.tar.gz", "linux"}, {"tool-arm64.dmg", "macos"},
	} {
		got := CorrectedTargets(tc.name, github.ArtifactMetadata{Targets: []github.ArtifactTarget{{Arch: "amd64"}}}, false)
		if got[0].Platform != tc.want {
			t.Errorf("%s: platform=%q want %q", tc.name, got[0].Platform, tc.want)
		}
	}
}

func TestHarvestFitUnknownAndWrongOS(t *testing.T) {
	for _, tc := range []struct {
		name    string
		target  github.ArtifactTarget
		partial bool
		wantFit int
		wantOS  bool
	}{
		{"unknown OS", github.ArtifactTarget{Arch: "amd64"}, false, fitUnproven, false},
		{"unknown OS and CPU", github.ArtifactTarget{}, false, fitUnproven, false},
		{"partial unknown OS", github.ArtifactTarget{Arch: "amd64"}, true, fitUnproven, false},
		{"wrong OS", github.ArtifactTarget{Platform: "linux", Arch: "amd64"}, false, 0, true},
		{"partial wrong OS", github.ArtifactTarget{Platform: "linux", Arch: "amd64"}, true, 0, true},
		{"partial missing CPU", github.ArtifactTarget{Platform: "macos", Arch: "arm64"}, true, fitUnproven, false},
		{"matching target", github.ArtifactTarget{Platform: "macos", Arch: "amd64"}, false, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := HarvestAsset{Name: "tool", Status: "inspected", Targets: []github.ArtifactTarget{tc.target}, TargetsPartial: tc.partial}
			if tc.partial && tc.wantOS {
				a.Name = "tool-linux-amd64.zip"
			}
			if got, otherOS := fit(a, MacOS, AMD64); got != tc.wantFit || otherOS != tc.wantOS {
				t.Fatalf("fit=%d otherOS=%v, want %d %v", got, otherOS, tc.wantFit, tc.wantOS)
			}
		})
	}
}

func TestHarvestScoreUnknownAndWrongOS(t *testing.T) {
	for _, partial := range []bool{false, true} {
		for _, targetOS := range []string{"", "linux"} {
			for _, runnable := range []bool{false, true} {
				t.Run(fmt.Sprintf("partial=%v/os=%q/runnable=%v", partial, targetOS, runnable), func(t *testing.T) {
					picked := HarvestAsset{Name: "tool", Status: "inspected", Targets: []github.ArtifactTarget{{Platform: targetOS, Arch: "amd64"}}, TargetsPartial: partial}
					if targetOS != "" {
						picked.Name = "tool-linux-amd64.zip"
					}
					rel := HarvestRelease{Repo: "test/app", Assets: []HarvestAsset{picked}}
					if runnable {
						rel.Assets = append(rel.Assets, HarvestAsset{Name: "native", Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "macos", Arch: "amd64"}}})
					}
					decide := func(_ string, assets []github.Asset, _ Platform, _ Arch, _ PickOpts) AssetDecision {
						return AssetDecision{Asset: &assets[0], Confidence: ConfidenceHigh}
					}
					s, _ := scoreHarvestCase(rel, MacOS, AMD64, decide, false)
					wantWrongOS, wantNoRunWrong, wantUnverified := 0, 0, 0
					if targetOS != "" {
						if runnable {
							wantWrongOS = 1
						} else {
							wantNoRunWrong = 1
						}
					} else if runnable {
						wantUnverified = 1
					}
					if s.WrongPlatform != wantWrongOS || s.NoRunWrong != wantNoRunWrong || s.Unverified != wantUnverified || s.WrongArch != 0 {
						t.Fatalf("score=%s, want wrong_os=%d norun_wrong=%d unverified=%d", s, wantWrongOS, wantNoRunWrong, wantUnverified)
					}
				})
			}
		}
	}
}

func TestHarvestPartialSampleDoesNotExcludeUnseenOS(t *testing.T) {
	a := HarvestAsset{Name: "cross-platform.zip", Status: "inspected", TargetsPartial: true,
		Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "amd64"}}}
	if got, otherOS := fit(a, MacOS, AMD64); got != fitUnproven || otherOS {
		t.Fatalf("one OS sample became an exhaustive list: fit=%d otherOS=%v", got, otherOS)
	}
}

func TestHarvestPartialCPUDoesNotComeFromFilename(t *testing.T) {
	a := HarvestAsset{Name: "sdk-windows-amd64.zip", Status: "inspected", TargetsPartial: true,
		Targets: []github.ArtifactTarget{{Platform: "windows"}}}
	if got, otherOS := fit(a, Windows, AMD64); got != fitUnproven || otherOS {
		t.Fatalf("filename verified unsampled CPU: fit=%d otherOS=%v", got, otherOS)
	}
}

func TestHarvestOpaqueTauriTargetIsUnverified(t *testing.T) {
	a := HarvestAsset{Name: "tool", Status: "inspected",
		Targets: []github.ArtifactTarget{{Platform: "tauri-target:macos-aarch64"}}}
	if got, otherOS := fit(a, MacOS, ARM64); got != fitUnproven || otherOS {
		t.Fatalf("opaque target became a proven wrong OS: fit=%d otherOS=%v", got, otherOS)
	}
}

// harvestPath skips the test when the harvest snapshot is absent. The data is
// kept out of the repo; restore shared/picker/harvest*.json* from commit
// b7a5b95 to rerun the evaluation.
func harvestPath(t *testing.T) string {
	t.Helper()
	path := sharedPickerPath(t, "harvest.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("harvest snapshot not in the repo; restore shared/picker/harvest*.json* from commit b7a5b95")
	}
	return path
}
