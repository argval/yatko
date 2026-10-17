// Package jev asks TypeSafe's Jev model (a System One decision model) to choose
// the release asset for a visitor. Code removes files that can never be
// installed; Jev ranks the real files that remain. It never invents a URL.
// Docs: https://docs.typesafe.ai/primitives/choice.md
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

const (
	defaultEndpoint = "https://api.typesafe.ai/v1/systemone"
	// Model is pinned, not "jev-latest": confidence thresholds are tuned
	// against one version, and an alias can move under us.
	Model = "jev-1.13.0"
	// NoneOption lets Jev say no file fits instead of forcing a guess.
	NoneOption = "none of these files"
	// maxOptions is the API's Choice limit (255) minus NoneOption.
	maxOptions = 254
)

// Confidence thresholds on the averaged probability of the chosen file.
// ponytail: tuned by hand on shared/picker/corpus.json; retune when the corpus grows.
const (
	highProbability   = 0.8
	mediumProbability = 0.5
)

// Client calls the TypeSafe System One endpoint.
type Client struct {
	apiKey   string
	endpoint string
	http     *http.Client
}

// NewClientFromEnv returns nil when no API key is set, so callers can treat
// Jev as optional. TYPESAFE_API_KEY is the SDK name; TYPESAFE_API is accepted too.
func NewClientFromEnv() *Client {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		key = os.Getenv("TYPESAFE_API")
	}
	if key == "" {
		return nil
	}
	return &Client{apiKey: key, endpoint: defaultEndpoint, http: &http.Client{Timeout: 10 * time.Second}}
}

// Usage is what one decision cost, for eval and logging.
type Usage struct {
	Model       string
	InputTokens int
	// Consistent is false when DecideChoice's forward and reversed option
	// orders chose different files. Decide judges each file alone, so it is always true.
	Consistent bool
}

// option is one Choice option; criteria must keep this order on the wire.
type option struct {
	name string
	desc any
}

// orderedCriteria marshals as a JSON object in slice order. encoding/json
// sorts map keys, which would destroy the forward/reverse order we rely on.
type orderedCriteria []option

func (o orderedCriteria) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, opt := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(opt.name)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(opt.desc)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

type question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]question `json:"questions"`
}

type answer struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
	Noul          float64            `json:"noul"`
}

type response struct {
	Model   string            `json:"model"`
	Answers map[string]answer `json:"answers"`
	Usage   struct {
		InputTokens int `json:"input_tokens"`
	} `json:"usage"`
}

// choiceInstructions tell Jev Yatko's download policy for DecideChoice. Matching filenames to an OS
// and CPU is Jev's judgment; which kind of installer we prefer is product policy.
var choiceInstructions = map[string]any{
	"question": "Which file from this GitHub release should the `visitor` download to install and run the program on their computer?",
	"rules": []string{
		"Apply these rules in order. An earlier rule always wins over a later rule.",
		"1. The file must be built for the visitor's operating system. A file built only for another operating system is wrong.",
		"2. The file must run on the visitor's CPU. Prefer a native build for that CPU, even when it is only a .zip and another CPU has an installer. A macOS universal build runs on both Intel and Apple Silicon. Use an x86-64 build on an arm64 Mac or arm64 Windows PC only when no native or universal build exists. A 32-bit x86 (386, i686) build never runs on an ARM CPU.",
		"3. When the visitor's CPU is unknown, choose the build most computers with that operating system run: x86-64, or a universal build on macOS.",
		"4. When `visitor.preferred_format` or `visitor.libc` is given, choose a file with that format or libc if one exists.",
		"5. Choose the main program, not a companion tool such as a separate CLI, server, editor extension (.vsix) or plugin, when the release has both.",
		"6. Choose the standard build, not a special variant such as baseline, profile, debug, symbols, portable, no-web or musl, unless the visitor asked for it. On Linux, glibc (gnu) is the standard build.",
		"7. Prefer the usual installer for the operating system. macOS: .dmg, then .pkg, then an archive. Windows: a setup .exe, then .msi, then .zip. Linux: a file ending in .AppImage, then .deb (or .rpm on Fedora, RHEL and CentOS), then a .tar.gz archive. An AppImage packed inside an archive counts as an archive. A bare binary with no file extension comes last.",
		"8. Choose `" + NoneOption + "` when no file is built for the visitor's operating system and CPU.",
	},
}

var osNames = map[picker.Platform]string{
	picker.MacOS:   "macOS",
	picker.Windows: "Windows",
	picker.Linux:   "Linux",
	picker.Android: "Android",
	picker.IOS:     "iOS",
}

var cpuNames = map[picker.Arch]string{
	picker.AMD64: "64-bit Intel or AMD (filenames say x86_64, x64, amd64)",
	picker.ARM64: "64-bit ARM, e.g. Apple Silicon (filenames say arm64, aarch64)",
	picker.ARM:   "32-bit ARM (filenames say arm, armv6, armv7, armhf, armel)",
	picker.X86:   "32-bit Intel or AMD (filenames say 386, i386, i686, x86, win32)",
}

