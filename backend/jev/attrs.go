package jev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
	"golang.org/x/sync/singleflight"
)

// Option keys sent to Jev. Keys are self-describing, so most need no criteria text.
const (
	osMac, osWindows, osLinux, osAndroid, osIOS = "macOS", "Windows", "Linux", "Android", "iOS"
	osBSD, osOther, osNotStated                 = "FreeBSD or another BSD", "another platform (web, Java, a game console, or a plugin host)", "operating system not stated"

	cpuAMD64, cpuARM64, cpuARM, cpuX86   = "64-bit x86 (x86_64, x64, amd64)", "64-bit ARM (arm64, aarch64)", "32-bit ARM (arm, armv6, armv7, armhf)", "32-bit x86 (386, i386, i686, x86, win32)"
	cpuUniversal, cpuOther, cpuNotStated = "universal (one macOS build for both Intel and Apple Silicon)", "another CPU (RISC-V, PowerPC, s390x, MIPS, LoongArch)", "CPU not stated"

	roleMain = "the main program, in its standard build"
)

var osCriteria = orderedCriteria{
	{name: osMac}, {name: osWindows},
	{name: osLinux, desc: "Includes .deb, .rpm, .AppImage, and Alpine Linux .apk packages (an .apk named with x86_64, aarch64 or armhf and not Android)."},
	{name: osAndroid, desc: "An Android app (.apk or .aab)."},
	{name: osIOS}, {name: osBSD}, {name: osOther}, {name: osNotStated},
}

var cpuCriteria = orderedCriteria{
	{name: cpuAMD64}, {name: cpuARM64}, {name: cpuARM}, {name: cpuX86},
	{name: cpuUniversal}, {name: cpuOther}, {name: cpuNotStated},
}

// The main-program option says that format, libc and CPU never make a build
// "special": those are code's choices, and Jev rated .tar.gz next to a bare
// binary, or a musl build, as variants when this was left unsaid.
var roleCriteria = orderedCriteria{
	{name: roleMain, desc: "The program the repository is about. The same program packaged in another file format (.tar.gz, .tar.xz, .zip, .AppImage, .deb, a bare binary), built for another libc (glibc, musl, static) or for another CPU is still the main program."},
	{name: "a companion tool", desc: "A separate CLI, server, daemon, SDK, library, editor extension (.vsix) or plugin shipped next to a main app."},
	{name: "a debug, symbols or profiling build"},
	{name: "a special variant", desc: "Baseline, legacy, portable, lite, minimal, no-web or similar, next to a standard build of the same program."},
	{name: "not a program", desc: "Source code, checksums, signatures, documentation, or data."},
}

var platformOS = map[picker.Platform]string{
	picker.MacOS: osMac, picker.Windows: osWindows, picker.Linux: osLinux, picker.Android: osAndroid, picker.IOS: osIOS,
}

var cpuArch = map[string]picker.Arch{cpuAMD64: picker.AMD64, cpuARM64: picker.ARM64, cpuARM: picker.ARM, cpuX86: picker.X86}

// osQuestion lets Jev infer an unmarked file's OS from the rest of the
// release. Those guesses are calibrated but not always right, so Decide
// auto-selects an inferred OS only at inferredOSThreshold.
const osQuestion = "Which operating system is `file` built for? If the name of `file` does not say, " +
	"use the other files in `files`: a release usually ships one set of builds per operating system, " +
	"so an unmarked file usually belongs to an operating system that has no other build in the release. " +
	"A portable build is usually for Windows. Answer \"operating system not stated\" only when `files` gives no clue either."

// inferredOSThreshold gates guesses for files whose name names no OS. On the
// tune split, guesses at >= 0.9 were right 236 times and wrong twice; at 0.5,
// 302 right and 12 wrong. Below it a file only "maybe" fits.
const inferredOSThreshold = 0.9

// filesPerRequest keeps one request well inside the 64k-token context:
// three questions per file. ponytail: sequential chunks; parallelize if big releases get slow.
const filesPerRequest = 40

// FileAttrs is what Jev believes about one file, independent of any visitor.
type FileAttrs struct {
	Name string
	OS   map[string]float64 // option key → probability
	CPU  map[string]float64
	Main float64 // probability it is the main program's standard build
}

func attrQuestion(file, text string, criteria orderedCriteria) question {
	return question{Type: "choice", Instructions: map[string]string{"file": file, "question": text}, Criteria: criteria}
}

