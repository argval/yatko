package jev

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

func assetsNamed(names ...string) []github.Asset {
	out := make([]github.Asset, len(names))
	for i, n := range names {
		out[i] = github.Asset{Name: n, State: "uploaded", BrowserDownloadURL: "https://github.com/o/r/releases/download/v1/" + n}
		if strings.HasSuffix(strings.ToLower(n), ".appimage") {
			f := picker.Classify(n)
			if len(f.Arches) == 1 {
				out[i].Artifact = &github.ArtifactMetadata{Status: "inspected", LauncherArch: string(f.Arches[0])}
			}
		}
	}
	return out
}

var linuxPair = assetsNamed("tool-linux-amd64.AppImage", "tool-linux-amd64.tar.gz")

// When several files run, Jev's Choice among them wins over the rule picker's
// format ranking, and the asset is the caller's own.
func TestDecideChoiceAmongRunnable(t *testing.T) {
	rules := picker.DecideAsset(linuxPair, picker.Linux, picker.AMD64, picker.PickOpts{})
	if name(rules) != "tool-linux-amd64.AppImage" {
		t.Fatalf("precondition: rules picked %s", name(rules))
	}
	c, fs := fakeJev(t, picks("tool-linux-amd64.tar.gz", 0.9))
	d, _, err := c.Decide(context.Background(), "o/tool", linuxPair, picker.Linux, picker.AMD64, picker.PickOpts{})
	if err != nil || name(d) != "tool-linux-amd64.tar.gz" || d.Confidence != picker.ConfidenceHigh || d.Status != "selected" {
		t.Fatalf("got %s %s %s err=%v reasons=%v", name(d), d.Status, d.Confidence, err, d.Reasons)
	}
	if d.Asset.BrowserDownloadURL != linuxPair[1].BrowserDownloadURL {
		t.Fatalf("asset URL %q is not the release's", d.Asset.BrowserDownloadURL)
	}
	if len(d.Alternatives) != 1 || d.Alternatives[0].Asset.Name != "tool-linux-amd64.AppImage" {
		t.Fatalf("alternatives = %+v", d.Alternatives)
	}
	if !slices.Equal(fs.lastRequest().options(t), []string{"tool-linux-amd64.AppImage", "tool-linux-amd64.tar.gz", InsufficientOption}) {
		t.Fatalf("options = %v", fs.lastRequest().options(t))
	}
}

// One runnable file needs no Choice; Describe is cached per release, so a
// second visitor context makes no Describe call either.
func TestDecideSingleRunnableSkipsChoice(t *testing.T) {
	c, fs := fakeJev(t, picks("", 0.9))
	mac := assetsNamed("app-arm64.dmg", "app-x64.dmg")
	d, _, err := c.Decide(context.Background(), "o/app", mac, picker.MacOS, picker.ARM64, picker.PickOpts{})
	if err != nil || name(d) != "app-arm64.dmg" || d.Confidence != picker.ConfidenceHigh {
		t.Fatalf("arm64: %s %s err=%v", name(d), d.Confidence, err)
	}
	if d, _, _ := c.Decide(context.Background(), "o/app", mac, picker.MacOS, picker.AMD64, picker.PickOpts{}); name(d) != "app-x64.dmg" {
		t.Fatalf("amd64: %s", name(d))
	}
	if fs.choices.Load() != 0 || fs.describes.Load() != 1 {
		t.Fatalf("describes=%d choices=%d, want 1 and 0", fs.describes.Load(), fs.choices.Load())
	}
}

// Windows on ARM runs x64 builds: with no native build, code selects the
// emulated one (the direct picker answered "none of these files" here).
func TestDecideEmulationWhenNoNativeBuild(t *testing.T) {
	c, _ := fakeJev(t, picks("", 0.9))
	d, _, err := c.Decide(context.Background(), "o/app", assetsNamed("app-windows-x64-setup.exe"), picker.Windows, picker.ARM64, picker.PickOpts{})
	if err != nil || name(d) != "app-windows-x64-setup.exe" || d.Confidence != picker.ConfidenceMedium || !d.ShouldAutoSelect() {
		t.Fatalf("got %s %s err=%v reasons=%v", name(d), d.Confidence, err, d.Reasons)
	}
}

