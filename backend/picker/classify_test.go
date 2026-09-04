package picker

import "testing"

func TestClassify_DarwinArm64Dmg(t *testing.T) {
	f := Classify("Logseq-darwin-arm64-2.0.1.dmg")
	if !f.HasPlatform(MacOS) {
		t.Fatalf("platforms = %v, want macos", f.Platforms)
	}
	if !f.HasArch(ARM64) {
		t.Fatalf("arches = %v, want arm64", f.Arches)
	}
	if f.Extension != "dmg" {
		t.Fatalf("extension = %q, want dmg", f.Extension)
	}
	if f.FormatPlatform != MacOS {
		t.Fatalf("format platform = %q, want macos", f.FormatPlatform)
	}
	if f.Kind != KindInstaller {
		t.Fatalf("kind = %q, want installer", f.Kind)
	}
	if f.Source || f.NonNative {
		t.Fatalf("source=%v nonNative=%v", f.Source, f.NonNative)
	}
}

func TestClassify_DoesNotInventPlatform(t *testing.T) {
	f := Classify("release.zip")
	if len(f.Platforms) != 0 {
		t.Fatalf("platforms = %v, want none", f.Platforms)
	}
	if len(f.Arches) != 0 {
		t.Fatalf("arches = %v, want none", f.Arches)
	}
	if !f.Source {
		t.Fatal("bare zip without os/arch should be source")
	}
}

func TestClassify_Win64IsWindowsAndAmd64(t *testing.T) {
	f := Classify("x16emu_win64-r49.zip")
	if !f.HasPlatform(Windows) {
		t.Fatalf("platforms = %v, want windows", f.Platforms)
	}
	if !f.HasArch(AMD64) {
		t.Fatalf("arches = %v, want amd64", f.Arches)
	}
}

func TestClassify_WasmIsNonNative(t *testing.T) {
	f := Classify("x16emu_wasm-r49.zip")
	if !f.NonNative {
		t.Fatal("expected non-native")
	}
}

func TestClassify_LongestArchAliasWins(t *testing.T) {
	cases := []struct {
		name      string
		wantArch  Arch
		notArch   Arch
		wantPlat  Platform
	}{
		{name: "tool-linux.arm32.zip", wantArch: ARM, notArch: ARM64, wantPlat: Linux},
		{name: "tool-linux.arm64.zip", wantArch: ARM64, notArch: ARM, wantPlat: Linux},
		{name: "tool-arm-unknown-linux-gnueabihf.gz", wantArch: ARM, notArch: ARM64, wantPlat: Linux},
		{name: "tool-aarch64-unknown-linux-gnu.gz", wantArch: ARM64, notArch: ARM, wantPlat: Linux},
		{name: "Dopamine-3.0.10-arm.dmg", wantArch: ARM64, notArch: ARM},
	}
	for _, tc := range cases {
		f := Classify(tc.name)
		if !f.HasArch(tc.wantArch) {
			t.Errorf("%s: arches = %v, want %s", tc.name, f.Arches, tc.wantArch)
		}
		if f.HasArch(tc.notArch) {
			t.Errorf("%s: arches = %v, must not include %s", tc.name, f.Arches, tc.notArch)
		}
		if tc.wantPlat != "" && !f.HasPlatform(tc.wantPlat) {
			t.Errorf("%s: platforms = %v, want %s", tc.name, f.Platforms, tc.wantPlat)
		}
	}
}

func TestIncompatiblePlatform_AndroidLinuxHost(t *testing.T) {
	android := Classify("bun-linux-aarch64-android.zip")
	if incompatiblePlatform(android, Android) {
		t.Fatal("linux+android zip should be eligible for android")
	}
	if !incompatiblePlatform(android, Linux) {
		t.Fatal("linux+android zip should be excluded for linux")
	}
	plain := Classify("bun-linux-aarch64.zip")
	if incompatiblePlatform(plain, Linux) {
		t.Fatal("plain linux zip should be eligible for linux")
	}
}
