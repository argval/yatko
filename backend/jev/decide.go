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

// Wire schema of the direct decision, one request per visitor context:
//   - "choice": Choice whose options are the guarded candidates' real asset
//     names (sorted), each described by its fileInput, plus NoCompatibleOption
//     and InsufficientOption.
//   - "runs_<i>": Noul per candidate i (same sorted order): does it run here?
//
// promptVersion is hashed into Primary's cache key; bump it whenever the
// questions, state shape or decision policy below change.
const promptVersion = "direct-v4"

const (
	// NoCompatibleOption: no candidate is built for this OS and CPU.
	NoCompatibleOption = NoneOption
	// InsufficientOption: a file may fit, but the facts cannot tell which.
	InsufficientOption = "not enough information"
	// maxCandidates bounds one request: one Noul per file plus the Choice
	// criteria must fit Jev's per-request context. Above it Decide errors and
	// the rule picker answers.
	maxCandidates = 100
)

// Thresholds that turn Jev's answers into confidence.
// ponytail: hand-set; retune on the paid corpus eval and primary disagreement logs.
const (
	// chosenMedium/chosenHigh: Choice probability of the chosen file.
	chosenMedium = 0.5
	chosenHigh   = 0.8
	// runsMedium/runsHigh: the chosen file's own "runs here" Noul. Choice
	// concentration alone is not safety, so both must pass.
	runsMedium = 0.7
	runsHigh   = 0.9
	// alternativeRuns lists a runner-up only when Jev thinks it may run.
	alternativeRuns = 0.5
)

var directInstructions = map[string]any{
	"question": "Which file from this release of `repository` should the `visitor` download to install and run the program on their computer?",
	"rules": []string{
		"Apply these rules in order. An earlier rule always wins over a later rule.",
		"1. `visitor.operating_system` and `visitor.cpu` were resolved by code, and `source` says how. Source `query` means the visitor chose the value explicitly: it overrides `visitor.user_agent` and `visitor.client_hints`. Use the user agent and client hints only for details code did not resolve, such as the Linux distribution.",
		"2. The file must be built for the visitor's operating system. A file whose `targets` name only another operating system never fits.",
		"3. The file must run on the visitor's CPU. Prefer a native build for that CPU. A macOS universal build runs on both Intel and Apple Silicon. Use an x86-64 build on an arm64 Mac or arm64 Windows PC only when no native or universal build exists. A 32-bit x86 build never runs on an ARM CPU.",
		"4. When `visitor.cpu.value` is unknown, do not assume a CPU. Choose a file only when it runs on every common CPU for that operating system, or when the release has builds for only one CPU on that operating system.",
		"5. `visitor.requested_format` and `visitor.requested_libc` are explicit requests: choose a file with that format or libc when one fits. A static build satisfies a gnu or musl request. A file that does not name its libc has not proven one.",
		"6. Choose the main program, not a companion tool such as a separate CLI, server, editor extension (.vsix) or plugin, when the release has both.",
		"7. Choose the standard build, not a special variant such as baseline, profile, debug, portable, no-web or musl, unless the visitor asked for it. On Linux, glibc (gnu) is the standard build.",
		"8. Prefer the usual installer for the operating system. macOS: .dmg, then .pkg, then an archive. Windows: a setup .exe, then .msi, then .zip. Linux: .AppImage, then .deb (or .rpm on Fedora, RHEL and CentOS), then a .tar.gz archive. A bare binary comes last. The final extension decides the format: `app.AppImage.tar.gz` is an archive, not an AppImage.",
		"9. Choose `" + NoCompatibleOption + "` when no file is built for the visitor's operating system and CPU. Choose `" + InsufficientOption + "` only when the CPU is unknown and the builds differ by CPU, or when the release ships several different programs and none is the main one. Files that differ only by format (.deb, .rpm, .AppImage, archive) are never a reason for `" + InsufficientOption + "`: rule 8 decides between them.",
	},
}