// "Not enough information" and "nothing runs" are answers, not errors.
func TestDecideAbstentions(t *testing.T) {
	c, _ := fakeJev(t, picks(InsufficientOption, 0.9))
	d, _, err := c.Decide(context.Background(), "o/tool", linuxPair, picker.Linux, picker.AMD64, picker.PickOpts{})
	if err != nil || d.Asset != nil || d.Status != "unknown" || d.ShouldAutoSelect() || len(d.Alternatives) != 2 {
		t.Fatalf("insufficient: %+v err=%v", d, err)
	}
	// Jev says the unmarked zip is a macOS build: nothing runs on Windows.
	c, fs := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
		return fakeReply{attrs: func(string) (string, string, float64) { return osMac, cpuARM64, 0.9 }}
	})
	if d, _, err := c.Decide(context.Background(), "o/app", assetsNamed("app.zip"), picker.Windows, picker.AMD64, picker.PickOpts{}); err != nil || d.Status != "incompatible" {
		t.Fatalf("nothing runs: %+v %v", d, err)
	}
	// Code answers without a call when the guard leaves nothing or the OS is unknown.
	before := fs.calls()
	mac := assetsNamed("app-arm64.dmg", "app-x64.dmg")
	if d, _, err := c.Decide(context.Background(), "o/app", mac, picker.Windows, picker.AMD64, picker.PickOpts{}); err != nil || d.Status != "incompatible" {
		t.Fatalf("windows: %+v %v", d, err)
	}
	if d, _, err := c.Decide(context.Background(), "o/app", mac, picker.Unknown, picker.UnknownArch, picker.PickOpts{}); err != nil || d.Status != "unknown" {
		t.Fatalf("unknown OS: %+v %v", d, err)
	}
	if fs.calls() != before {
		t.Fatalf("calls = %d, want %d", fs.calls(), before)
	}
}

// The Choice carries the raw UA, resolved OS/CPU with provenance and the
// explicit requests; files proven for another OS are never offered.
func TestDecideStateCarriesExplicitContext(t *testing.T) {
	c, fs := fakeJev(t, containing(".deb", 0.9))
	opts := picker.PickOpts{
		Libc: picker.LibcGNU, UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
		PlatformSource: "query", ArchSource: "ua", ClientHints: map[string]string{"platform": "Windows"},
	}
	assets := assetsNamed("app-setup-x64.exe", "app_amd64.deb", "app-linux-x86_64-gnu.tar.gz")
	d, _, err := c.Decide(context.Background(), "o/app", assets, picker.Linux, picker.AMD64, opts)
	if err != nil || name(d) != "app_amd64.deb" {
		t.Fatalf("got %s err=%v", name(d), err)
	}
	r := fs.lastRequest()
	var v visitorInput
	if err := json.Unmarshal(r.State["visitor"], &v); err != nil {
		t.Fatal(err)
	}
	want := visitorInput{UserAgent: "Mozilla (Windows NT 10.0; Win64; x64)", OS: resolved{"Linux", "query"}, CPU: resolved{cpuNames[picker.AMD64], "ua"},
		ClientHints: opts.ClientHints, RequestedLibc: "gnu"}
	if got, _ := json.Marshal(v); string(got) != string(mustJSON(want)) {
		t.Fatalf("visitor state\n got %s\nwant %s", got, mustJSON(want))
	}
	if _, ok := r.State["product"]; ok {
		t.Fatal("empty product description must not be sent")
	}
	if opts := r.options(t); !slices.Equal(opts, []string{"app-linux-x86_64-gnu.tar.gz", "app_amd64.deb", InsufficientOption}) {
		t.Fatalf("options = %v", opts)
	}
}

