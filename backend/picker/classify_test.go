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