// Describe asks Jev what each installable file is: OS, CPU and role. The
// answers do not depend on the visitor, so one cached call serves every visitor.
func (c *Client) Describe(ctx context.Context, repo string, assets []github.Asset) ([]FileAttrs, Usage, error) {
	candidates := installable(assets, true)
	names := make([]string, len(candidates))
	for i, a := range candidates {
		names[i] = a.Name
	}
	key := cacheKey(c.model, repo, names)
	if attrs, ok := c.attrCache.get(key); ok {
		return attrs, Usage{Model: c.model, Consistent: true}, nil
	}
	// Concurrent visitors of one cold release share a single paid call. Only
	// the caller that made it reports its tokens, so cost totals stay honest.
	paid := Usage{Model: c.model, Consistent: true}
	v, err, _ := c.attrCache.flight.Do(key, func() (any, error) {
		attrs, usage, err := c.describe(ctx, repo, names)
		if err == nil {
			c.attrCache.put(key, attrs)
			paid = usage
		}
		return attrs, err
	})
	if err != nil {
		return nil, Usage{}, err
	}
	return v.([]FileAttrs), paid, nil
}

func (c *Client) describe(ctx context.Context, repo string, names []string) ([]FileAttrs, Usage, error) {
	usage := Usage{Consistent: true}
	attrs := make([]FileAttrs, 0, len(names))
	for start := 0; start < len(names); start += filesPerRequest {
		chunk := names[start:min(start+filesPerRequest, len(names))]
		questions := make(map[string]question, 3*len(chunk))
		for i, name := range chunk {
			questions[fmt.Sprintf("os_%d", i)] = attrQuestion(name, osQuestion, osCriteria)
			questions[fmt.Sprintf("cpu_%d", i)] = attrQuestion(name, "Which CPU is `file` built for?", cpuCriteria)
			questions[fmt.Sprintf("role_%d", i)] = attrQuestion(name, "What is `file` in this release of `repository`? `files` lists every file in the release.", roleCriteria)
		}
		resp, err := c.post(ctx, request{
			Model:     c.model,
			State:     map[string]any{"repository": repo, "files": names},
			Questions: questions,
		})
		if err != nil {
			return nil, Usage{}, err
		}
		usage.Model = resp.Model
		usage.InputTokens += resp.Usage.InputTokens
		for i, name := range chunk {
			osA, ok1 := resp.Answers[fmt.Sprintf("os_%d", i)]
			cpuA, ok2 := resp.Answers[fmt.Sprintf("cpu_%d", i)]
			roleA, ok3 := resp.Answers[fmt.Sprintf("role_%d", i)]
			if !ok1 || !ok2 || !ok3 {
				return nil, Usage{}, fmt.Errorf("jev: response missing answers for %q", name)
			}
			attrs = append(attrs, FileAttrs{Name: name, OS: osA.Probabilities, CPU: cpuA.Probabilities, Main: roleA.Probabilities[roleMain]})
		}
	}
	return attrs, usage, nil
}