// prefer=deb filters before Jev: the compatible tarball is never offered.
func TestDecidePreferFilters(t *testing.T) {
	c, fs := fakeJev(t, picks("", 0.9))
	assets := assetsNamed("app_amd64.deb", "app-linux-x86_64.tar.gz")
	d, _, err := c.Decide(context.Background(), "o/app", assets, picker.Linux, picker.AMD64, picker.PickOpts{Prefer: "deb"})
	if err != nil || name(d) != "app_amd64.deb" || fs.choices.Load() != 0 || len(d.Alternatives) != 0 {
		t.Fatalf("got %s alts=%d choices=%d err=%v", name(d), len(d.Alternatives), fs.choices.Load(), err)
	}
}

func TestDecideLibcRequirementBeforeFormat(t *testing.T) {
	for _, libc := range []picker.Libc{picker.LibcMusl, picker.LibcStatic} {
		t.Run(string(libc), func(t *testing.T) {
			wanted := "tool-linux-amd64-" + string(libc) + ".tar.gz"
			c, _ := fakeJev(t, picks(wanted, 0.95))
			d, _, err := c.Decide(context.Background(), "o/tool", assetsNamed("tool_amd64.deb", wanted), picker.Linux, picker.AMD64, picker.PickOpts{Prefer: "deb", Libc: libc})
			if err != nil || !d.ShouldAutoSelect() || name(d) != wanted {
				t.Fatalf("requested libc lost to unproven format: %+v err=%v", d, err)
			}
		})
	}
	wanted := "tool-linux-amd64-musl.deb"
	c, _ := fakeJev(t, picks(wanted, 0.95))
	d, _, err := c.Decide(context.Background(), "o/tool", assetsNamed(wanted, "tool-linux-amd64-musl.tar.gz"), picker.Linux, picker.AMD64, picker.PickOpts{Prefer: "deb", Libc: picker.LibcMusl})
	if err != nil || !d.ShouldAutoSelect() || name(d) != wanted {
		t.Fatalf("compatible preferred format lost: %+v err=%v", d, err)
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// An unknown CPU stays unknown on the wire and never yields high confidence.
func TestDecideUnknownCPUStaysUnknown(t *testing.T) {
	c, fs := fakeJev(t, picks("app-x64.dmg", 0.99))
	d, _, err := c.Decide(context.Background(), "o/app", assetsNamed("app-arm64.dmg", "app-x64.dmg"), picker.MacOS, picker.UnknownArch, picker.PickOpts{})
	if err != nil || name(d) != "app-x64.dmg" || d.Confidence != picker.ConfidenceMedium {
		t.Fatalf("got %s %s err=%v", name(d), d.Confidence, err)
	}
	if !strings.Contains(string(fs.lastRequest().State["visitor"]), `"cpu":{"value":"unknown"}`) {
		t.Fatalf("visitor state %s", fs.lastRequest().State["visitor"])
	}
}

// Confidence needs a concentrated Choice, and an OS only Jev inferred caps it.
func TestDecideConfidence(t *testing.T) {
	for _, c := range []struct {
		assets []github.Asset
		p      float64
		want   picker.Confidence
	}{
		{linuxPair, 0.95, picker.ConfidenceHigh},
		{linuxPair, 0.45, picker.ConfidenceMedium}, // split vote, clear lead (0.45 vs 0.275)
	} {
		client, _ := fakeJev(t, picks(c.assets[0].Name, c.p))
		d, _, err := client.Decide(context.Background(), "o/tool", c.assets, picker.Linux, picker.AMD64, picker.PickOpts{})
		if err != nil || d.Confidence != c.want || (d.Status == "selected") != (c.want != picker.ConfidenceLow) {
			t.Errorf("p=%.2f: %s %s %s err=%v", c.p, name(d), d.Status, d.Confidence, err)
		}
	}
	// An unmarked zip Jev places on Windows with certainty: medium, not high.
	client, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
		return fakeReply{attrs: func(string) (string, string, float64) { return osWindows, cpuAMD64, 0.95 }}
	})
	d, _, err := client.Decide(context.Background(), "o/app", assetsNamed("app.zip"), picker.Windows, picker.AMD64, picker.PickOpts{})
	if err != nil || name(d) != "app.zip" || d.Confidence != picker.ConfidenceMedium {
		t.Fatalf("inferred OS: %s %s err=%v reasons=%v", name(d), d.Confidence, err, d.Reasons)
	}
}

