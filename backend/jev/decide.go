package jev

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// The hybrid decision, per visitor context:
//  1. Code guards the release (ValidateSelection) and Describe asks Jev, once
//     per release, what each file is: OS, CPU and role (describe.go).
//  2. Code turns those answers, overridden by inspected targets, into a fit
//     tier per file (native, emulated, maybe) and keeps the files that run.
//  3. When several files run, one Choice asks Jev which of them this visitor
//     should download (format, main program, variant, explicit requests).
//
// promptVersion is hashed into Primary's cache key; bump it whenever the
// questions, state shape or decision policy below change.
const promptVersion = "hybrid-v6"

const (
	// NoCompatibleOption names "no file fits" in logs and tests; code decides
	// it from fit tiers, so it is never a Choice option.
	NoCompatibleOption = NoneOption
	// InsufficientOption: several files run, but the facts cannot tell which one.
	InsufficientOption = "not enough information"
	// maxCandidates bounds one decision. Above it Decide errors and the rule
	// picker answers.
	maxCandidates = 100
)

// Thresholds that turn Jev's answers into confidence.
// ponytail: hand-set; retune on the harvest tune split and jev_primary_diff logs.
const (
	// chosenMedium/chosenHigh: Choice probability of the chosen file (1 when
	// only one file runs and no Choice is asked).
	chosenMedium = 0.5
	chosenHigh   = 0.8
	// fitMedium/fitHigh: probability the chosen file runs at its fit tier.
	fitMedium = 0.5
	fitHigh   = 0.8
	// clearLead requires a file to lead abstention by this factor. A split
	// preference vote below chosenMedium can still count as medium when the
	// chosen file leads every other option by the same factor.
	clearLead = 1.5
)

// Fit tiers, low to high. fitMaybe may run but is never auto-selected
// (an unmarked Mac build on an Intel Mac could be arm64-only).
const (
	fitNone = iota
	fitMaybe
	fitEmulated
	fitNative
)

var tierNames = []string{"none", "maybe", "emulated", "native"}

var choiceInstructions = map[string]any{
	"question": "Every option is a file from this release of `repository` that runs on the `visitor`'s computer. Which one should the visitor download to install and use the program?",
	"rules": []string{
		"Apply these rules in order. An earlier rule always wins over a later rule.",
		"1. `visitor.operating_system` and `visitor.cpu` were resolved by code, and `source` says how. Source `query` means the visitor chose the value explicitly: it overrides `visitor.user_agent` and `visitor.client_hints`. Use the user agent and client hints only for details code did not resolve, such as the Linux distribution.",
		"2. Each option's `runs` says how code found it runs on the visitor's computer. A file that runs `through emulation` still runs: Windows on ARM and Apple Silicon Macs run x86-64 builds. Prefer a native build over an emulated one, and a macOS universal build over a single-CPU Mac build.",
		"3. When `visitor.cpu.value` is unknown, do not assume a CPU. Choose a file only when it runs on every common CPU for that operating system, or when the options have builds for only one CPU.",
		"4. `visitor.requested_format` and `visitor.requested_libc` are explicit requests: choose a file with that format or libc. A static build satisfies a gnu or musl request.",
		"5. Choose the main program, not a companion tool such as a separate CLI, server, editor extension (.vsix) or plugin. `product`, when present, is the repository's own description: use it to tell the main program from companions.",
		"6. Choose the standard build, not a special variant such as baseline, profile, debug, portable, no-web or musl, unless the visitor asked for it. On Linux, glibc (gnu) is the standard build.",
		"7. Prefer the usual installer for the operating system. macOS: .dmg, then .pkg, then an archive. Windows: a setup .exe, then .msi, then .zip. Linux: .AppImage, then .deb (or .rpm on Fedora, RHEL and CentOS), then a .tar.gz archive. A bare binary comes last. The final extension decides the format: `app.AppImage.tar.gz` is an archive, not an AppImage.",
		"8. Choose `" + InsufficientOption + "` only when the CPU is unknown and the options differ by CPU, or when the options are several different programs and none is the main one. Options that differ only by format are never a reason for `" + InsufficientOption + "`: rule 7 decides between them.",
	},
}

