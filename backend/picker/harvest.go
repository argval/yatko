package picker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/argval/yatko/github"
)

// HarvestAsset is one release asset plus what Yatko's artifact inspector
// proved from its contents (ELF/PE/Mach-O headers, archive listings).
type HarvestAsset struct {
	Repo           string                  `json:"repo"`
	Tag            string                  `json:"tag"`
	Name           string                  `json:"name"`
	Status         string                  `json:"status"`
	Targets        []github.ArtifactTarget `json:"targets"`
	TargetsPartial bool                    `json:"targets_partial"`
	LauncherArch   string                  `json:"launcher_arch,omitempty"`
	// Manifest holds publisher manifest facts (harvest_manifests.json), if any.
	Manifest *github.ArtifactMetadata `json:"-"`
	// Deep holds full-file inspection facts (harvest_deep.json), if any.
	Deep *github.ArtifactMetadata `json:"-"`
}

// deepAuthoritative mirrors handlers.authoritativeScanMetadata: a full-file
// result settles the asset when it proves OS and CPU, or proves it unusable.
func deepAuthoritative(m github.ArtifactMetadata) bool {
	if m.Status == "failed" {
		return true
	}
	if m.Status != "inspected" {
		return false
	}
	if m.TargetsPartial {
		return false
	}
	if m.Role == "source" || m.Role == "metadata" || m.Role == "debug" {
		return true
	}
	return len(m.Targets) > 0 && !slices.ContainsFunc(m.Targets, func(t github.ArtifactTarget) bool { return t.Platform == "" || t.Arch == "" })
}

// contentEvidence marks targets as read from the file, as the inspector's own
// evidence does in production.
var contentEvidence = []github.ArtifactEvidence{{Source: "content-inspection", Field: "inspection", Value: "harvest"}}

// Observed combines publisher manifest facts with content inspection the way
// /dl does (github.MergeObservations). An authoritative full-file result
// replaces the shallow content read. ok is false when no evidence exists.
func (a HarvestAsset) Observed() (m github.ArtifactMetadata, ok bool) {
	if a.Deep != nil && a.Deep.TargetsPartial {
		base := a
		base.Deep = nil
		observed, _ := base.Observed()
		return github.MergeObservations(observed, *a.Deep), true
	}
	content := a.Status == "inspected" || a.Status == "failed"
	shallow := github.ArtifactMetadata{Status: a.Status, Targets: a.Targets, Evidence: contentEvidence, TargetsPartial: a.TargetsPartial, LauncherArch: a.LauncherArch}
	switch {
	case a.Deep != nil && deepAuthoritative(*a.Deep) && a.Manifest != nil:
		return github.MergeObservations(*a.Manifest, *a.Deep), true
	case a.Deep != nil && deepAuthoritative(*a.Deep):
		return *a.Deep, true
	case a.Manifest != nil && content:
		return github.MergeObservations(*a.Manifest, shallow), true
	case a.Manifest != nil:
		return *a.Manifest, true
	case content:
		return shallow, true
	}
	return m, false
}

// FromPayload reports whether the asset's labels come from an authoritative
// full-file read of the app payload rather than a shallow header read.
func (a HarvestAsset) FromPayload() bool {
	return a.Deep != nil && deepAuthoritative(*a.Deep)
}

// labelTargets are the targets the scorer trusts: file contents, corroborated
// or filled in by the publisher's manifest.
func (a HarvestAsset) labelTargets() []github.ArtifactTarget {
	m, _ := a.Observed()
	if m.Status == "failed" {
		return nil
	}
	if len(m.Conflicts) > 0 {
		if a.Status == "inspected" {
			return a.Targets
		}
		return nil
	}
	return m.Targets
}

// HarvestRelease groups harvested assets by release.
type HarvestRelease struct {
	Repo   string
	Tag    string
	Assets []HarvestAsset
}

// HarvestVisitors are the visitor buckets the harvest eval scores.
var HarvestVisitors = []struct {
	Platform Platform
	Arch     Arch
}{
	{MacOS, ARM64}, {MacOS, AMD64},
	{Windows, AMD64}, {Windows, ARM64},
	{Linux, AMD64}, {Linux, ARM64},
}