// An explicit musl request is never met by a file that does not prove its libc.
func TestDecideUnprovenLibcAbstains(t *testing.T) {
	c, _ := fakeJev(t, picks("", 0.95))
	assets := assetsNamed("tool-linux-amd64.tar.gz", "tool-linux-amd64-gnu.tar.gz")
	d, _, err := c.Decide(context.Background(), "o/tool", assets, picker.Linux, picker.AMD64, picker.PickOpts{Libc: picker.LibcMusl})
	if err != nil || name(d) != "tool-linux-amd64.tar.gz" || d.ShouldAutoSelect() {
		t.Fatalf("got %s %s err=%v", name(d), d.Confidence, err)
	}
	for _, a := range d.Alternatives {
		if a.Asset.Name == "tool-linux-amd64-gnu.tar.gz" {
			t.Fatal("gnu build offered for a musl request")
		}
	}
}

// An unmarked Linux build is glibc by convention: a gnu request may still
// auto-select it, capped at medium.
func TestDecideUnmarkedBuildMeetsGNURequest(t *testing.T) {
	c, _ := fakeJev(t, picks("", 0.95))
	assets := assetsNamed("tool-linux-amd64.tar.gz", "tool-linux-amd64-musl.tar.gz")
	d, _, err := c.Decide(context.Background(), "o/tool", assets, picker.Linux, picker.AMD64, picker.PickOpts{Libc: picker.LibcGNU})
	if err != nil || name(d) != "tool-linux-amd64.tar.gz" || d.Confidence != picker.ConfidenceMedium || !d.ShouldAutoSelect() {
		t.Fatalf("got %s %s err=%v", name(d), d.Confidence, err)
	}
}

func TestDecideRejectsMalformedResponses(t *testing.T) {
	for label, tamper := range map[string]func(map[string]any){
		"missing describe answer": func(a map[string]any) { delete(a, "os_0") },
		"missing choice":          func(a map[string]any) { delete(a, "choice") },
		"invented asset": func(a map[string]any) {
			a["choice"] = map[string]any{"type": "choice", "choice": "evil.AppImage", "probabilities": map[string]float64{"evil.AppImage": 1}}
		},
		"bad sum": func(a map[string]any) {
			a["choice"] = map[string]any{"type": "choice", "choice": linuxPair[0].Name, "probabilities": map[string]float64{
				linuxPair[0].Name: 0.9, linuxPair[1].Name: 0.9, InsufficientOption: 0}}
		},
		"choice not argmax": func(a map[string]any) {
			a["choice"] = map[string]any{"type": "choice", "choice": linuxPair[0].Name, "probabilities": map[string]float64{
				linuxPair[0].Name: 0.2, linuxPair[1].Name: 0.8, InsufficientOption: 0}}
		},
	} {
		c, _ := fakeJev(t, func(_ *testing.T, r wireRequest) fakeReply {
			return fakeReply{p: 0.9, tamper: tamper}
		})
		if d, _, err := c.Decide(context.Background(), "o/tool", linuxPair, picker.Linux, picker.AMD64, picker.PickOpts{}); err == nil {
			t.Errorf("%s: accepted %+v", label, d)
		}
	}
}