// resolved is one code-resolved visitor fact and how it was chosen.
type resolved struct {
	Value  string `json:"value"`
	Source string `json:"source,omitempty"`
}

type visitorInput struct {
	UserAgent       string            `json:"user_agent,omitempty"`
	OS              resolved          `json:"operating_system"`
	CPU             resolved          `json:"cpu"`
	ClientHints     map[string]string `json:"client_hints,omitempty"`
	RequestedFormat string            `json:"requested_format,omitempty"`
	RequestedLibc   string            `json:"requested_libc,omitempty"`
}

type targetInput struct {
	OS   string `json:"os"`
	CPU  string `json:"cpu"`
	Libc string `json:"libc,omitempty"`
}

// fileInput is what Jev sees about one candidate. It doubles as the Choice
// option description and the Noul's `file`, so no index lookups are needed.
type fileInput struct {
	Name         string        `json:"name"`
	Label        string        `json:"label,omitempty"`
	SizeBytes    int64         `json:"size_bytes,omitempty"`
	Format       string        `json:"format,omitempty"`
	Role         string        `json:"role,omitempty"`
	Variants     []string      `json:"variants,omitempty"`
	Runs         string        `json:"runs,omitempty"` // code's fit verdict, Choice options only
	Targets      []targetInput `json:"targets,omitempty"`
	TargetSource string        `json:"target_source"`
	Evidence     []string      `json:"evidence,omitempty"`
}

func describeFile(a github.Asset) fileInput {
	f := picker.FactsForAsset(a)
	in := fileInput{Name: a.Name, Label: a.Label, SizeBytes: a.Size, Format: cmp.Or(f.Extension, "bare binary"), Variants: f.Variants, TargetSource: "filename"}
	if f.Role != "unknown" {
		in.Role = f.Role
	}
	if observedArtifact(a.Artifact) || a.ArchivePlatform != "" {
		in.TargetSource = "file contents or publisher manifest"
	}
	for _, t := range f.Targets {
		in.Targets = append(in.Targets, targetInput{OS: cmp.Or(t.Platform, "unknown"), CPU: cmp.Or(t.Arch, "unknown"), Libc: t.Libc})
	}
	for _, e := range f.Evidence {
		if !strings.HasPrefix(e, "filename") {
			in.Evidence = append(in.Evidence, e)
		}
	}
	return in
}

// fit is code's view of one asset for one visitor: ok is the hard guard,
// the rest only caps confidence. Unknown facts never count as proof.
type fit struct {
	ok         bool
	osProven   bool // a compatible target names the visitor's OS
	archProven bool // a compatible target names the visitor's CPU (or universal)
	emulated   bool // every CPU-specific compatible target needs emulation
	libcProven bool // explicit libc request met by a named libc or static
}

