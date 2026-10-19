package jev

import (
	"context"
	"encoding/json"
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
	}
	return out
}

// Jev's direct choice wins over the rule picker's format ranking when the
// model judges a CLI's tarball the right file, and the asset is the caller's own.
func TestDecideDirectChoiceDiffersFromRanking(t *testing.T) {
	assets := assetsNamed("tool-linux-amd64.AppImage", "tool-linux-amd64.tar.gz")
	rules := picker.DecideAsset(assets, picker.Linux, picker.AMD64, picker.PickOpts{})
	if name(rules) != "tool-linux-amd64.AppImage" {
		t.Fatalf("precondition: rules picked %s", name(rules))
	}
	c, _ := fakeJev(t, picks("tool-linux-amd64.tar.gz", 0.9))
	d, _, err := c.Decide(context.Background(), "o/tool", assets, picker.Linux, picker.AMD64, picker.PickOpts{})
	if err != nil || name(d) != "tool-linux-amd64.tar.gz" || d.Confidence != picker.ConfidenceHigh || d.Status != "selected" {
		t.Fatalf("got %s %s %s err=%v reasons=%v", name(d), d.Status, d.Confidence, err, d.Reasons)
	}
	if d.Asset.BrowserDownloadURL != assets[1].BrowserDownloadURL {
		t.Fatalf("asset URL %q is not the release's", d.Asset.BrowserDownloadURL)
	}
	if len(d.Alternatives) != 1 || d.Alternatives[0].Asset.Name != "tool-linux-amd64.AppImage" {
		t.Fatalf("alternatives = %+v", d.Alternatives)
	}
}

// No match and "not enough information" are answers, not errors.
func TestDecideAbstentions(t *testing.T) {
	assets := assetsNamed("app-arm64.dmg", "app-x64.dmg")
	for choice, status := range map[string]string{NoCompatibleOption: "incompatible", InsufficientOption: "unknown"} {
		c, _ := fakeJev(t, picks(choice, 0.9))
		d, _, err := c.Decide(context.Background(), "o/app", assets, picker.MacOS, picker.UnknownArch, picker.PickOpts{})
		if err != nil || d.Asset != nil || d.Status != status || d.ShouldAutoSelect() {
			t.Fatalf("%s: %+v err=%v", choice, d, err)
		}
		if len(d.Alternatives) != 2 {
			t.Fatalf("%s: abstention should list runnable alternatives, got %d", choice, len(d.Alternatives))
		}
	}
	// Code answers without a call when nothing can run or the OS is unknown.
	c, fs := fakeJev(t, picks(NoCompatibleOption, 1))
	if d, _, err := c.Decide(context.Background(), "o/app", assets, picker.Windows, picker.AMD64, picker.PickOpts{}); err != nil || d.Status != "incompatible" {
		t.Fatalf("windows: %+v %v", d, err)
	}
	if d, _, err := c.Decide(context.Background(), "o/app", assets, picker.Unknown, picker.UnknownArch, picker.PickOpts{}); err != nil || d.Status != "unknown" {
		t.Fatalf("unknown OS: %+v %v", d, err)
	}
	if fs.calls.Load() != 0 {
		t.Fatalf("calls = %d, want 0", fs.calls.Load())
	}
}

// The request carries the raw UA, resolved OS/CPU with provenance and the
// explicit requests; files proven for another OS are never offered.
func TestDecideStateCarriesExplicitContext(t *testing.T) {
	c, fs := fakeJev(t, containing(".deb", 0.9))
	opts := picker.PickOpts{
		Prefer: "deb", Libc: picker.LibcGNU, UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
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
		ClientHints: opts.ClientHints, RequestedFormat: ".deb", RequestedLibc: "gnu"}
	if got, _ := json.Marshal(v); string(got) != string(mustJSON(want)) {
		t.Fatalf("visitor state\n got %s\nwant %s", got, mustJSON(want))
	}
	if _, ok := r.State["product"]; ok {
		t.Fatal("empty product description must not be sent")
	}
	if opts := r.options(t); slices.Contains(opts, "app-setup-x64.exe") {
		t.Fatalf("windows installer offered to a linux visitor: %v", opts)
	}
	if len(r.Questions) != 3 { // choice + runs_0 + runs_1
		t.Fatalf("questions = %d", len(r.Questions))
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// An unknown CPU stays unknown on the wire and never yields high confidence.
func TestDecideUnknownCPUStaysUnknown(t *testing.T) {
	c, fs := fakeJev(t, picks("app-x64.dmg", 0.99))
	d, _, err := c.Decide(context.Background(), "o/app", assetsNamed("app-arm64.dmg", "app-x64.dmg"), picker.MacOS, picker.UnknownArch, picker.PickOpts{})
	if err != nil || d.Confidence != picker.ConfidenceMedium {
		t.Fatalf("got %s %s err=%v", name(d), d.Confidence, err)
	}
	if !strings.Contains(string(fs.lastRequest().State["visitor"]), `"cpu":{"value":"unknown"}`) {
		t.Fatalf("visitor state %s", fs.lastRequest().State["visitor"])
	}
}

// Confidence needs a concentrated Choice; the file's own "runs" Noul gates
// only files whose OS or CPU the targets do not prove.
func TestDecideGatesOnRunsNoul(t *testing.T) {
	assets := assetsNamed("app-windows-x64.zip", "app-x64.zip", "app-linux-x64.tar.gz")
	for _, c := range []struct {
		file    string
		p, runs float64
		want    picker.Confidence
	}{
		{"app-windows-x64.zip", 0.95, 0.4, picker.ConfidenceHigh}, // proven: runs ignored
		{"app-windows-x64.zip", 0.45, 0.95, picker.ConfidenceLow},
		{"app-x64.zip", 0.95, 0.95, picker.ConfidenceMedium}, // OS inferred: capped
		{"app-x64.zip", 0.95, 0.8, picker.ConfidenceLow},
	} {
		client, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
			return fakeReply{choice: c.file, p: c.p, runs: func(string) float64 { return c.runs }}
		})
		d, _, err := client.Decide(context.Background(), "o/app", assets, picker.Windows, picker.AMD64, picker.PickOpts{})
		if err != nil || name(d) != c.file || d.Confidence != c.want || (d.Status == "selected") != (c.want != picker.ConfidenceLow) {
			t.Errorf("%s p=%.2f runs=%.2f: %s %s %s err=%v", c.file, c.p, c.runs, name(d), d.Status, d.Confidence, err)
		}
	}
}