// A rounded near-tie (seen live: 0.45 chosen vs 0.46) is an answer, not an
// error, but too split to auto-download.
func TestDecideAcceptsRoundedNearTie(t *testing.T) {
	c, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
		return fakeReply{tamper: func(a map[string]any) {
			if _, ok := a["choice"]; ok {
				a["choice"] = map[string]any{"type": "choice", "choice": linuxPair[0].Name, "probabilities": map[string]float64{
					linuxPair[0].Name: 0.45, linuxPair[1].Name: 0.46, InsufficientOption: 0.09}}
			}
		}}
	})
	d, _, err := c.Decide(context.Background(), "o/tool", linuxPair, picker.Linux, picker.AMD64, picker.PickOpts{})
	if err != nil || name(d) != linuxPair[0].Name || d.ShouldAutoSelect() {
		t.Fatalf("near tie: %s %s %s err=%v", name(d), d.Status, d.Confidence, err)
	}
}

func TestDecideServiceErrorIsError(t *testing.T) {
	c, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply { return fakeReply{status: http.StatusBadRequest} })
	if _, _, err := c.Decide(context.Background(), "o/app", assetsNamed("app-arm64.dmg"), picker.MacOS, picker.ARM64, picker.PickOpts{}); err == nil {
		t.Fatal("HTTP 400 must be an error")
	}
}

func TestValidateSelection(t *testing.T) {
	inspected := func(name, os, arch, libc string) github.Asset {
		return github.Asset{Name: name, Artifact: &github.ArtifactMetadata{Status: "inspected",
			Targets:  []github.ArtifactTarget{{Platform: os, Arch: arch, Libc: libc}},
			Evidence: []github.ArtifactEvidence{{Source: "deep-inspection", Field: "format", Value: "elf"}}}}
	}
	musl := picker.PickOpts{Libc: picker.LibcMusl}
	cases := []struct {
		label    string
		a        github.Asset
		platform picker.Platform
		arch     picker.Arch
		opts     picker.PickOpts
		want     bool
	}{
		{"named match", github.Asset{Name: "app-linux-amd64.tar.gz"}, picker.Linux, picker.AMD64, picker.PickOpts{}, true},
		{"other OS", github.Asset{Name: "app-windows-amd64.zip"}, picker.Linux, picker.AMD64, picker.PickOpts{}, false},
		{"observed other OS", inspected("app.zip", "macos", "arm64", ""), picker.Windows, picker.AMD64, picker.PickOpts{}, false},
		{"other CPU", github.Asset{Name: "app-linux-arm64.tar.gz"}, picker.Linux, picker.AMD64, picker.PickOpts{}, false},
		{"rosetta", github.Asset{Name: "app-x64.dmg"}, picker.MacOS, picker.ARM64, picker.PickOpts{}, true},
		{"unknown CPU visitor", github.Asset{Name: "app-linux-arm64.tar.gz"}, picker.Linux, picker.UnknownArch, picker.PickOpts{}, true},
		{"unmarked file", github.Asset{Name: "app.zip"}, picker.Windows, picker.AMD64, picker.PickOpts{}, true},
		{"gnu for musl", inspected("app-linux-amd64", "linux", "amd64", "gnu"), picker.Linux, picker.AMD64, musl, false},
		{"static for musl", inspected("app-linux-amd64", "linux", "amd64", "static"), picker.Linux, picker.AMD64, musl, true},
		{"unknown libc for musl", github.Asset{Name: "app-linux-amd64.tar.gz"}, picker.Linux, picker.AMD64, musl, true},
		{"source", github.Asset{Name: "app-1.0-src.tar.gz"}, picker.Linux, picker.AMD64, picker.PickOpts{}, false},
		{"checksum", github.Asset{Name: "checksums.txt"}, picker.Linux, picker.AMD64, picker.PickOpts{}, false},
		{"not uploaded", github.Asset{Name: "app-linux-amd64.tar.gz", State: "starter"}, picker.Linux, picker.AMD64, picker.PickOpts{}, false},
		{"failed inspection", github.Asset{Name: "app-linux-amd64.tar.gz", Artifact: &github.ArtifactMetadata{Status: "failed"}}, picker.Linux, picker.AMD64, picker.PickOpts{}, false},
		{"conflict", github.Asset{Name: "app-linux-amd64.tar.gz", Artifact: &github.ArtifactMetadata{Status: "inspected", Conflicts: []string{"x"}}}, picker.Linux, picker.AMD64, picker.PickOpts{}, false},
	}
	for _, c := range cases {
		if got := ValidateSelection(c.a, c.platform, c.arch, c.opts); got != c.want {
			t.Errorf("%s: got %v want %v", c.label, got, c.want)
		}
	}
}

