package jev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

var roleCriteria = orderedCriteria{
	{name: roleMain},
	{name: "a companion tool", desc: "A separate CLI, server, daemon, SDK, library, editor extension (.vsix) or plugin shipped next to a main app."},
	{name: "a debug, symbols or profiling build"},
	{name: "a special variant", desc: "Baseline, legacy, portable, lite, minimal, no-web or similar, next to a standard build of the same program."},
	{name: "not a program", desc: "Source code, checksums, signatures, documentation, or data."},
}

var platformOS = map[picker.Platform]string{
	picker.MacOS: osMac, picker.Windows: osWindows, picker.Linux: osLinux, picker.Android: osAndroid, picker.IOS: osIOS,
}

var cpuArch = map[string]picker.Arch{cpuAMD64: picker.AMD64, cpuARM64: picker.ARM64, cpuARM: picker.ARM, cpuX86: picker.X86}

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
	candidates := installable(assets)
	names := make([]string, len(candidates))
	for i, a := range candidates {
		names[i] = a.Name
	}
	key := cacheKey(repo, names)
	if attrs, ok := c.attrCache.get(key); ok {
		return attrs, Usage{Model: Model, Consistent: true}, nil
	}
	// Concurrent visitors of one cold release share a single paid call. Only
	// the caller that made it reports its tokens, so cost totals stay honest.
	paid := Usage{Model: Model, Consistent: true}
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
			questions[fmt.Sprintf("os_%d", i)] = attrQuestion(name, "Which operating system is `file` built for?", osCriteria)
			questions[fmt.Sprintf("cpu_%d", i)] = attrQuestion(name, "Which CPU is `file` built for?", cpuCriteria)
			questions[fmt.Sprintf("role_%d", i)] = attrQuestion(name, "What is `file` in this release of `repository`? `files` lists every file in the release.", roleCriteria)
		}
		resp, err := c.post(ctx, request{
			Model:     Model,
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
	wantMusl := opts.Libc == picker.LibcMusl
	judged := make([]fileJudgment, len(attrs))
	for i, a := range attrs {
		native, emulated := runProbability(a, platform, arch)
		j := fileJudgment{asset: github.Asset{Name: a.Name}, main: a.Main, rank: picker.FormatRank(a.Name, platform, opts)}
		switch {
		case native >= 0.5:
			j.fit, j.fitP = fitNative, native
		case native+emulated >= 0.5:
			j.fit, j.fitP = fitEmulated, native+emulated
		default:
			j.fit, j.fitP = fitNone, 1-native-emulated
		}
		// libc is spelled out in filenames, so code reads it: glibc is the default.
		j.libcMiss = picker.FactsForAsset(j.asset).HasMusl != wantMusl
		judged[i] = j
	}
	d := choose(judged)
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
// "CPU not stated" means x86-64 on Windows and Linux, any CPU on Android, and
// unproven on macOS (it may be universal or arm64-only), so it is never native there.
func runProbability(a FileAttrs, platform picker.Platform, arch picker.Arch) (native, emulated float64) {
	pOS := a.OS[platformOS[platform]]
	for option, p := range a.CPU {
		cpu, known := cpuArch[option]
		switch {
		case known && cpu == arch:
			native += p
		case option == cpuUniversal && platform == picker.MacOS:
			native += p
		case option == cpuNotStated && platform == picker.Android:
			native += p
		case option == cpuNotStated && platform == picker.MacOS:
			emulated += p
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
	return pOS * native, pOS * emulated
}

func cacheKey(repo string, names []string) string {
	sum := sha256.Sum256([]byte(Model + "\x00" + repo + "\x00" + strings.Join(names, "\x00")))
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