func assess(a github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) fit {
	if a.State != "" && a.State != "uploaded" {
		return fit{}
	}
	f := picker.FactsForAsset(a)
	// Filename-vs-contents disagreement is not a veto: the observed targets in
	// f.Targets already replace the filename's (tigerbeetle-universal-macos.zip).
	conflicts := slices.DeleteFunc(slices.Clone(f.Conflicts), func(c string) bool { return c == picker.ConflictFilenameTargets })
	if f.Source || f.NonNative || f.Role == "debug" || f.Role == "metadata" || len(conflicts) > 0 ||
		(a.Artifact != nil && a.Artifact.Status == "failed") {
		return fit{}
	}
	// Unknown formats (checksums.txt, README) are not downloads; .bin is a common bare-binary suffix.
	if f.Extension == "" && !picker.NativeBinaryName(f.Canonical) && !strings.HasSuffix(f.Canonical, ".bin") {
		return fit{}
	}
	if len(f.Targets) == 0 {
		if platform != picker.Unknown && (picker.IncompatiblePlatform(f, platform) ||
			f.FormatPlatform != "" && f.FormatPlatform != platform) {
			return fit{}
		}
		return fit{ok: true, libcProven: opts.Libc == picker.LibcAny}
	}
	r := fit{libcProven: opts.Libc == picker.LibcAny, emulated: arch != picker.UnknownArch}
	for _, t := range f.Targets {
		if t.Platform != "" && platform != picker.Unknown && t.Platform != string(platform) {
			continue
		}
		tArch := picker.Arch(t.Arch)
		_, emu := picker.EmulationRank(platform, arch, tArch)
		unproven := tArch == picker.UnknownArch || tArch == picker.ArchUnproven
		if arch != picker.UnknownArch && !unproven && tArch != arch && !emu {
			continue
		}
		if opts.Libc != picker.LibcAny && t.Libc != "" && t.Libc != string(opts.Libc) && t.Libc != string(picker.LibcStatic) {
			continue
		}
		r.ok = true
		r.osProven = r.osProven || (platform != picker.Unknown && t.Platform == string(platform))
		r.archProven = r.archProven || (arch != picker.UnknownArch && tArch == arch)
		r.emulated = r.emulated && emu
		r.libcProven = r.libcProven || (t.Libc != "" && (t.Libc == string(opts.Libc) || t.Libc == string(picker.LibcStatic)))
	}
	r.emulated = r.ok && r.emulated
	return r
}

// ValidateSelection is the code guard every served Jev pick passes: a real
// uploaded program with no conflicting evidence and no observed OS, CPU or
// explicit-libc incompatibility. Unknown facts pass; they only lower confidence.
func ValidateSelection(asset github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) bool {
	return assess(asset, platform, arch, opts).ok && !picker.NeedsVerification(picker.FactsForAsset(asset), platform, arch, opts.Libc)
}

// prepared is one decision's visitor-side inputs: the guarded candidates and
// every describable release file, both in canonical (name) order, plus the
// visitor state, or an answer needing no call.
type prepared struct {
	repo        string
	state       map[string]any
	candidates  []github.Asset
	describable []github.Asset
	immediate   *picker.AssetDecision
}

func (c *Client) prepare(repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (prepared, error) {
	if _, ok := osNames[platform]; !ok {
		return prepared{immediate: &picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow, Reasons: []string{"jev: operating system unknown"}}}, nil
	}
	var candidates []github.Asset
	unverified := false
	for _, a := range assets {
		if ValidateSelection(a, platform, arch, opts) {
			candidates = append(candidates, a)
		} else if picker.NeedsVerification(picker.FactsForAsset(a), platform, arch, opts.Libc) {
			unverified = true
		}
	}
	if len(candidates) == 0 {
		if unverified {
			return prepared{immediate: &picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow, Reasons: []string{"jev: compatible executable headers have not been verified"}}}, nil
		}
		return prepared{immediate: &picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow, Reasons: []string{"jev: no installable asset fits this visitor"}}}, nil
	}
	// A proven runtime requirement outranks packaging preference.
	if opts.Libc != picker.LibcAny && opts.Libc != picker.LibcGNU {
		proven := slices.DeleteFunc(slices.Clone(candidates), func(a github.Asset) bool {
			return !assess(a, platform, arch, opts).libcProven
		})
		if len(proven) > 0 {
			candidates = proven
		}
	}
	// An explicit ?prefer= format is a filter, not a hint, whenever some
	// compatible file has it (same FormatRank policy as the rule picker).
	// ponytail: can narrow to a companion's .deb when only it has the format.
	if opts.Prefer != "" {
		preferred := slices.DeleteFunc(slices.Clone(candidates), func(a github.Asset) bool {
			return picker.FormatRank(a.Name, platform, opts) != -1
		})
		if len(preferred) > 0 {
			candidates = preferred
		}
	}
	if len(candidates) > maxCandidates {
		return prepared{}, fmt.Errorf("jev: %d candidates exceeds limit %d", len(candidates), maxCandidates)
	}
	for _, a := range candidates {
		if a.Name == InsufficientOption {
			return prepared{}, fmt.Errorf("jev: asset name %q collides with an outcome option", a.Name)
		}
	}
	// Sorted so requests, and so cache keys, do not depend on GitHub's order.
	byName := func(x, y github.Asset) int { return strings.Compare(x.Name, y.Name) }
	slices.SortFunc(candidates, byName)

	visitor := visitorInput{
		UserAgent:   uaVersion.ReplaceAllString(opts.UserAgent, ""),
		OS:          resolved{Value: osNames[platform], Source: string(opts.PlatformSource)},
		CPU:         resolved{Value: cmp.Or(cpuNames[arch], "unknown"), Source: string(opts.ArchSource)},
		ClientHints: coarseHints(opts.ClientHints),
	}
	if opts.Prefer != "" {
		visitor.RequestedFormat = "." + opts.Prefer
	}
	if opts.Libc != picker.LibcAny {
		visitor.RequestedLibc = string(opts.Libc)
	}
	state := map[string]any{"repository": repo, "visitor": visitor}
	if opts.ProductDescription != "" {
		state["product"] = opts.ProductDescription
	}
	return prepared{repo: repo, state: state, candidates: candidates, describable: describableAssets(assets)}, nil
}