// The model input, and so the cache key, follows canonical name order, and
// every model-visible fact or file identity change makes a new key.
func TestPreparedOrderAndKey(t *testing.T) {
	c := &Client{model: "jev-test"}
	base := assetsNamed("b-linux-amd64.tar.gz", "a-linux-amd64.tar.gz", "c-1.0-src.tar.gz")
	k := func(assets []github.Asset, opts picker.PickOpts) (string, []string) {
		p, err := c.prepare("o/r", assets, picker.Linux, picker.AMD64, opts)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, a := range p.candidates {
			names = append(names, a.Name)
		}
		return key(p, "jev-test", assets), names
	}
	k0, names := k(base, picker.PickOpts{})
	if !slices.Equal(names, []string{"a-linux-amd64.tar.gz", "b-linux-amd64.tar.gz"}) {
		t.Fatalf("candidates = %v", names)
	}
	reversed := slices.Clone(base)
	slices.Reverse(reversed)
	if k1, _ := k(reversed, picker.PickOpts{}); k1 != k0 {
		t.Fatal("asset order changed the key")
	}
	counted := slices.Clone(base)
	counted[0].DownloadCount = 99
	if k1, _ := k(counted, picker.PickOpts{}); k1 != k0 {
		t.Fatal("download count changed the key")
	}
	changed := func(label string, assets []github.Asset, opts picker.PickOpts) {
		if k1, _ := k(assets, opts); k1 == k0 {
			t.Errorf("%s did not change the key", label)
		}
	}
	changed("user agent", base, picker.PickOpts{UserAgent: "curl/8"})
	chrome := func(v, os string) picker.PickOpts {
		return picker.PickOpts{UserAgent: "Mozilla/5.0 (X11; " + os + "Linux x86_64) AppleWebKit/537.36 Chrome/" + v + " Safari/537.36",
			ClientHints: map[string]string{"platform_version": `"6.` + v + `"`}}
	}
	if k1, _ := k(base, chrome("141.0.0.0", "")); k1 != func() string { k2, _ := k(base, chrome("142.0.1.2", "")); return k2 }() {
		t.Error("browser or OS minor version changed the key")
	}
	changed("linux distro", base, chrome("141.0.0.0", "Fedora; "))
	changed("source", base, picker.PickOpts{PlatformSource: "query"})
	changed("client hints", base, picker.PickOpts{ClientHints: map[string]string{"bitness": "64"}})
	changed("product", base, picker.PickOpts{ProductDescription: "A CLI"})
	evidence := slices.Clone(base)
	evidence[0].Artifact = &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "musl"}}}
	changed("artifact evidence", evidence, picker.PickOpts{})
	filtered := slices.Clone(base)
	filtered[2].Digest = "sha256:new" // a file Jev never sees still identifies the release
	changed("filtered file identity", filtered, picker.PickOpts{})
	reupload := slices.Clone(base)
	reupload[1].ID = 7
	changed("asset identity", reupload, picker.PickOpts{})
}

// A requested format nobody ships narrows nothing: every compatible file stays.
func TestDecidePreferWithoutMatchKeepsAll(t *testing.T) {
	c, fs := fakeJev(t, picks("app_amd64.deb", 0.95))
	assets := assetsNamed("app_amd64.deb", "app-linux-x86_64.tar.gz")
	if _, _, err := c.Decide(context.Background(), "o/app", assets, picker.Linux, picker.AMD64, picker.PickOpts{Prefer: "rpm"}); err != nil {
		t.Fatal(err)
	}
	if opts := fs.lastRequest().options(t); !slices.Contains(opts, "app_amd64.deb") || !slices.Contains(opts, "app-linux-x86_64.tar.gz") {
		t.Fatalf("options = %v", opts)
	}
}

