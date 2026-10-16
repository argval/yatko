package picker

import (
	"testing"

	"github.com/argval/yatko/github"
)

func TestUnsupportedFilenameCPUsRemainConstraints(t *testing.T) {
	for _, cpu := range []string{"riscv64", "ppc64le", "ppc64", "s390x", "mips", "mips64el", "loongarch64", "sparc64", "aarch64_be"} {
		for _, arch := range []Arch{AMD64, ARM64, ARM, X86} {
			name := "app-linux-" + cpu + ".tar.gz"
			if d := DecideAsset([]github.Asset{{Name: name}}, Linux, arch, PickOpts{}); d.ShouldAutoSelect() {
				t.Errorf("%s offered to %s: %+v", name, arch, d)
			}
		}
	}
}

func TestWin32NamespaceUsesExplicitCPU(t *testing.T) {
	for _, cpu := range []string{"x64", "arm64"} {
		f := Classify("app-win32-" + cpu + ".zip")
		want := AMD64
		if cpu == "arm64" {
			want = ARM64
		}
		if len(f.Arches) != 1 || f.Arches[0] != want {
			t.Fatalf("%s: %v", cpu, f.Arches)
		}
	}
	if f := Classify("app-win32.zip"); !f.HasArch(X86) {
		t.Fatalf("plain win32 lost x86: %+v", f)
	}
}

func TestDebugSidecarsNeverBecomeOnlyDownload(t *testing.T) {
	for _, role := range []string{"debuginfo", "debugsymbols", "debug-symbols"} {
		d := DecideAsset([]github.Asset{{Name: "app-linux-amd64-" + role + ".tar.gz"}}, Linux, AMD64, PickOpts{})
		if d.ShouldAutoSelect() {
			t.Fatalf("debug-only release selected: %+v", d)
		}
	}
}

func TestObservedUniversalBuildWins(t *testing.T) {
	universal := github.Asset{Name: "App.dmg", Artifact: &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "macos", Arch: "amd64"}, {Platform: "macos", Arch: "arm64"}}}}
	for _, arch := range []Arch{AMD64, ARM64} {
		d := DecideAsset([]github.Asset{{Name: "App-macos-" + string(arch) + ".dmg"}, universal}, MacOS, arch, PickOpts{})
		if !d.ShouldAutoSelect() || d.Asset.Name != universal.Name {
			t.Fatalf("%s: %+v", arch, d)
		}
	}
	if f := Classify("App-universal.dmg"); !f.HasArch(AMD64) || !f.HasArch(ARM64) {
		t.Fatalf("universal DMG: %+v", f)
	}
}

func TestObservedLibcRanksLikeFilenameLibc(t *testing.T) {
	assets := []github.Asset{
		{Name: "A-linux-amd64.tar.gz", Artifact: &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "musl"}}}},
		{Name: "B-linux-amd64.tar.gz", Artifact: &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "gnu"}}}},
	}
	d := DecideAsset(assets, Linux, AMD64, PickOpts{})
	if !d.ShouldAutoSelect() || d.Asset.Name != assets[1].Name {
		t.Fatalf("default libc ranking: %+v", d)
	}
	for i, libc := range []Libc{LibcMusl, LibcGNU} {
		d = DecideAsset(assets, Linux, AMD64, PickOpts{Libc: libc})
		if !d.ShouldAutoSelect() || d.Asset.Name != assets[i].Name {
			t.Fatalf("%s: %+v", libc, d)
		}
	}
}

func TestAdditionalInstallableFormats(t *testing.T) {
	for _, tc := range []struct {
		ext      string
		platform Platform
	}{{"flatpak", Linux}, {"snap", Linux}, {"msix", Windows}, {"appx", Windows}, {"tar", Linux}} {
		name := "app-" + string(tc.platform) + "-amd64." + tc.ext
		d := DecideAsset([]github.Asset{{Name: name}}, tc.platform, AMD64, PickOpts{})
		if !d.ShouldAutoSelect() {
			t.Fatalf("%s: %+v", name, d)
		}
	}
}