const runsQuestion = "Can the `visitor` install and run `file` on their computer as it is? Answer no when `file` is for another operating system, " +
	"for a CPU their computer cannot run even through emulation, for a libc other than `visitor.requested_libc`, or is not a program " +
	"(source code, checksums, signatures, documentation). When `visitor.cpu.value` is unknown, answer yes only when `file` runs on every common CPU for that operating system."

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
	Targets      []targetInput `json:"targets,omitempty"`
	TargetSource string        `json:"target_source"`
	Evidence     []string      `json:"evidence,omitempty"`
}

func describeFile(a github.Asset) fileInput {
	f := picker.FactsForAsset(a)
	in := fileInput{Name: a.Name, Label: a.Label, SizeBytes: a.Size, Format: f.Extension, Variants: f.Variants, TargetSource: "filename"}
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
	if f.Source || f.NonNative || f.Role == "debug" || f.Role == "metadata" || len(f.Conflicts) > 0 ||
		(a.Artifact != nil && a.Artifact.Status == "failed") {
		return fit{}
	}
	// Unknown formats (checksums.txt, README) are not downloads; .bin is a common bare-binary suffix.
	if f.Extension == "" && !picker.NativeBinaryName(f.Canonical) && !strings.HasSuffix(f.Canonical, ".bin") {
		return fit{}
	}
	if len(f.Targets) == 0 {
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
	return assess(asset, platform, arch, opts).ok
}

// prepared is one ready-to-send decision: the guarded candidates in canonical
// (name) order and the request built from them, or an answer needing no call.
type prepared struct {
	req        request
	candidates []github.Asset
	immediate  *picker.AssetDecision
}

func (c *Client) prepare(repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (prepared, error) {
	if _, ok := osNames[platform]; !ok {
		return prepared{immediate: &picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow, Reasons: []string{"jev: operating system unknown"}}}, nil
	}
	var candidates []github.Asset
	for _, a := range assets {
		if ValidateSelection(a, platform, arch, opts) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return prepared{immediate: &picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow, Reasons: []string{"jev: no installable asset fits this visitor"}}}, nil
	}
	if len(candidates) > maxCandidates {
		return prepared{}, fmt.Errorf("jev: %d candidates exceeds limit %d", len(candidates), maxCandidates)
	}
	// Sorted so the request, and so the cache key, do not depend on GitHub's order.
	slices.SortFunc(candidates, func(x, y github.Asset) int { return strings.Compare(x.Name, y.Name) })

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
	names := make([]string, len(candidates))
	criteria := make(orderedCriteria, 0, len(candidates)+2)
	questions := make(map[string]question, len(candidates)+1)
	for i, a := range candidates {
		if a.Name == NoCompatibleOption || a.Name == InsufficientOption {
			return prepared{}, fmt.Errorf("jev: asset name %q collides with an outcome option", a.Name)
		}
		names[i] = a.Name
		in := describeFile(a)
		criteria = append(criteria, option{name: a.Name, desc: in})
		questions[fmt.Sprintf("runs_%d", i)] = question{Type: "noul", Instructions: map[string]any{"file": in, "question": runsQuestion}}
	}
	state["release_files"] = names
	criteria = append(criteria,
		option{name: NoCompatibleOption, desc: "No file is built for the visitor's operating system and CPU."},
		option{name: InsufficientOption, desc: "Some file may fit, but the visitor or file facts cannot tell which one."})
	questions["choice"] = question{Type: "choice", Instructions: directInstructions, Criteria: criteria}
	return prepared{candidates: candidates, req: request{Model: c.model, State: state, Questions: questions}}, nil
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

// Decide asks Jev to choose directly among the release's real files for one
// visitor context. No-match and "not enough information" are successful
// abstentions; err is set only for service failures and malformed answers.
func (c *Client) Decide(ctx context.Context, repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, Usage, error) {
	p, err := c.prepare(repo, assets, platform, arch, opts)
	if err != nil || p.immediate != nil {
		return deref(p.immediate), Usage{Model: c.model, Consistent: true}, err
	}
	return c.run(ctx, p, platform, arch, opts)
}

func deref(d *picker.AssetDecision) picker.AssetDecision {
	if d == nil {
		return picker.AssetDecision{}
	}
	return *d
}

func (c *Client) run(ctx context.Context, p prepared, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, Usage, error) {
	resp, err := c.post(ctx, p.req)
	if err != nil {
		return picker.AssetDecision{}, Usage{}, err
	}
	usage := Usage{Model: resp.Model, InputTokens: resp.Usage.InputTokens, Consistent: true}
	d, err := interpret(resp, p, platform, arch, opts)
	return d, usage, err
}

// interpret validates the response shape and applies the code-side gates.
func interpret(resp *response, p prepared, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, error) {
	ch, ok := resp.Answers["choice"]
	if !ok || ch.Type != "choice" {
		return picker.AssetDecision{}, errors.New("jev: malformed response: missing choice answer")
	}
	options := make([]string, 0, len(p.candidates)+2)
	for _, a := range p.candidates {
		options = append(options, a.Name)
	}
	options = append(options, NoCompatibleOption, InsufficientOption)
	if err := validChoice(ch, options); err != nil {
		return picker.AssetDecision{}, err
	}
	runs := make([]float64, len(p.candidates))
	for i := range p.candidates {
		a, ok := resp.Answers[fmt.Sprintf("runs_%d", i)]
		if !ok || a.Type != "noul" || a.Noul == nil || !unit(*a.Noul) {
			return picker.AssetDecision{}, fmt.Errorf("jev: malformed response: bad runs_%d answer", i)
		}
		runs[i] = *a.Noul
	}

	chosen := -1
	var alts []picker.RankedAsset
	for i, a := range p.candidates {
		if a.Name == ch.Choice {
			chosen = i
		} else if runs[i] >= alternativeRuns {
			alts = append(alts, picker.RankedAsset{Asset: a, Reasons: []string{fmt.Sprintf("jev p=%.2f runs=%.2f", ch.Probabilities[a.Name], runs[i])}})
		}
	}
	slices.SortStableFunc(alts, func(x, y picker.RankedAsset) int {
		return cmp.Compare(ch.Probabilities[y.Asset.Name], ch.Probabilities[x.Asset.Name])
	})
	reasons := []string{fmt.Sprintf("jev chose %q p=%.2f confidence=%.2f", ch.Choice, ch.Probabilities[ch.Choice], ch.Confidence)}
	switch ch.Choice {
	case NoCompatibleOption:
		return picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow, Reasons: reasons, Alternatives: alts}, nil
	case InsufficientOption:
		return picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow, Reasons: reasons, Alternatives: alts}, nil
	}

	asset := p.candidates[chosen]
	pChosen, pRuns := ch.Probabilities[asset.Name], runs[chosen]
	reasons[0] += fmt.Sprintf(" runs=%.2f", pRuns)
	conf := gate(assess(asset, platform, arch, opts), pChosen, pRuns, arch, opts, &reasons)
	status := "selected"
	if conf == picker.ConfidenceLow {
		status = "unknown"
	}
	return picker.AssetDecision{
		Status:       status,
		Asset:        &asset,
		Confidence:   conf,
		Reasons:      reasons,
		Alternatives: alts,
		Facts:        picker.FactsForAsset(asset),
	}, nil
}