// A split vote among files that all run still auto-selects when the pick
// clearly leads the runner-up; a near tie does not.
func TestDecideSplitVoteNeedsClearLead(t *testing.T) {
	three := assetsNamed("tool-linux-amd64.AppImage", "tool-linux-amd64.deb", "tool-linux-amd64.tar.gz")
	for _, c := range []struct {
		probs map[string]float64
		want  bool
	}{
		{map[string]float64{three[0].Name: 0.44, three[1].Name: 0.2, three[2].Name: 0.2, InsufficientOption: 0.16}, true},
		{map[string]float64{three[0].Name: 0.44, three[1].Name: 0.40, three[2].Name: 0.1, InsufficientOption: 0.06}, false},
		{map[string]float64{three[0].Name: 0.41, three[1].Name: 0.20, three[2].Name: 0, InsufficientOption: 0.39}, false},
		{map[string]float64{three[0].Name: 0.51, three[1].Name: 0.10, three[2].Name: 0, InsufficientOption: 0.39}, false},
		{map[string]float64{three[0].Name: 0.61, three[1].Name: 0, three[2].Name: 0, InsufficientOption: 0.39}, true},
	} {
		client, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
			return fakeReply{tamper: func(a map[string]any) {
				if _, ok := a["choice"]; ok {
					a["choice"] = map[string]any{"type": "choice", "choice": three[0].Name, "probabilities": c.probs}
				}
			}}
		})
		d, _, err := client.Decide(context.Background(), "o/tool", three, picker.Linux, picker.AMD64, picker.PickOpts{})
		if err != nil || d.ShouldAutoSelect() != c.want || (c.want && d.Confidence != picker.ConfidenceMedium) {
			t.Errorf("probs %v: %s %s err=%v", c.probs, d.Status, d.Confidence, err)
		}
	}
}

// Content that proves a CPU must not turn an unstated OS into Linux.
func TestDecideObservedUnknownOSDoesNotInventLinux(t *testing.T) {
	for _, probability := range []float64{0, 0.6} {
		c, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
			return fakeReply{tamper: func(answers map[string]any) {
				if _, ok := answers["os_0"]; !ok {
					return
				}
				probs := map[string]float64{}
				for _, option := range osCriteria {
					probs[option.name] = 0
				}
				probs[osLinux], probs[osBSD] = probability, 1-probability
				choice := osBSD
				if probability > 0.5 {
					choice = osLinux
				}
				answers["os_0"] = map[string]any{"type": "choice", "choice": choice, "probabilities": probs}
			}}
		})
		assets := assetsNamed("tool-1.0.tar.gz")
		assets[0].Artifact = &github.ArtifactMetadata{Status: "inspected", Role: "application",
			Targets: []github.ArtifactTarget{{Arch: "amd64"}}, Evidence: []github.ArtifactEvidence{{Source: "content-inspection"}}}
		d, _, err := c.Decide(context.Background(), "o/tool", assets, picker.Linux, picker.AMD64, picker.PickOpts{})
		if err != nil || d.ShouldAutoSelect() {
			t.Fatalf("Linux probability %.1f: auto=%v err=%v reasons=%v", probability, d.ShouldAutoSelect(), err, d.Reasons)
		}
	}
}

func TestValidChoiceRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, field := range []string{"probability", "confidence"} {
			a := answer{Type: "choice", Choice: "yes", Probabilities: map[string]float64{"yes": 1, "no": 0}, Confidence: 1}
			if field == "probability" {
				a.Probabilities["yes"] = value
			} else {
				a.Confidence = value
			}
			if err := validChoice(a, []string{"yes", "no"}); err == nil {
				t.Errorf("accepted %s=%v", field, value)
			}
		}
	}
}