// An explicit libc request is never met by a file that does not prove its libc.
func TestDecideUnprovenLibcAbstains(t *testing.T) {
	c, fs := fakeJev(t, picks("tool-linux-amd64.tar.gz", 0.95))
	assets := assetsNamed("tool-linux-amd64.tar.gz", "tool-linux-amd64-gnu.tar.gz")
	d, _, err := c.Decide(context.Background(), "o/tool", assets, picker.Linux, picker.AMD64, picker.PickOpts{Libc: picker.LibcMusl})
	if err != nil || name(d) != "tool-linux-amd64.tar.gz" || d.ShouldAutoSelect() {
		t.Fatalf("got %s %s err=%v", name(d), d.Confidence, err)
	}
	if slices.Contains(fs.lastRequest().options(t), "tool-linux-amd64-gnu.tar.gz") {
		t.Fatal("gnu build offered for a musl request")
	}
}

// An unmarked Linux build is glibc by convention: a gnu request may still
// auto-select it, capped at medium.
func TestDecideUnmarkedBuildMeetsGNURequest(t *testing.T) {
	c, _ := fakeJev(t, picks("tool-linux-amd64.tar.gz", 0.95))
	assets := assetsNamed("tool-linux-amd64.tar.gz", "tool-linux-amd64-musl.tar.gz")
	d, _, err := c.Decide(context.Background(), "o/tool", assets, picker.Linux, picker.AMD64, picker.PickOpts{Libc: picker.LibcGNU})
	if err != nil || name(d) != "tool-linux-amd64.tar.gz" || d.Confidence != picker.ConfidenceMedium || !d.ShouldAutoSelect() {
		t.Fatalf("got %s %s err=%v", name(d), d.Confidence, err)
	}
}

func TestDecideRejectsMalformedResponses(t *testing.T) {
	assets := assetsNamed("app-arm64.dmg", "app-x64.dmg")
	for label, tamper := range map[string]func(map[string]any){
		"missing runs":   func(a map[string]any) { delete(a, "runs_1") },
		"noul as choice": func(a map[string]any) { a["runs_0"] = map[string]any{"type": "choice", "choice": "x"} },
		"noul above 1":   func(a map[string]any) { a["runs_0"] = map[string]any{"type": "noul", "noul": 1.5} },
		"missing choice": func(a map[string]any) { delete(a, "choice") },
		"invented asset": func(a map[string]any) {
			a["choice"] = map[string]any{"type": "choice", "choice": "evil.dmg", "probabilities": map[string]float64{"evil.dmg": 1}}
		},
		"bad sum": func(a map[string]any) {
			a["choice"] = map[string]any{"type": "choice", "choice": "app-arm64.dmg", "probabilities": map[string]float64{
				"app-arm64.dmg": 0.9, "app-x64.dmg": 0.9, NoCompatibleOption: 0, InsufficientOption: 0}}
		},
		"choice not argmax": func(a map[string]any) {
			a["choice"] = map[string]any{"type": "choice", "choice": "app-arm64.dmg", "probabilities": map[string]float64{
				"app-arm64.dmg": 0.2, "app-x64.dmg": 0.8, NoCompatibleOption: 0, InsufficientOption: 0}}
		},
	} {
		c, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
			return fakeReply{choice: "app-arm64.dmg", p: 0.9, tamper: tamper}
		})
		if d, _, err := c.Decide(context.Background(), "o/app", assets, picker.MacOS, picker.ARM64, picker.PickOpts{}); err == nil {
			t.Errorf("%s: accepted %+v", label, d)
		}
	}
}

// A rounded near-tie (seen live: 0.45 chosen vs 0.46) is an answer, not an
// error, but too split to auto-download.
func TestDecideAcceptsRoundedNearTie(t *testing.T) {
	assets := assetsNamed("app-arm64.dmg", "app-x64.dmg")
	c, _ := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
		return fakeReply{choice: "app-arm64.dmg", p: 0.9, tamper: func(a map[string]any) {
			a["choice"] = map[string]any{"type": "choice", "choice": "app-arm64.dmg", "probabilities": map[string]float64{
				"app-arm64.dmg": 0.45, "app-x64.dmg": 0.46, NoCompatibleOption: 0.09, InsufficientOption: 0}}
		}}
	})
	d, _, err := c.Decide(context.Background(), "o/app", assets, picker.MacOS, picker.ARM64, picker.PickOpts{})
	if err != nil || name(d) != "app-arm64.dmg" || d.ShouldAutoSelect() {
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
		return key(p, assets), names
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
