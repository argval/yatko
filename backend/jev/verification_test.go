package jev

import (
	"context"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
	"testing"
)

func TestJevCannotInferPastMissingInspection(t *testing.T) {
	c, fs := fakeJev(t, picks("", 1))
	for _, a := range []github.Asset{
		{Name: "tool-linux-arm64.AppImage"},
		{Name: "sdk-windows-amd64.zip", Artifact: &github.ArtifactMetadata{Status: "inspected", Role: "application", TargetsPartial: true, Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "arm64"}}}},
		{Name: "sdk-windows-amd64.zip", Artifact: &github.ArtifactMetadata{Status: "inspected", Role: "application", TargetsPartial: true, Targets: []github.ArtifactTarget{{Platform: "windows"}}}},
		{Name: "sdk-windows-amd64.zip", Artifact: &github.ArtifactMetadata{Status: "unsupported", TargetsPartial: true}},
	} {
		platform, arch := picker.Windows, picker.AMD64
		if a.Name == "tool-linux-arm64.AppImage" {
			platform, arch = picker.Linux, picker.ARM64
		}
		d, _, err := c.Decide(context.Background(), "test/tool", []github.Asset{a}, platform, arch, picker.PickOpts{})
		if err != nil || d.ShouldAutoSelect() || d.Status != "unknown" {
			t.Fatalf("unchecked file: %+v %v", d, err)
		}
		if ValidateSelection(a, platform, arch, picker.PickOpts{}) {
			t.Fatalf("pin validation accepted unchecked %s", a.Name)
		}
	}
	if fs.choices.Load() != 0 || fs.describes.Load() != 0 {
		t.Fatal("paid calls made for unchecked candidates")
	}
}

func TestJevEmptyPartialRejectsKnownForeignOS(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform picker.Platform
		arch     picker.Arch
	}{
		{"sdk-windows-amd64.zip", picker.MacOS, picker.AMD64},
		{"sdk-amd64.msi", picker.MacOS, picker.AMD64},
		{"bun-linux-aarch64-android.zip", picker.Linux, picker.ARM64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, fs := fakeJev(t, func(*testing.T, wireRequest) fakeReply {
				return fakeReply{attrs: func(string) (string, string, float64) {
					cpu := cpuAMD64
					if tc.arch == picker.ARM64 {
						cpu = cpuARM64
					}
					return platformOS[tc.platform], cpu, 0.95
				}}
			})
			a := github.Asset{Name: tc.name, Artifact: &github.ArtifactMetadata{Status: "unsupported", TargetsPartial: true}}
			d, _, err := c.Decide(context.Background(), "test/tool", []github.Asset{a}, tc.platform, tc.arch, picker.PickOpts{})
			if err != nil || d.ShouldAutoSelect() || d.Status != "incompatible" {
				t.Fatalf("foreign filename OS bypassed empty inspection: %+v err=%v", d, err)
			}
			if ValidateSelection(a, tc.platform, tc.arch, picker.PickOpts{}) || fs.calls() != 0 {
				t.Fatal("known foreign OS reached model or pin validation")
			}
		})
	}
}

func TestJevRejectsMismatchedAppImageAndAcceptsMatchingSample(t *testing.T) {
	c, _ := fakeJev(t, picks("", 1))
	a := github.Asset{Name: "tool-linux-arm64.AppImage", Artifact: &github.ArtifactMetadata{Status: "inspected", LauncherArch: "amd64", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64"}}}}
	for _, arch := range []picker.Arch{picker.AMD64, picker.ARM64} {
		if ValidateSelection(a, picker.Linux, arch, picker.PickOpts{}) {
			t.Fatalf("mismatched launcher accepted for %s", arch)
		}
	}
	a.Name = "tool.AppImage"
	a.Artifact.Targets = append(a.Artifact.Targets, github.ArtifactTarget{Platform: "linux", Arch: "arm64"})
	if ValidateSelection(a, picker.Linux, picker.ARM64, picker.PickOpts{}) {
		t.Fatal("multi-CPU payload bypassed the launcher's CPU")
	}
	if !ValidateSelection(a, picker.Linux, picker.AMD64, picker.PickOpts{}) {
		t.Fatal("matching launcher rejected")
	}
	a = github.Asset{Name: "sdk-windows.zip", Artifact: &github.ArtifactMetadata{Status: "inspected", Role: "application", TargetsPartial: true, Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "amd64"}}}}
	d, _, err := c.Decide(context.Background(), "test/tool", []github.Asset{a}, picker.Windows, picker.AMD64, picker.PickOpts{})
	if err != nil || !d.ShouldAutoSelect() {
		t.Fatalf("matching sample rejected: %+v %v", d, err)
	}
}

func TestManifestConflictCannotBypassLauncherCheck(t *testing.T) {
	a := github.Asset{Name: "tool-linux-arm64.AppImage", Artifact: &github.ArtifactMetadata{Status: "inspected", Role: "application", Targets: []github.ArtifactTarget{{Platform: "linux", Arch: "amd64"}}, Evidence: []github.ArtifactEvidence{{Source: "electron", Field: "cpu", Value: "amd64"}}}}
	if ValidateSelection(a, picker.Linux, picker.AMD64, picker.PickOpts{}) {
		t.Fatal("publisher CPU disagreement was treated as verified launcher evidence")
	}
	c, _ := fakeJev(t, picks("", 1))
	d, _, err := c.Decide(context.Background(), "test/tool", []github.Asset{a}, picker.Linux, picker.AMD64, picker.PickOpts{})
	if err != nil || d.ShouldAutoSelect() {
		t.Fatalf("unverified launcher selected through filename conflict: %+v %v", d, err)
	}
}