// visitorState is the small, named state Jev judges. Code resolves OS and
// CPU from the User-Agent first: Mac browsers report "Intel" even on Apple
// Silicon, so the raw UA is sent only on Linux, where it names the distro.
func visitorState(platform picker.Platform, arch picker.Arch, opts picker.PickOpts) map[string]string {
	v := map[string]string{"operating_system": osNames[platform], "cpu": "unknown"}
	if name, ok := cpuNames[arch]; ok {
		v["cpu"] = name
	}
	if opts.Prefer != "" {
		v["preferred_format"] = "." + opts.Prefer
	}
	if opts.Libc != picker.LibcAny {
		v["libc"] = string(opts.Libc)
	}
	if platform == picker.Linux && opts.UserAgent != "" {
		v["user_agent"] = opts.UserAgent
	}
	return v
}

// DecideChoice asks one Choice over every file (the first experiment). It
// matches OS/CPU well but mixes in format preference, which Jev handles poorly;
// Decide (split.go) is the default. Kept so the eval can compare both.
func (c *Client) DecideChoice(ctx context.Context, repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, Usage, error) {
	if _, ok := osNames[platform]; !ok {
		return picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow, Reasons: []string{"unknown platform"}}, Usage{}, nil
	}
	candidates := picker.Installable(assets)
	if len(candidates) == 0 {
		return picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow, Reasons: []string{"no installable assets"}}, Usage{}, nil
	}
	if len(candidates) > maxOptions {
		return picker.AssetDecision{}, Usage{}, fmt.Errorf("jev: %d candidates exceeds Choice limit", len(candidates))
	}

	// Ask the same Choice in forward and reversed order in one request.
	// Jev 1.13 can lean toward the first option; averaging cancels that out.
	forward := make(orderedCriteria, 0, len(candidates)+1)
	for _, a := range candidates {
		forward = append(forward, option{name: a.Name})
	}
	reverse := slices.Clone(forward)
	slices.Reverse(reverse)
	forward = append(forward, option{name: NoneOption, desc: "No file fits this visitor's operating system and CPU."})
	reverse = append(reverse, forward[len(forward)-1])

	req := request{
		Model: Model,
		State: map[string]any{"repository": repo, "visitor": visitorState(platform, arch, opts)},
		Questions: map[string]question{
			"forward": {Type: "choice", Instructions: choiceInstructions, Criteria: forward},
			"reverse": {Type: "choice", Instructions: choiceInstructions, Criteria: reverse},
		},
	}
	resp, err := c.post(ctx, req)
	if err != nil {
		return picker.AssetDecision{}, Usage{}, err
	}
	fwd, okF := resp.Answers["forward"]
	rev, okR := resp.Answers["reverse"]
	if !okF || !okR {
		return picker.AssetDecision{}, Usage{}, errors.New("jev: response missing answers")
	}
	usage := Usage{Model: resp.Model, InputTokens: resp.Usage.InputTokens, Consistent: fwd.Choice == rev.Choice}
	return decisionFrom(candidates, fwd, rev, usage.Consistent), usage, nil
}

// decisionFrom averages both orderings and maps the top probability onto
// high/medium/low. An order-dependent answer is never auto-selected as high.
func decisionFrom(candidates []github.Asset, fwd, rev answer, consistent bool) picker.AssetDecision {
	prob := func(name string) float64 { return (fwd.Probabilities[name] + rev.Probabilities[name]) / 2 }

	best, bestP := NoneOption, prob(NoneOption)
	var bestAsset *github.Asset
	for i := range candidates {
		// Strict > keeps ties on the first asset in release order: deterministic.
		if p := prob(candidates[i].Name); p > bestP {
			best, bestP, bestAsset = candidates[i].Name, p, &candidates[i]
		}
	}
	reasons := []string{fmt.Sprintf("jev chose %q p=%.2f (forward=%q reverse=%q)", best, bestP, fwd.Choice, rev.Choice)}
	if bestAsset == nil {
		return picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow, Reasons: reasons}
	}

	conf := picker.ConfidenceLow
	switch {
	case bestP >= highProbability && consistent:
		conf = picker.ConfidenceHigh
	case bestP >= mediumProbability:
		conf = picker.ConfidenceMedium
	}

	var alts []picker.RankedAsset
	for _, a := range candidates {
		if p := prob(a.Name); a.Name != best && p >= 0.05 {
			alts = append(alts, picker.RankedAsset{Asset: a, Reasons: []string{fmt.Sprintf("jev p=%.2f", p)}})
		}
	}
	slices.SortStableFunc(alts, func(x, y picker.RankedAsset) int {
		px, py := prob(x.Asset.Name), prob(y.Asset.Name)
		switch {
		case px > py:
			return -1
		case px < py:
			return 1
		}
		return 0
	})

	return picker.AssetDecision{
		Status:       "selected",
		Asset:        bestAsset,
		Confidence:   conf,
		Reasons:      reasons,
		Alternatives: alts,
		Facts:        picker.FactsForAsset(*bestAsset),
	}
}

// post sends one request, retrying 429/529 with exponential backoff as the
// API docs ask (https://docs.typesafe.ai/api.md#handling-rate-limits).
func (c *Client) post(ctx context.Context, req request) (*response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	backoff := 500 * time.Millisecond
	for attempt := 0; ; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")
		res, err := c.http.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("jev: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("jev: read body: %w", err)
		}
		if (res.StatusCode == http.StatusTooManyRequests || res.StatusCode == 529) && attempt < 3 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			continue
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("jev: HTTP %d: %s", res.StatusCode, bytes.TrimSpace(data))
		}
		var out response
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("jev: decode: %w", err)
		}
		return &out, nil
	}
}