// Decide asks Jev what each file is (Describe), then matches the visitor in
// code: P(native) = P(OS) × P(CPU runs natively). The "CPU not stated" policy
// and the format preference are Yatko's rules, so they live here, not in a prompt.
func (c *Client) Decide(ctx context.Context, repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, Usage, error) {
	if _, ok := platformOS[platform]; !ok {
		return picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow, Reasons: []string{"unknown platform"}}, Usage{}, nil
	}
	attrs, usage, err := c.Describe(ctx, repo, assets)
	if err != nil {
		return picker.AssetDecision{}, Usage{}, err
	}
	if len(attrs) == 0 {
		return picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow, Reasons: []string{"no installable assets"}}, usage, nil
	}
	if arch == picker.UnknownArch {
		arch = picker.AMD64 // most computers; rules make the same assumption
	}
	inspected := map[string]github.Asset{}
	for _, a := range assets {
		if a.Artifact != nil && a.Artifact.Status == "inspected" && len(a.Artifact.Targets) > 0 {
			inspected[a.Name] = a
		}
	}
	wantMusl := opts.Libc == picker.LibcMusl
	judged := make([]fileJudgment, len(attrs))
	for i, a := range attrs {
		obs, observed := inspected[a.Name]
		switch {
		case observed:
			a = withObservedTargets(a, picker.CorrectedTargets(obs.Name, obs.Artifact.Targets))
		case appBundle(a.Name):
			// Code knows this one: both Jev versions read Tauri's
			// Name_x64.app.tar.gz as Linux.
			a.OS, observed = map[string]float64{osMac: 1}, true
		}
		native, emulated, maybe := runProbability(a, platform, arch)
		if f := picker.FactsForAsset(github.Asset{Name: a.Name}); !observed && len(f.Platforms) == 0 && f.FormatPlatform == "" &&
			a.OS[platformOS[platform]] < inferredOSThreshold {
			// The OS is Jev's inference, and not a confident one: never auto-select.
			native, emulated, maybe = 0, 0, native+emulated+maybe
		}
		j := fileJudgment{asset: github.Asset{Name: a.Name}, main: a.Main, rank: picker.FormatRank(a.Name, platform, opts)}
		switch {
		case native >= 0.5:
			j.fit, j.fitP = fitNative, native
		case native+emulated >= 0.5:
			j.fit, j.fitP = fitEmulated, native+emulated
		case native+emulated+maybe >= 0.5:
			j.fit, j.fitP = fitMaybe, native+emulated+maybe
		default:
			j.fit, j.fitP = fitNone, 1-native-emulated-maybe
		}
		// libc is spelled out in filenames, so code reads it: glibc is the default.
		j.libcMiss = picker.FactsForAsset(j.asset).HasMusl != wantMusl
		j.universal = platform == picker.MacOS && a.CPU[cpuUniversal] >= 0.5
		judged[i] = j
	}
	d := choose(judged, func(tied []fileJudgment) (int, bool, string) {
		return c.breakTie(ctx, repo, platform, arch, tied, &usage)
	})
	// Return the caller's asset (with its download URL), not our name-only copy.
	if d.Asset != nil {
		for i := range assets {
			if assets[i].Name == d.Asset.Name {
				d.Asset = &assets[i]
				break
			}
		}
	}
	return d, usage, nil
}

// runProbability combines Jev's OS and CPU distributions for one visitor.
// "CPU not stated" means x86-64 on Windows and Linux and any CPU on Android.
// On macOS it always runs on Apple Silicon (native, Rosetta or universal). On
// an Intel Mac, unmarked .dmg/.pkg installers run (universal or Intel in
// practice), but ~20% of unmarked zips and binaries were arm64-only in the
// harvest, so those only "maybe" run.
func runProbability(a FileAttrs, platform picker.Platform, arch picker.Arch) (native, emulated, maybe float64) {
	pOS := a.OS[platformOS[platform]]
	macInstaller := picker.FactsForAsset(github.Asset{Name: a.Name}).FormatPlatform == picker.MacOS
	for option, p := range a.CPU {
		cpu, known := cpuArch[option]
		switch {
		case known && cpu == arch:
			native += p
		case option == cpuUniversal && platform == picker.MacOS:
			native += p
		case option == cpuNotStated && platform == picker.Android:
			native += p
		case option == cpuNotStated && platform == picker.MacOS && (arch == picker.ARM64 || macInstaller):
			emulated += p
		case option == cpuNotStated && platform == picker.MacOS:
			maybe += p
		case option == cpuNotStated && (platform == picker.Windows || platform == picker.Linux):
			cpu, known = picker.AMD64, true
			fallthrough
		case known:
			if cpu == arch {
				native += p
			} else if _, ok := picker.EmulationRank(platform, arch, cpu); ok {
				emulated += p
			}
		}
	}
	return pOS * native, pOS * emulated, pOS * maybe
}

func cacheKey(model, repo string, names []string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + repo + "\x00" + strings.Join(names, "\x00")))
	return hex.EncodeToString(sum[:])
}

// attrCache remembers Describe results per release in this process, so every
// visitor bucket after the first is free. ponytail: reset at 2k releases;
// move to the Redis cache if shadow traffic shows repeat calls across instances.
type attrCache struct {
	mu     sync.Mutex
	m      map[string][]FileAttrs
	flight singleflight.Group
}

func (c *attrCache) get(key string) ([]FileAttrs, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	return v, ok
}

func (c *attrCache) put(key string, v []FileAttrs) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) >= 2000 {
		c.m = map[string][]FileAttrs{}
	}
	c.m[key] = v
}

// tieQuestion compares near-equal files directly. Describe judges each file
// alone, so two builds of one app look the same; a Choice compares them.
const tieQuestion = "All `candidates` run on the `visitor`'s computer. Which one should a typical visitor download " +
	"to install and use the main program of `repository`? Prefer the standard build over a special edition, " +
	"and a build for a common, current system version over one for an older or niche version."

