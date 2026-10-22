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

// Option keys for Describe. Keys are self-describing, so most need no criteria text.
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
// "special": Jev rated .tar.gz next to a bare binary, or a musl build, as
// variants when this was left unsaid.
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
// release. Those guesses are calibrated but not always right, so fit only
// trusts an inferred OS at inferredOSThreshold.
const osQuestion = "Which operating system is `file` built for? If the name of `file` does not say, " +
	"use the other files in `files`: a release usually ships one set of builds per operating system, " +
	"so an unmarked file usually belongs to an operating system that has no other build in the release. " +
	"A portable build is usually for Windows. Answer \"operating system not stated\" only when `files` gives no clue either."

// inferredOSThreshold gates guesses for files whose name names no OS. On the
// harvest tune split, guesses at >= 0.9 were right 236 times and wrong twice;
// at 0.5, 302 right and 12 wrong. Below it a file only "maybe" fits.
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

// Describe asks Jev what each file is: OS, CPU and role. The answers do not
// depend on the visitor, so one cached call serves every visitor of a release.
// product (the repo description, may be empty) helps tell the main program
// from companions.
func (c *Client) Describe(ctx context.Context, repo, product string, assets []github.Asset) ([]FileAttrs, Usage, error) {
	names := make([]string, len(assets))
	for i, a := range assets {
		names[i] = a.Name
	}
	key := cacheKey(c.model, repo+"\x00"+product, names)
	if attrs, ok := c.attrCache.get(key); ok {
		return attrs, Usage{Model: c.model}, nil
	}
	// Concurrent visitors of one cold release share a single paid call. Only
	// the caller that made it reports its tokens, so cost totals stay honest.
	paid := Usage{Model: c.model}
	v, err, _ := c.attrCache.flight.Do(key, func() (any, error) {
		attrs, usage, err := c.describe(ctx, repo, product, names)
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

func (c *Client) describe(ctx context.Context, repo, product string, names []string) ([]FileAttrs, Usage, error) {
	var usage Usage
	attrs := make([]FileAttrs, 0, len(names))
	for start := 0; start < len(names); start += filesPerRequest {
		chunk := names[start:min(start+filesPerRequest, len(names))]
		questions := make(map[string]question, 3*len(chunk))
		for i, name := range chunk {
			questions[fmt.Sprintf("os_%d", i)] = attrQuestion(name, osQuestion, osCriteria)
			questions[fmt.Sprintf("cpu_%d", i)] = attrQuestion(name, "Which CPU is `file` built for?", cpuCriteria)
			questions[fmt.Sprintf("role_%d", i)] = attrQuestion(name, "What is `file` in this release of `repository`? `files` lists every file in the release; `product`, when present, describes the repository.", roleCriteria)
		}
		state := map[string]any{"repository": repo, "files": names}
		if product != "" {
			state["product"] = product
		}
		resp, err := c.post(ctx, request{Model: c.model, State: state, Questions: questions})
		if err != nil {
			return nil, Usage{}, err
		}
		usage.Model = resp.Model
		usage.InputTokens += resp.Usage.InputTokens
		for i, name := range chunk {
			osA, err := describeAnswer(resp, "os", i, name, osCriteria)
			if err != nil {
				return nil, Usage{}, err
			}
			cpuA, err := describeAnswer(resp, "cpu", i, name, cpuCriteria)
			if err != nil {
				return nil, Usage{}, err
			}
			roleA, err := describeAnswer(resp, "role", i, name, roleCriteria)
			if err != nil {
				return nil, Usage{}, err
			}
			attrs = append(attrs, FileAttrs{Name: name, OS: osA.Probabilities, CPU: cpuA.Probabilities, Main: roleA.Probabilities[roleMain]})
		}
	}
	return attrs, usage, nil
}

// describeAnswer returns the answer to question kind_i, which must be a Choice
// valid for criteria (see validChoice). The error names the question and file.
func describeAnswer(resp *response, kind string, i int, file string, criteria orderedCriteria) (answer, error) {
	a, ok := resp.Answers[fmt.Sprintf("%s_%d", kind, i)]
	if !ok || a.Type != "choice" {
		return answer{}, fmt.Errorf("jev: malformed response: missing %s choice answer for %q", kind, file)
	}
	options := make([]string, len(criteria))
	for j, o := range criteria {
		options[j] = o.name
	}
	if err := validChoice(a, options); err != nil {
		return answer{}, fmt.Errorf("%s answer for %q: %w", kind, file, err)
	}
	return a, nil
}

// runProbability combines Jev's OS and CPU distributions for one visitor.
// "CPU not stated" means x86-64 on Windows and Linux and any CPU on Android.
// On macOS it always runs on Apple Silicon (native, Rosetta or universal). On
// an Intel Mac, unmarked .dmg/.pkg installers run (universal or Intel in
// practice), but ~20% of unmarked zips and binaries were arm64-only in the
// harvest, so those only "maybe" run. With the visitor's CPU unknown, any
// common CPU counts and the Choice and confidence caps handle the rest.
func runProbability(a FileAttrs, platform picker.Platform, arch picker.Arch) (native, emulated, maybe float64) {
	pOS := a.OS[platformOS[platform]]
	macInstaller := picker.FactsForAsset(github.Asset{Name: a.Name}).FormatPlatform == picker.MacOS
	for option, p := range a.CPU {
		cpu, known := cpuArch[option]
		switch {
		case arch == picker.UnknownArch:
			if option != cpuOther {
				native += p
			}
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
// visitor after the first skips the call. ponytail: reset at 2k releases;
// move to the Redis cache if primary logs show repeat calls across instances.
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
		arches = append(arches, picker.Arch(t.Arch))
		if t.Platform == "" {
			continue
		}
		k, ok := platformKey[t.Platform]
		if !ok {
			k = osOther
		}
		if osKey != "" && osKey != k {
			return a // several OSes in one file: keep Jev's view
		}
		osKey = k
	}
	if osKey != "" {
		a.OS = map[string]float64{osKey: 1}
	}
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

// observedArtifact reports whether targets come from content or a publisher
// manifest. After enrichment every asset carries filename-derived targets too
// (picker.DescribeAsset), which must not override Jev's reading of the name.
func observedArtifact(m *github.ArtifactMetadata) bool {
	return m != nil && m.Status == "inspected" && len(m.Targets) > 0 &&
		slices.ContainsFunc(m.Evidence, func(e github.ArtifactEvidence) bool { return e.Source != "" && e.Source != "filename" })
}
