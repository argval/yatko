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
	attrs := func(os, cpu string) FileAttrs {
		return FileAttrs{OS: map[string]float64{os: 1}, CPU: map[string]float64{cpu: 1}}
	}
	cases := []struct {
		name             string
		a                FileAttrs
		platform         picker.Platform
		arch             picker.Arch
		native, emulated float64
	}{
		{"windows unstated is x64", attrs(osWindows, cpuNotStated), picker.Windows, picker.AMD64, 1, 0},
		{"windows unstated emulates on arm64", attrs(osWindows, cpuNotStated), picker.Windows, picker.ARM64, 0, 1},
		{"linux unstated never runs on arm64", attrs(osLinux, cpuNotStated), picker.Linux, picker.ARM64, 0, 0},
		{"macOS unstated is unproven", attrs(osMac, cpuNotStated), picker.MacOS, picker.ARM64, 0, 1},
		{"macOS universal is native", attrs(osMac, cpuUniversal), picker.MacOS, picker.AMD64, 1, 0},
		{"rosetta runs x64 on arm64", attrs(osMac, cpuAMD64), picker.MacOS, picker.ARM64, 0, 1},
		{"other OS never runs", attrs(osLinux, cpuAMD64), picker.Windows, picker.AMD64, 0, 0},
	}
	for _, c := range cases {
		n, e := runProbability(c.a, c.platform, c.arch)
		if math.Abs(n-c.native) > 1e-9 || math.Abs(e-c.emulated) > 1e-9 {
			t.Errorf("%s: native=%.2f emulated=%.2f, want %.2f %.2f", c.name, n, e, c.native, c.emulated)
		}
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

// inspectedLabel turns inspected targets into the Describe option keys, using
// the same corrections as picker's harvest scorer. Empty means "no clean label".
func inspectedLabel(a picker.HarvestAsset) (osKey, cpuKey string) {
	if len(a.Targets) == 0 {
		return "", ""
	}
	osName := ""
	var arches []picker.Arch
	for _, t := range a.Targets {
		if t.Platform != "" {
			if osName != "" && osName != t.Platform {
				return "", "" // several OSes in one file: no single answer
			}
			osName = t.Platform
		}
		arches = append(arches, picker.Arch(t.Arch))
	}
	if osName == "" {
		osName = string(picker.Linux)
	}
	keys := map[string]string{"macos": osMac, "windows": osWindows, "linux": osLinux, "android": osAndroid, "freebsd": osBSD, "openbsd": osBSD, "netbsd": osBSD}
	osKey = keys[osName]
	if osKey == "" {
		return "", ""
	}
	slices.Sort(arches)
	arches = slices.Compact(arches)
	switch {
	case len(arches) == 2 && osKey == osMac && slices.Contains(arches, picker.AMD64) && slices.Contains(arches, picker.ARM64):
		return osKey, cpuUniversal
	case len(arches) != 1 || arches[0] == picker.UnknownArch:
		return osKey, ""
	}
	// Windows setup stubs are often 32-bit x86 whatever they install.
	nameArches := picker.FactsForAsset(github.Asset{Name: a.Name}).Arches
	if osKey == osWindows && arches[0] == picker.X86 && len(nameArches) > 0 && !slices.Contains(nameArches, picker.X86) {
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