// breakTie asks one Choice over the tied files in forward and reverse order
// (Jev 1.13 leans toward the first option) and accepts the winner only when
// both orders agree and it has a clear majority (see tieBreakMargin).
func (c *Client) breakTie(ctx context.Context, repo string, platform picker.Platform, arch picker.Arch, tied []fileJudgment, usage *Usage) (int, bool, string) {
	forward := make(orderedCriteria, len(tied))
	names := make([]string, len(tied))
	for i, j := range tied {
		forward[i] = option{name: j.asset.Name}
		names[i] = j.asset.Name
	}
	reverse := slices.Clone(forward)
	slices.Reverse(reverse)
	visitor := map[string]string{"operating_system": osNames[platform], "cpu": cpuNames[arch]}
	resp, err := c.post(ctx, request{
		Model: c.model,
		State: map[string]any{"repository": repo, "visitor": visitor, "candidates": names},
		Questions: map[string]question{
			"forward": {Type: "choice", Instructions: tieQuestion, Criteria: forward},
			"reverse": {Type: "choice", Instructions: tieQuestion, Criteria: reverse},
		},
	})
	if err != nil {
		return 0, false, "jev tie-break failed: " + err.Error()
	}
	usage.InputTokens += resp.Usage.InputTokens
	fwd, rev := resp.Answers["forward"], resp.Answers["reverse"]
	best, bestP := 0, -1.0
	for i, name := range names {
		if p := (fwd.Probabilities[name] + rev.Probabilities[name]) / 2; p > bestP {
			best, bestP = i, p
		}
	}
	need := max(0.5, 1/float64(len(names))+tieBreakMargin)
	if len(names) >= manyProgramsTie {
		need = manyProgramsProbability
	}
	ok := fwd.Choice == rev.Choice && fwd.Choice == names[best] && bestP >= need
	return best, ok, fmt.Sprintf("jev tie-break among %d: %q p=%.2f agree=%v", len(names), names[best], bestP, fwd.Choice == rev.Choice)
}

// tieBreakMargin: the winner needs a majority and must beat an even split by
// this much, and both orders must agree. Every tied file already runs on the
// visitor's computer, so a weak but consistent preference is safe; Pake's 16
// different apps gave ChatGPT.dmg a consistent 45%, which still abstains.
const tieBreakMargin = 0.05

// After every policy step one program rarely leaves 4+ equal files, so a tie
// that large usually means several different programs (Pake ships 16 apps,
// and jev-1.14 gave ChatGPT_x86_64.AppImage 56%). Those need a strong winner.
const (
	manyProgramsTie         = 4
	manyProgramsProbability = 0.8
)

var platformKey = map[string]string{
	"macos": osMac, "windows": osWindows, "linux": osLinux, "android": osAndroid, "ios": osIOS,
	"freebsd": osBSD, "openbsd": osBSD, "netbsd": osBSD, "dragonfly": osBSD,
}

// withObservedTargets replaces Jev's OS/CPU guesses with what Yatko's artifact
// inspector proved from the file's contents. The role stays Jev's judgment, and
// an unproven CPU (a Windows setup stub) keeps Jev's CPU answer.
func withObservedTargets(a FileAttrs, targets []github.ArtifactTarget) FileAttrs {
	osKey := ""
	var arches []picker.Arch
	for _, t := range targets {
		k, ok := platformKey[t.Platform]
		if !ok {
			k = osOther
		}
		if osKey != "" && osKey != k {
			return a // several OSes in one file: keep Jev's view
		}
		osKey = k
		arches = append(arches, picker.Arch(t.Arch))
	}
	a.OS = map[string]float64{osKey: 1}
	slices.Sort(arches)
	arches = slices.Compact(arches)
	switch {
	case len(arches) == 2 && osKey == osMac && slices.Contains(arches, picker.AMD64) && slices.Contains(arches, picker.ARM64):
		a.CPU = map[string]float64{cpuUniversal: 1}
	case len(arches) == 1 && arches[0] != picker.UnknownArch && arches[0] != picker.ArchUnproven:
		a.CPU = map[string]float64{cpuOther: 1}
		for key, arch := range cpuArch {
			if arch == arches[0] {
				a.CPU = map[string]float64{key: 1}
			}
		}
	}
	return a
}

// appBundle reports a zipped or tarred macOS .app bundle (Maccy.app.zip,
// Yaak_x64.app.tar.gz).
func appBundle(name string) bool {
	n := strings.ToLower(name)
	for _, ext := range []string{".app.zip", ".app.tar.gz", ".app.tgz", ".app.tar.xz"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}