// gate maps Jev's probabilities to confidence, then caps it where code knows
// the evidence is weaker than the model's certainty. The "runs" Noul only
// gates picks whose OS or CPU code cannot prove: on proven files it is noisier
// than the targets themselves (eval: correct native picks scored 0.5-0.7).
func gate(f fit, pChosen, pRuns float64, arch picker.Arch, opts picker.PickOpts, reasons *[]string) picker.Confidence {
	if f.osProven && (f.archProven || f.emulated) {
		pRuns = 1
	}
	conf := picker.ConfidenceLow
	switch {
	case pChosen >= chosenHigh && pRuns >= runsHigh:
		conf = picker.ConfidenceHigh
	case pChosen >= chosenMedium && pRuns >= runsMedium:
		conf = picker.ConfidenceMedium
	}
	limit := func(ceiling picker.Confidence, why string) {
		if conf == picker.ConfidenceHigh || (conf == picker.ConfidenceMedium && ceiling == picker.ConfidenceLow) {
			conf = ceiling
			*reasons = append(*reasons, why)
		}
	}
	if !f.osProven && pRuns < runsHigh {
		// The OS is Jev's inference from context (an unmarked .zip): demand a confident "runs".
		limit(picker.ConfidenceLow, "operating system not proven by the filename or contents")
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