// LoadHarvest reads shared/picker/harvest.jsonl, keeping release order, and
// attaches publisher manifest facts (harvest_manifests.json) and full-file
// inspection facts (harvest_deep.json) from the same directory.
func LoadHarvest(path string) ([]HarvestRelease, error) {
	manifests := map[string]map[string]github.ArtifactMetadata{}
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(path), "harvest_manifests.json")); err == nil {
		if err := json.Unmarshal(data, &manifests); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	deep, err := loadDeepReport(filepath.Join(filepath.Dir(path), "harvest_deep.json"))
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []HarvestRelease
	index := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var a HarvestAsset
		if err := json.Unmarshal(sc.Bytes(), &a); err != nil {
			return nil, err
		}
		var coverage struct {
			TargetsPartial *bool `json:"targets_partial"`
		}
		if err := json.Unmarshal(sc.Bytes(), &coverage); err != nil {
			return nil, err
		}
		// Legacy snapshots did not record whether bounded archive reads stopped early.
		if coverage.TargetsPartial == nil && sampledArchive(a.Name) {
			a.TargetsPartial = true
		}
		key := a.Repo + "@" + a.Tag
		if m, ok := manifests[key][a.Name]; ok {
			a.Manifest = &m
		}
		if m, ok := deep[key][a.Name]; ok {
			a.Deep = &m
		}
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, HarvestRelease{Repo: a.Repo, Tag: a.Tag})
		}
		out[i].Assets = append(out[i].Assets, a)
	}
	return out, sc.Err()
}

// HarvestSplit puts ~70% of repos in "tune" and the rest in "holdout", stable
// across runs. Tune prompts and thresholds only on "tune"; report "holdout" once.
func HarvestSplit(repo string) string {
	h := fnv.New32a()
	h.Write([]byte(repo))
	if h.Sum32()%10 < 7 {
		return "tune"
	}
	return "holdout"
}

// HarvestScore counts outcomes against inspected targets. It judges only
// "does the pick run here", not format or main-build choice; the human-labeled
// corpus covers those.
type HarvestScore struct {
	Cases          int // release × visitor with at least one inspected file that runs
	Runs           int // auto-picked a file proven to run (native, or emulated when no native exists)
	WrongPlatform  int // auto-picked a file proven built for another OS
	WrongArch      int // auto-picked a file proven not to run on this CPU
	EmulatedNative int // auto-picked an emulated build although a native one exists
	Unverified     int // auto-picked a file the inspector could not read
	FalseAbstain   int // no auto-pick although an inspected file runs
	NoRunCases     int // release × visitor where no inspected file provably runs
	NoRunWrong     int // ...and the picker still auto-picked a file proven not to run
	DecideNanos    int64
}

// ArchUnproven marks an inspected target whose OS is proven but whose CPU
// label is not trustworthy (see CorrectedTargets).
const ArchUnproven Arch = "unproven"

// CorrectedTargets fixes known inspector quirks in a file's observed targets,
// so they match what a visitor can actually run:
//   - An empty platform is an ELF/Mach-O slice whose header names no OS. It
//     takes the OS of the file's other slices (universal zips), else an OS the
//     filename names, else stays unknown. A default ELF ABI proves no OS.
//   - On Windows, a 32-bit x86 label on a file whose name names no CPU or
//     another CPU is usually the setup stub, and an x64 label on an arm64(ec)
//     name is a hybrid binary: the CPU is unproven. This applies only to
//     shallow reads; payload (full-file) evidence reads the app itself.
//   - An empty CPU in a complete observation takes the one CPU the filename
//     names, else stays empty, which callers treat as unproven.
//     Installer EXEs keep an unknown payload CPU unproven even with a named CPU.
func CorrectedTargets(name string, observed github.ArtifactMetadata, payload bool) []github.ArtifactTarget {
	targets := observed.Targets
	facts := FactsForAsset(github.Asset{Name: name})
	defaultOS := string(facts.FormatPlatform)
	switch {
	case len(facts.Platforms) == 1:
		defaultOS = string(facts.Platforms[0])
	default:
		for _, os := range []string{"dragonfly", "solaris", "illumos", "sunos", "aix", "plan9", "haiku"} {
			if hasBoundedKeyword(facts.Canonical, os) {
				defaultOS = os
			}
		}
	}
	for _, t := range targets {
		if t.Platform != "" {
			defaultOS = t.Platform
		}
	}
	out := make([]github.ArtifactTarget, len(targets))
	for i, t := range targets {
		if t.Platform == "" {
			t.Platform = defaultOS
		}
		t.Arch = normalizeInspectedArch(t.Arch)
		if t.Arch == "" && !observed.TargetsPartial && len(facts.Arches) == 1 && !strings.HasSuffix(facts.Canonical, ".exe") {
			t.Arch = string(facts.Arches[0])
		}
		if !payload && Platform(t.Platform) == Windows && t.Arch != "" && !slices.Contains(facts.Arches, Arch(t.Arch)) &&
			(Arch(t.Arch) == X86 || len(facts.Arches) > 0) {
			t.Arch = string(ArchUnproven)
		}
		out[i] = t
	}
	return out
}