// describableAssets is every real program in the release, whatever the visitor,
// in name order. Describe sees this list, so its answer is shared by every pick
// of a release and can be warmed before the first one (Primary.Warm).
func describableAssets(assets []github.Asset) []github.Asset {
	var out []github.Asset
	for _, a := range assets {
		if assess(a, picker.Unknown, picker.UnknownArch, picker.PickOpts{}).ok {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(x, y github.Asset) int { return strings.Compare(x.Name, y.Name) })
	return out
}

// uaVersion matches browser/engine versions ("Chrome/141.0", "rv:130.0"). They
// never change which file fits but would split the cache per browser release;
// OS and distro tokens ("Windows NT 10.0", "Fedora", "Mac OS X") are kept.
var uaVersion = regexp.MustCompile(`(?:/|rv:)\d[\d._]*`)

// coarseHints keeps the platform_version major only (Windows 10 vs 11, macOS
// release) for the same cache reason.
func coarseHints(hints map[string]string) map[string]string {
	if v, ok := hints["platform_version"]; ok {
		hints = maps.Clone(hints)
		hints["platform_version"], _, _ = strings.Cut(strings.Trim(v, `"`), ".")
	}
	return hints
}

// Decide picks the release file for one visitor context (see the steps at the
// top of this file). No-match and "not enough information" are successful
// abstentions; err is set only for service failures and malformed answers.
func (c *Client) Decide(ctx context.Context, repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, Usage, error) {
	p, err := c.prepare(repo, assets, platform, arch, opts)
	if err != nil || p.immediate != nil {
		return deref(p.immediate), Usage{Model: c.model}, err
	}
	return c.run(ctx, p, platform, arch, opts)
}

func deref(d *picker.AssetDecision) picker.AssetDecision {
	if d == nil {
		return picker.AssetDecision{}
	}
	return *d
}

// judged is code's fit verdict for one candidate, built from Jev's attributes.
type judged struct {
	asset     github.Asset
	tier      int
	fitP      float64 // probability it runs at that tier
	main      float64 // probability it is the main program's standard build
	universal bool
}

// judge turns one file's attributes into a fit tier for this visitor.
// Proven OS/CPU (filename tokens, inspected targets) override Jev's guesses,
// and an OS that only Jev inferred must clear inferredOSThreshold to count.
func judge(attrs FileAttrs, a github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) judged {
	observed := false
	// What code proves from the filename or contents replaces Jev's guess
	// (Jev read bun-linux-aarch64-android.zip as Linux).
	if f := assess(a, platform, arch, opts); f.osProven {
		attrs.OS = map[string]float64{platformOS[platform]: 1}
		if f.archProven {
			for key, cpu := range cpuArch {
				if cpu == arch {
					attrs.CPU = map[string]float64{key: 1}
				}
			}
		}
		observed = true
	}
	switch {
	case observedArtifact(a.Artifact):
		payload := slices.ContainsFunc(a.Artifact.Evidence, func(e github.ArtifactEvidence) bool { return e.Source == "deep-inspection" })
		attrs = withObservedTargets(attrs, picker.CorrectedTargets(a.Name, *a.Artifact, payload))
	case appBundle(a.Name):
		// Code knows this one: Jev reads Tauri's Name_x64.app.tar.gz as Linux.
		attrs.OS, observed = map[string]float64{osMac: 1}, true
	}
	native, emulated, maybe := runProbability(attrs, platform, arch)
	if f := picker.FactsForAsset(github.Asset{Name: a.Name}); !observed && len(f.Platforms) == 0 && f.FormatPlatform == "" &&
		attrs.OS[platformOS[platform]] < inferredOSThreshold {
		native, emulated, maybe = 0, 0, native+emulated+maybe
	}
	j := judged{asset: a, main: attrs.Main, universal: platform == picker.MacOS && attrs.CPU[cpuUniversal] >= 0.5}
	switch {
	case native >= 0.5:
		j.tier, j.fitP = fitNative, native
	case native+emulated >= 0.5:
		j.tier, j.fitP = fitEmulated, native+emulated
	case native+emulated+maybe >= 0.5:
		j.tier, j.fitP = fitMaybe, native+emulated+maybe
	default:
		j.tier, j.fitP = fitNone, native+emulated+maybe
	}
	return j
}

// runnable keeps the best fit tier, then the main builds and Mac universal
// builds when any exist: the policy steps code can decide on its own.
func runnable(all []judged) []judged {
	tier := fitNone
	for _, j := range all {
		tier = max(tier, j.tier)
	}
	if tier == fitNone {
		return nil
	}
	pool := slices.DeleteFunc(slices.Clone(all), func(j judged) bool { return j.tier != tier })
	for _, keep := range []func(judged) bool{
		func(j judged) bool { return j.main >= 0.5 },
		func(j judged) bool { return j.universal },
	} {
		if kept := slices.DeleteFunc(slices.Clone(pool), func(j judged) bool { return !keep(j) }); len(kept) > 0 {
			pool = kept
		}
	}
	return pool
}

func (j judged) String() string {
	return fmt.Sprintf("%q fit=%s p=%.2f main=%.2f", j.asset.Name, tierNames[j.tier], j.fitP, j.main)
}

func (c *Client) run(ctx context.Context, p prepared, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, Usage, error) {
	product, _ := p.state["product"].(string)
	attrs, usage, err := c.Describe(ctx, p.repo, product, p.describable)
	if err != nil {
		return picker.AssetDecision{}, Usage{}, err
	}
	byName := make(map[string]FileAttrs, len(attrs))
	for _, a := range attrs {
		byName[a.Name] = a
	}
	all := make([]judged, 0, len(p.candidates))
	for _, a := range p.candidates {
		at, ok := byName[a.Name]
		if !ok {
			return picker.AssetDecision{}, Usage{}, fmt.Errorf("jev: no attributes for candidate %q", a.Name)
		}
		all = append(all, judge(at, a, platform, arch, opts))
	}
	pool := runnable(all)
	if len(pool) == 0 {
		return picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow,
			Reasons: []string{"jev: no file runs on this operating system and CPU"}}, usage, nil
	}

	chosen, pChosen := pool[0], 1.0
	reasons := []string{"jev: only runnable file " + chosen.String()}
	var probs map[string]float64
	if len(pool) > 1 {
		ch, chUsage, err := c.choose(ctx, p, pool)
		if err != nil {
			return picker.AssetDecision{}, Usage{}, err
		}
		usage.InputTokens += chUsage.InputTokens
		probs = ch.Probabilities
		reasons = []string{fmt.Sprintf("jev chose %q p=%.2f confidence=%.2f among %d runnable", ch.Choice, ch.Probabilities[ch.Choice], ch.Confidence, len(pool))}
		if ch.Choice == InsufficientOption {
			return picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow, Reasons: reasons, Alternatives: alternatives(all, "", probs)}, usage, nil
		}
		runnerUp := ch.Probabilities[InsufficientOption]
		for _, j := range pool {
			if j.asset.Name == ch.Choice {
				chosen, pChosen = j, ch.Probabilities[ch.Choice]
			} else {
				runnerUp = max(runnerUp, ch.Probabilities[j.asset.Name])
			}
		}
		if pChosen < clearLead*ch.Probabilities[InsufficientOption] {
			pChosen = 0
			reasons = append(reasons, "not enough information is too close to the chosen file")
		} else if pChosen < chosenMedium && pChosen >= clearLead*runnerUp {
			pChosen = chosenMedium
			reasons = append(reasons, fmt.Sprintf("split vote, clear lead over runner-up p=%.2f", runnerUp))
		}
		reasons = append(reasons, chosen.String())
	}

	asset := chosen.asset
	conf := gate(assess(asset, platform, arch, opts), pChosen, chosen.fitP, chosen.tier, arch, opts, &reasons)
	status := "selected"
	if conf == picker.ConfidenceLow {
		status = "unknown"
	}
	return picker.AssetDecision{
		Status:       status,
		Asset:        &asset,
		Confidence:   conf,
		Reasons:      reasons,
		Alternatives: alternatives(all, asset.Name, probs),
		Facts:        picker.FactsForAsset(asset),
	}, usage, nil
}

