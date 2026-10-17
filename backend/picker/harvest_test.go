package picker

import (
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
	releases, err := LoadHarvest(sharedPickerPath(t, "harvest.jsonl"))
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