// normalizeInspectedArch maps the inspector's package-format arch spellings
// (rpm:armv7, linux-i686, arm64e) onto picker Arch values.
func normalizeInspectedArch(a string) string {
	a = strings.TrimPrefix(strings.TrimPrefix(a, "rpm:"), "linux-")
	switch a {
	case "i686":
		return string(X86)
	case "armv5", "armv5tel", "armv6", "armv7", "armhf":
		return string(ARM)
	case "arm64e":
		return string(ARM64)
	}
	return a
}

// fit is how an inspected asset runs for a visitor: 0 no, 1 emulated,
// 2 native, or fitUnproven when the OS or CPU is not proven.
func fit(a HarvestAsset, platform Platform, arch Arch) (fitLevel int, otherOS bool) {
	otherOS = true
	unproven := false
	observed, _ := a.Observed()
	observed.Targets = a.labelTargets()
	for _, t := range CorrectedTargets(a.Name, observed, a.FromPayload()) {
		if t.Platform == "" || strings.HasPrefix(t.Platform, "tauri-target:") {
			unproven = true
			otherOS = false
			continue
		}
		if Platform(t.Platform) != platform {
			continue
		}
		otherOS = false
		switch {
		case Arch(t.Arch) == ArchUnproven || t.Arch == "":
			// An unknown CPU is not proof that it runs: AutoClip_aarch64.dmg
			// has no inspected CPU and does not run on an Intel Mac.
			unproven = true
		case Arch(t.Arch) == arch:
			return 2, false
		default:
			if _, ok := EmulationRank(platform, arch, Arch(t.Arch)); ok {
				fitLevel = 1
			}
		}
	}
	if fitLevel == 0 && unproven {
		return fitUnproven, false
	}
	if fitLevel == 0 && observed.TargetsPartial {
		facts := Classify(a.Name)
		if !otherOS || !(IncompatiblePlatform(facts, platform) || facts.FormatPlatform != "" && facts.FormatPlatform != platform) {
			return fitUnproven, false
		}
	}
	return fitLevel, otherOS
}

func sampledArchive(name string) bool {
	n := strings.ToLower(name)
	for _, suffix := range []string{".zip", ".7z", ".tar", ".tar.gz", ".tgz", ".tar.xz", ".txz", ".tar.bz2", ".tbz2", ".tbz", ".tar.zst", ".tzst"} {
		if strings.HasSuffix(n, suffix) {
			return true
		}
	}
	return false
}

const fitUnproven = -1