// choose asks one Choice among the runnable files, each described by its
// fileInput, plus InsufficientOption, and validates the answer.
func (c *Client) choose(ctx context.Context, p prepared, pool []judged) (answer, Usage, error) {
	criteria := make(orderedCriteria, 0, len(pool)+1)
	options := make([]string, 0, len(pool)+1)
	for _, j := range pool {
		in := describeFile(j.asset)
		in.Runs = map[int]string{fitNative: "natively", fitEmulated: "through emulation", fitMaybe: "possibly"}[j.tier]
		criteria = append(criteria, option{name: j.asset.Name, desc: in})
		options = append(options, j.asset.Name)
	}
	criteria = append(criteria, option{name: InsufficientOption, desc: "Several files may fit, but the visitor or file facts cannot tell which one."})
	options = append(options, InsufficientOption)
	resp, err := c.post(ctx, request{Model: c.model, State: p.state,
		Questions: map[string]question{"choice": {Type: "choice", Instructions: choiceInstructions, Criteria: criteria}}})
	if err != nil {
		return answer{}, Usage{}, err
	}
	ch, ok := resp.Answers["choice"]
	if !ok || ch.Type != "choice" {
		return answer{}, Usage{}, errors.New("jev: malformed response: missing choice answer")
	}
	if err := validChoice(ch, options); err != nil {
		return answer{}, Usage{}, err
	}
	return ch, Usage{Model: resp.Model, InputTokens: resp.Usage.InputTokens}, nil
}

