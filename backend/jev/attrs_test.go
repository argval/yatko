package jev

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// runProbability owns Yatko's "CPU not stated" policy; these cases pin it.
func TestRunProbabilityPolicy(t *testing.T) {
	named := func(name, os, cpu string) FileAttrs {
		return FileAttrs{Name: name, OS: map[string]float64{os: 1}, CPU: map[string]float64{cpu: 1}}
	}
	attrs := func(os, cpu string) FileAttrs { return named("app", os, cpu) }
	cases := []struct {
		name                    string
		a                       FileAttrs
		platform                picker.Platform
		arch                    picker.Arch
		native, emulated, maybe float64
	}{
		{"windows unstated is x64", attrs(osWindows, cpuNotStated), picker.Windows, picker.AMD64, 1, 0, 0},
		{"windows unstated emulates on arm64", attrs(osWindows, cpuNotStated), picker.Windows, picker.ARM64, 0, 1, 0},
		{"linux unstated never runs on arm64", attrs(osLinux, cpuNotStated), picker.Linux, picker.ARM64, 0, 0, 0},
		{"macOS unstated runs on Apple Silicon", attrs(osMac, cpuNotStated), picker.MacOS, picker.ARM64, 0, 1, 0},
		{"macOS unstated zip only maybe runs on Intel", named("app.zip", osMac, cpuNotStated), picker.MacOS, picker.AMD64, 0, 0, 1},
		{"macOS unstated dmg runs on Intel", named("app.dmg", osMac, cpuNotStated), picker.MacOS, picker.AMD64, 0, 1, 0},
		{"macOS universal is native", attrs(osMac, cpuUniversal), picker.MacOS, picker.AMD64, 1, 0, 0},
		{"rosetta runs x64 on arm64", attrs(osMac, cpuAMD64), picker.MacOS, picker.ARM64, 0, 1, 0},
		{"other OS never runs", attrs(osLinux, cpuAMD64), picker.Windows, picker.AMD64, 0, 0, 0},
	}
	for _, c := range cases {
		n, e, m := runProbability(c.a, c.platform, c.arch)
		if math.Abs(n-c.native) > 1e-9 || math.Abs(e-c.emulated) > 1e-9 || math.Abs(m-c.maybe) > 1e-9 {
			t.Errorf("%s: native=%.2f emulated=%.2f maybe=%.2f, want %.2f %.2f %.2f", c.name, n, e, m, c.native, c.emulated, c.maybe)
		}
	}
}

// Inspected targets override Jev's guess, with the setup-stub CPU left to Jev.
func TestWithObservedTargets(t *testing.T) {
	guess := FileAttrs{Name: "App-x64.zip", OS: map[string]float64{osWindows: 0.9}, CPU: map[string]float64{cpuAMD64: 1}, Main: 0.9}
	got := withObservedTargets(guess, picker.CorrectedTargets(guess.Name, []github.ArtifactTarget{{Platform: "macos", Arch: "amd64"}}))
	if got.OS[osMac] != 1 || got.CPU[cpuAMD64] != 1 || got.Main != 0.9 {
		t.Fatalf("observed macOS zip: %+v", got)
	}
	stub := FileAttrs{Name: "App-Setup-x64.exe", OS: map[string]float64{osWindows: 1}, CPU: map[string]float64{cpuAMD64: 1}}
	got = withObservedTargets(stub, picker.CorrectedTargets(stub.Name, []github.ArtifactTarget{{Platform: "windows", Arch: "386"}}))
	if got.CPU[cpuAMD64] != 1 {
		t.Fatalf("setup stub must keep Jev's CPU, got %+v", got.CPU)
	}
}

// TestJevAttrAccuracy measures Describe file by file against inspected
// targets on the tune split only. It reuses the harvest eval's cached calls.
func TestJevAttrAccuracy(t *testing.T) {
	c, _ := evalClient(t)
	releases, err := picker.LoadHarvest(sharedPath("harvest.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var osN, osOK, cpuN, cpuOK, cpuUnstated int
	for _, rel := range releases {
		if picker.HarvestSplit(rel.Repo) != "tune" {
			continue
		}
		assets := make([]github.Asset, len(rel.Assets))
		byName := map[string]picker.HarvestAsset{}
		for i, a := range rel.Assets {
			assets[i] = github.Asset{Name: a.Name}
			byName[a.Name] = a
		}
		attrs, _, err := c.Describe(context.Background(), rel.Repo, assets)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range attrs {
			wantOS, wantCPU := inspectedLabel(byName[a.Name])
			if wantOS == "" {
				continue
			}
			osN++
			if argmax(a.OS) == wantOS {
				osOK++
			} else if testing.Verbose() {
				t.Logf("os  %s: got %q want %q", a.Name, argmax(a.OS), wantOS)
			}
			if wantCPU == "" {
				continue
			}
			cpuN++
			got := argmax(a.CPU)
			switch {
			case got == wantCPU:
				cpuOK++
			case got == cpuNotStated:
				cpuUnstated++
			case testing.Verbose():
				t.Logf("cpu %s: got %q want %q", a.Name, got, wantCPU)
			}
		}
	}
	t.Logf("attrs tune: os %d/%d (%.1f%%)  cpu %d/%d (%.1f%%), not-stated %d, wrong %d",
		osOK, osN, 100*float64(osOK)/float64(max(osN, 1)), cpuOK, cpuN, 100*float64(cpuOK)/float64(max(cpuN, 1)), cpuUnstated, cpuN-cpuOK-cpuUnstated)
}

// inspectedLabel turns corrected inspected targets (picker.CorrectedTargets)
// into Describe option keys. Empty means "no clean label".
func inspectedLabel(a picker.HarvestAsset) (osKey, cpuKey string) {
	targets := picker.CorrectedTargets(a.Name, a.Targets)
	if len(targets) == 0 {
		return "", ""
	}
	keys := map[string]string{"macos": osMac, "windows": osWindows, "linux": osLinux, "android": osAndroid, "freebsd": osBSD, "openbsd": osBSD, "netbsd": osBSD, "dragonfly": osBSD,
		"solaris": osOther, "illumos": osOther, "sunos": osOther, "aix": osOther, "plan9": osOther, "haiku": osOther}
	var arches []picker.Arch
	for _, t := range targets {
		k := keys[t.Platform]
		if k == "" || (osKey != "" && osKey != k) {
			return "", "" // unknown OS, or several OSes in one file
		}
		osKey = k
		arches = append(arches, picker.Arch(t.Arch))
	}
	slices.Sort(arches)
	arches = slices.Compact(arches)
	switch {
	case len(arches) == 2 && osKey == osMac && slices.Contains(arches, picker.AMD64) && slices.Contains(arches, picker.ARM64):
		return osKey, cpuUniversal
	case len(arches) != 1 || arches[0] == picker.UnknownArch || arches[0] == picker.ArchUnproven:
		return osKey, ""
	}
	for key, arch := range cpuArch {
		if arch == arches[0] {
			return osKey, key
		}
	}
	return osKey, cpuOther
}

func argmax(probs map[string]float64) string {
	best, bestP := "", -1.0
	for k, p := range probs {
		if p > bestP || (p == bestP && k < best) {
			best, bestP = k, p
		}
	}
	return best
}