// ScoreHarvestWith runs decide over every release and visitor. Cold (warm=false)
// passes names only, the evidence a first /dl request has. Warm also passes the
// inspector's results, as /dl has once enrichArtifacts has probed a release;
// it is an upper bound, since the scorer's labels are the same observations.
// workers > 1 runs cases in parallel; decide must be safe for that.
func ScoreHarvestWith(releases []HarvestRelease, decide Decider, workers int, warm bool) (HarvestScore, []string) {
	type job struct {
		rel     HarvestRelease
		visitor int
	}
	type result struct {
		score HarvestScore
		line  string
	}
	jobs := make(chan job)
	results := make(chan result)
	var wg sync.WaitGroup
	for range max(workers, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				s, line := scoreHarvestCase(j.rel, HarvestVisitors[j.visitor].Platform, HarvestVisitors[j.visitor].Arch, decide, warm)
				results <- result{s, line}
			}
		}()
	}
	go func() {
		for _, rel := range releases {
			for v := range HarvestVisitors {
				jobs <- job{rel, v}
			}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var total HarvestScore
	var lines []string
	for r := range results {
		s := r.score
		total.Cases += s.Cases
		total.Runs += s.Runs
		total.WrongPlatform += s.WrongPlatform
		total.WrongArch += s.WrongArch
		total.EmulatedNative += s.EmulatedNative
		total.Unverified += s.Unverified
		total.FalseAbstain += s.FalseAbstain
		total.NoRunCases += s.NoRunCases
		total.NoRunWrong += s.NoRunWrong
		total.DecideNanos += s.DecideNanos
		if r.line != "" {
			lines = append(lines, r.line)
		}
	}
	return total, lines
}

func scoreHarvestCase(rel HarvestRelease, platform Platform, arch Arch, decide Decider, warm bool) (HarvestScore, string) {
	var s HarvestScore
	bestFit := 0
	byName := map[string]HarvestAsset{}
	assets := make([]github.Asset, len(rel.Assets))
	for i, a := range rel.Assets {
		assets[i] = github.Asset{Name: a.Name}
		if m, ok := a.Observed(); warm && ok {
			assets[i].Artifact = &m
		}
		byName[a.Name] = a
		if f, _ := fit(a, platform, arch); f > bestFit {
			bestFit = f
		}
	}
	tag := fmt.Sprintf("%s %s/%s", rel.Repo, platform, arch)
	if bestFit <= 0 {
		// Nothing provably runs here, so only a proven-wrong pick is scorable:
		// a picker that guesses an OS for unmarked files fails here, not above.
		s.NoRunCases = 1
		d, elapsed := timedDecide(decide, rel.Repo, assets, platform, arch)
		s.DecideNanos = elapsed
		if !d.ShouldAutoSelect() {
			return s, ""
		}
		picked := byName[d.Asset.Name]
		if f, _ := fit(picked, platform, arch); len(picked.labelTargets()) > 0 && f == 0 {
			s.NoRunWrong = 1
			return s, fmt.Sprintf("%s: WRONG pick %s where nothing runs", tag, d.Asset.Name)
		}
		return s, ""
	}
	s.Cases = 1

	d, elapsed := timedDecide(decide, rel.Repo, assets, platform, arch)
	s.DecideNanos = elapsed
	if !d.ShouldAutoSelect() {
		s.FalseAbstain = 1
		return s, fmt.Sprintf("%s: abstained (%s) %s", tag, d.Confidence, strings.Join(d.Reasons, "; "))
	}
	picked := byName[d.Asset.Name]
	if len(picked.labelTargets()) == 0 {
		s.Unverified = 1
		return s, fmt.Sprintf("%s: picked uninspected %s", tag, d.Asset.Name)
	}
	f, otherOS := fit(picked, platform, arch)
	switch {
	case f == fitUnproven:
		s.Unverified = 1
		return s, fmt.Sprintf("%s: picked %s (OS or CPU not proven)", tag, d.Asset.Name)
	case otherOS:
		s.WrongPlatform = 1
		return s, fmt.Sprintf("%s: WRONG OS %s", tag, d.Asset.Name)
	case f == 0:
		s.WrongArch = 1
		return s, fmt.Sprintf("%s: WRONG CPU %s", tag, d.Asset.Name)
	case f == 1 && bestFit == 2:
		s.EmulatedNative = 1
		return s, fmt.Sprintf("%s: emulated %s though native exists", tag, d.Asset.Name)
	}
	s.Runs = 1
	return s, ""
}

func timedDecide(decide Decider, repo string, assets []github.Asset, platform Platform, arch Arch) (AssetDecision, int64) {
	start := time.Now()
	d := decide(repo, assets, platform, arch, PickOpts{})
	return d, time.Since(start).Nanoseconds()
}

func (s HarvestScore) String() string {
	pct := func(n int) float64 { return 100 * float64(n) / float64(max(s.Cases, 1)) }
	return fmt.Sprintf("cases=%d runs=%d (%.1f%%) wrong_os=%d wrong_cpu=%d emulated_over_native=%d unverified=%d (%.1f%%) false_abstain=%d (%.1f%%) norun_cases=%d norun_wrong=%d",
		s.Cases, s.Runs, pct(s.Runs), s.WrongPlatform, s.WrongArch, s.EmulatedNative, s.Unverified, pct(s.Unverified), s.FalseAbstain, pct(s.FalseAbstain), s.NoRunCases, s.NoRunWrong)
}

// loadDeepReport reads an `artifactscan inspect` report (a JSON array of
// {repo, tag, asset, metadata}) keyed by repo@tag and asset name. A missing
// file means no full-file evidence.
func loadDeepReport(path string) (map[string]map[string]github.ArtifactMetadata, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var report []struct {
		Repo     string                   `json:"repo"`
		Tag      string                   `json:"tag"`
		Asset    github.Asset             `json:"asset"`
		Metadata *github.ArtifactMetadata `json:"metadata"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	out := map[string]map[string]github.ArtifactMetadata{}
	for _, r := range report {
		if r.Metadata == nil {
			continue
		}
		key := r.Repo + "@" + r.Tag
		if out[key] == nil {
			out[key] = map[string]github.ArtifactMetadata{}
		}
		out[key][r.Asset.Name] = *r.Metadata
	}
	return out, nil
}