// alternatives lists the other files that may run, most preferred first.
func alternatives(all []judged, chosen string, probs map[string]float64) []picker.RankedAsset {
	var alts []judged
	for _, j := range all {
		if j.asset.Name != chosen && j.tier > fitNone {
			alts = append(alts, j)
		}
	}
	slices.SortStableFunc(alts, func(x, y judged) int {
		return cmp.Or(cmp.Compare(probs[y.asset.Name], probs[x.asset.Name]), cmp.Compare(y.tier, x.tier), cmp.Compare(y.fitP, x.fitP))
	})
	out := make([]picker.RankedAsset, len(alts))
	for i, j := range alts {
		out[i] = picker.RankedAsset{Asset: j.asset, Reasons: []string{j.String()}}
	}
	return out
}

// gate maps Jev's probabilities to confidence, then caps it where code knows
// the evidence is weaker than the model's certainty. When code proves the OS
// and CPU from the filename or contents, that proof replaces Jev's fit.
func gate(f fit, pChosen, pFit float64, tier int, arch picker.Arch, opts picker.PickOpts, reasons *[]string) picker.Confidence {
	if f.osProven && (f.archProven || f.emulated) {
		pFit = 1
	}
	conf := picker.ConfidenceLow
	switch {
	case pChosen >= chosenHigh && pFit >= fitHigh:
		conf = picker.ConfidenceHigh
	case pChosen >= chosenMedium && pFit >= fitMedium:
		conf = picker.ConfidenceMedium
	}
	limit := func(ceiling picker.Confidence, why string) {
		if conf == picker.ConfidenceHigh || (conf == picker.ConfidenceMedium && ceiling == picker.ConfidenceLow) {
			conf = ceiling
			*reasons = append(*reasons, why)
		}
	}
	switch tier {
	case fitMaybe:
		limit(picker.ConfidenceLow, "may not run on this CPU")
	case fitEmulated:
		limit(picker.ConfidenceMedium, "runs under emulation")
	}
	if !f.osProven {
		limit(picker.ConfidenceMedium, "operating system inferred by jev")
	}
	// Past the guard an unproven libc means unmarked. Unmarked Linux builds are
	// glibc by convention, so a gnu request keeps medium; a musl request does not.
	switch {
	case f.libcProven:
	case opts.Libc == picker.LibcGNU:
		limit(picker.ConfidenceMedium, "unmarked build assumed glibc")
	default:
		limit(picker.ConfidenceLow, "requested libc not proven")
	}
	switch {
	case arch == picker.UnknownArch:
		limit(picker.ConfidenceMedium, "visitor CPU unknown")
	case f.emulated:
		limit(picker.ConfidenceMedium, "runs under emulation")
	case !f.archProven:
		limit(picker.ConfidenceMedium, "CPU not proven by the filename or contents")
	}
	return conf
}

