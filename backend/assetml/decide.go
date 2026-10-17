package assetml

import (
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// nativeArches lists the model's arch classes that run natively on a visitor
// CPU. "universal" is a macOS build that contains both Intel and Apple Silicon.
var nativeArches = map[picker.Arch][]string{
	picker.AMD64:       {"amd64", "universal"},
	picker.ARM64:       {"arm64", "universal"},
	picker.ARM:         {"arm"},
	picker.X86:         {"386"},
	picker.UnknownArch: {"amd64", "universal"}, // most computers; matches the rule picker's default
}

// emulatedArches run through emulation: Rosetta on arm64 Macs, Windows on
// ARM's x64/x86 emulation, and 32-bit x86 on 64-bit Windows.
func emulatedArches(platform picker.Platform, arch picker.Arch) []string {
	switch {
	case platform == picker.MacOS && arch == picker.ARM64:
		return []string{"amd64"}
	case platform == picker.Windows && arch == picker.ARM64:
		return []string{"amd64", "386"}
	case platform == picker.Windows && arch == picker.AMD64:
		return []string{"386"}
	}
	return nil
}

// Decide picks a release asset with the trained filename model. The model
// gives each file's OS/CPU/libc; picker.ChooseJudged applies Yatko's policy.
// It is local, deterministic and free, so it can run on every request.
func Decide(_ string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) picker.AssetDecision {
	candidates := picker.Installable(assets)
	if len(candidates) == 0 {
		return picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow, Reasons: []string{"no installable assets"}}
	}
	judged := make([]picker.FileJudgment, len(candidates))
	for i, a := range candidates {
		pred := Predict(a.Name)
		pOS := pred.OS[string(platform)]
		probs := [3]float64{}
		probs[picker.FitNative] = pOS * sum(pred.Arch, nativeArches[arch])
		probs[picker.FitEmulated] = pOS * sum(pred.Arch, emulatedArches(platform, arch))
		probs[picker.FitNone] = 1 - probs[picker.FitNative] - probs[picker.FitEmulated]
		fit := picker.FitNone
		for level := picker.FitEmulated; level <= picker.FitNative; level++ {
			if probs[level] > probs[fit] {
				fit = level
			}
		}
		judged[i] = picker.FileJudgment{
			Asset: a,
			Fit:   fit,
			FitP:  probs[fit],
			Main:  mainBuild(a, platform, opts),
			Rank:  picker.FormatRank(a.Name, platform, opts),
		}
	}
	return picker.ChooseJudged(judged, "ml")
}

// mainBuild scores "standard build for this visitor" from the filename's
// catalog tokens: variants (debug, profile, baseline, portable...) and, on Linux,
// a libc other than the requested one (glibc by default; static fits any).
// ponytail: libc stays rule-based: headers cannot see static musl, so the model has no libc labels.
func mainBuild(a github.Asset, platform picker.Platform, opts picker.PickOpts) float64 {
	f := picker.FactsForAsset(a)
	main := 0.9
	if len(f.Variants) > 0 {
		main = 0.1
	}
	if platform == picker.Linux {
		want := opts.Libc
		if want == picker.LibcAny {
			want = picker.LibcGNU
		}
		if f.Libc != picker.LibcAny && f.Libc != want && f.Libc != picker.LibcStatic {
			main *= 0.5 // below the 0.5 "main" bar, so a matching libc wins when one exists
		}
	}
	return main
}

func sum(probs map[string]float64, keys []string) float64 {
	total := 0.0
	for _, k := range keys {
		total += probs[k]
	}
	return total
}