// validChoice checks a Choice answer against the options we sent: every
// option has a finite probability in [0, 1], they sum to 1, nothing extra is
// returned, and the choice is the most probable option. Probabilities arrive
// rounded to 2 decimals, so a near-tie choice (0.45 vs 0.46) is still valid.
func validChoice(a answer, options []string) error {
	if a.Type != "choice" || !unit(a.Confidence) {
		return errors.New("jev: malformed response: invalid choice type or confidence")
	}
	if len(a.Probabilities) != len(options) {
		return fmt.Errorf("jev: malformed response: %d probabilities for %d options", len(a.Probabilities), len(options))
	}
	sum, top := 0.0, 0.0
	for _, o := range options {
		p, ok := a.Probabilities[o]
		if !ok || !unit(p) {
			return fmt.Errorf("jev: malformed response: bad probability for %q", o)
		}
		sum += p
		top = max(top, p)
	}
	p, ok := a.Probabilities[a.Choice]
	switch {
	case math.Abs(sum-1) > 0.02:
		return fmt.Errorf("jev: malformed response: probabilities sum to %.3f", sum)
	case !ok:
		return fmt.Errorf("jev: malformed response: choice %q was not offered", a.Choice)
	case p < top-0.02:
		return fmt.Errorf("jev: malformed response: choice %q is not the most probable option", a.Choice)
	}
	return nil
}

func unit(p float64) bool { return !math.IsNaN(p) && p >= 0 && p <= 1 }
