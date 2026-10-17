package github

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"howett.net/plist"
)

// deepAppFixture supplies archive bytes without requiring external extractors.
func deepAppFixture(files map[string][]byte) *deepArchive {
	var names []string
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	a := &deepArchive{kind: "zip"}
	for _, name := range names {
		a.entries = append(a.entries, deepEntry{path: name, size: int64(len(files[name]))})
	}
	a.readEntry = func(e deepEntry, off, n int64) ([]byte, error) {
		return files[e.path][off : off+n], nil
	}
	return a
}

func deepAppPlist(t *testing.T, executable string, format int) []byte {
	t.Helper()
	data, err := plist.Marshal(struct {
		Executable string `plist:"CFBundleExecutable"`
	}{executable}, format)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDeepMacAppsFollowDeclaredExecutable(t *testing.T) {
	for _, format := range []int{plist.XMLFormat, plist.BinaryFormat} {
		t.Run(fmt.Sprint(format), func(t *testing.T) {
			a := deepAppFixture(map[string][]byte{
				"Download.app/Contents/Info.plist":                                        deepAppPlist(t, "Actual Main", format),
				"Download.app/Contents/MacOS/Actual Main":                                 machoFixture(0x01000007, 2, 0),
				"Download.app/Contents/MacOS/Download":                                    machoFixture(0x0100000c, 2, 1),
				"Download.app/Contents/Frameworks/Helper.app/Contents/Info.plist":         deepAppPlist(t, "Helper", format),
				"Download.app/Contents/Frameworks/Helper.app/Contents/MacOS/Helper":       machoFixture(0x0100000c, 2, 1),
				"Download.app/Contents/Frameworks/Other.framework/Versions/A/OtherBinary": machoFixture(0x0100000c, 6, 1),
			})
			m, ok, err := a.inspectMacApps()
			want := []ArtifactTarget{{Platform: "macos", Arch: "amd64"}}
			if err != nil || !ok || !slices.Equal(m.Targets, want) || !deepProven(m) {
				t.Fatalf("metadata=%+v recognized=%v error=%v", m, ok, err)
			}
		})
	}
}

func TestDeepMacAppsRequireMainInsteadOfHelper(t *testing.T) {
	for _, tc := range []struct {
		name string
		main []byte
		exe  string
	}{
		{"missing main", nil, "Missing"},
		{"escaped main", machoFixture(0x01000007, 2, 1), "../Helper"},
		{"wrong platform", machoFixture(0x0100000c, 2, 2), "Main"},
		{"ELF is not Mac", elfFixture(elf.ELFOSABI_NONE, elf.ET_EXEC, "", false), "Main"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{
				"App.app/Contents/Info.plist":   deepAppPlist(t, tc.exe, plist.XMLFormat),
				"App.app/Contents/MacOS/Helper": machoFixture(0x0100000c, 2, 1),
			}
			if tc.main != nil {
				files["App.app/Contents/MacOS/Main"] = tc.main
			}
			m, ok, err := deepAppFixture(files).inspectMacApps()
			if err != nil || !ok || deepProven(m) || len(m.Targets) != 0 {
				t.Fatalf("metadata=%+v recognized=%v error=%v", m, ok, err)
			}
		})
	}
}

func TestDeepMacAppsIntersectTopLevelBundles(t *testing.T) {
	a := deepAppFixture(map[string][]byte{
		"One.app/Contents/Info.plist": deepAppPlist(t, "One", plist.XMLFormat),
		"One.app/Contents/MacOS/One":  machoFixture(0x01000007, 2, 1),
		"Two.app/Contents/Info.plist": deepAppPlist(t, "Two", plist.XMLFormat),
		"Two.app/Contents/MacOS/Two":  machoFixture(0x0100000c, 2, 1),
	})
	m, ok, err := a.inspectMacApps()
	if err != nil || !ok || deepProven(m) || len(m.Targets) != 0 {
		t.Fatalf("different main CPUs were combined: %+v %v %v", m, ok, err)
	}
}

func TestDeepMacAppsPropagateReadFailure(t *testing.T) {
	a := deepAppFixture(map[string][]byte{"App.app/Contents/Info.plist": []byte("invalid")})
	want := errors.New("extractor read failed")
	a.readEntry = func(deepEntry, int64, int64) ([]byte, error) { return nil, want }
	_, ok, err := a.inspectMacApps()
	if !ok || !errors.Is(err, want) {
		t.Fatalf("recognized=%v error=%v", ok, err)
	}
}

func TestDeepMSIXIdentityAndNeutralMain(t *testing.T) {
	for _, architecture := range []string{"arm64", "neutral"} {
		t.Run(architecture, func(t *testing.T) {
			manifest := `<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"><Identity ProcessorArchitecture="` + architecture + `"/><Applications><Application Executable="bin\App.exe"/></Applications></Package>`
			a := deepAppFixture(map[string][]byte{
				"AppxManifest.xml": []byte(manifest),
				"bin/App.exe":      hybridPEFixture(0x8664, 0, false),
				"helper.exe":       machoFixture(0x0100000c, 2, 1),
			})
			m, ok, err := a.inspectMSIX()
			arch := "amd64"
			if architecture == "arm64" {
				arch = "arm64"
			}
			want := []ArtifactTarget{{Platform: "windows", Arch: arch}}
			if err != nil || !ok || !deepProven(m) || !slices.Equal(m.Targets, want) || m.Role != "installer" {
				t.Fatalf("metadata=%+v recognized=%v error=%v", m, ok, err)
			}
		})
	}
}

func TestDeepMSIXBundleSkipsResourceAndStubArchitectures(t *testing.T) {
	manifest := `<Bundle xmlns="http://schemas.microsoft.com/appx/2013/bundle"><Packages>
<Package Type="application" Architecture="x64" FileName="x64.msix"/>
<Package Type="application" Architecture="arm64" FileName="arm64.msix"/>
<Package Type="resource" Architecture="x86" FileName="resources.msix"/>
<Package Type="application" Architecture="x86" FileName="stub.msix" IsStub="true"/>
</Packages></Bundle>`
	a := deepAppFixture(map[string][]byte{
		"AppxMetadata/AppxBundleManifest.xml": []byte(manifest),
		"x64.msix":                            []byte("package"),
		"arm64.msix":                          []byte("package"),
	})
	m, ok, err := a.inspectMSIX()
	want := []ArtifactTarget{{Platform: "windows", Arch: "amd64"}, {Platform: "windows", Arch: "arm64"}}
	if err != nil || !ok || !deepProven(m) || !slices.Equal(m.Targets, want) {
		t.Fatalf("metadata=%+v recognized=%v error=%v", m, ok, err)
	}
}

func TestDeepMSIXNeutralNeverUsesUndeclaredHelper(t *testing.T) {
	for _, executable := range []string{"Missing.exe", `..\helper.exe`, `C:\helper.exe`, ""} {
		t.Run(executable, func(t *testing.T) {
			manifest := `<Package><Identity ProcessorArchitecture="neutral"/><Applications><Application Executable="` + executable + `"/></Applications></Package>`
			a := deepAppFixture(map[string][]byte{
				"AppxManifest.xml": []byte(manifest),
				"helper.exe":       fixture(t, "go-windows-amd64.head"),
			})
			m, ok, err := a.inspectMSIX()
			if err != nil || !ok || deepProven(m) || len(m.Targets) != 0 {
				t.Fatalf("metadata=%+v recognized=%v error=%v", m, ok, err)
			}
		})
	}
}

func TestDeepMSIXAmbiguousPathsStayUnknown(t *testing.T) {
	manifest := []byte(`<Package><Identity ProcessorArchitecture="x64"/><Applications><Application Executable="App.exe"/></Applications></Package>`)
	a := deepAppFixture(map[string][]byte{"AppxManifest.xml": manifest, "appxmanifest.xml": manifest})
	m, ok, err := a.inspectMSIX()
	if err != nil || !ok || deepProven(m) || len(m.Targets) != 0 {
		t.Fatalf("metadata=%+v recognized=%v error=%v", m, ok, err)
	}
}

func TestDeepMSIXNeutralCLRDoesNotClaimStubCPU(t *testing.T) {
	data := slices.Clone(fixture(t, "go-windows-amd64.head"))
	pe := int(binary.LittleEndian.Uint32(data[0x3c:]))
	binary.LittleEndian.PutUint16(data[pe+4:], 0x14c)
	binary.LittleEndian.PutUint16(data[pe+24:], 0x10b)
	binary.LittleEndian.PutUint32(data[pe+24+96+14*8:], 0x1234)
	manifest := `<Package><Identity ProcessorArchitecture="neutral"/><Applications><Application Executable="App.exe"/></Applications></Package>`
	a := deepAppFixture(map[string][]byte{"AppxManifest.xml": []byte(manifest), "App.exe": data})
	m, ok, err := a.inspectMSIX()
	if err != nil || !ok || deepProven(m) || len(m.Targets) != 0 || !strings.Contains(m.Evidence[0].Value, "CLR") {
		t.Fatalf("metadata=%+v recognized=%v error=%v", m, ok, err)
	}
}

// A bundle whose CFBundleExecutable is a launcher script takes the CPU of the
// Mach-O beside it (Sniffnet); a script with no Mach-O stays unknown.
func TestDeepMacAppsFollowLauncherScript(t *testing.T) {
	a := deepAppFixture(map[string][]byte{
		"App.app/Contents/Info.plist":       deepAppPlist(t, "wrapper.sh", plist.XMLFormat),
		"App.app/Contents/MacOS/wrapper.sh": []byte("#!/usr/bin/env zsh\nexec \"$(dirname \"$0\")/app\"\n"),
		"App.app/Contents/MacOS/app":        machoFixture(0x0100000c, 2, 1),
	})
	m, ok, err := a.inspectMacApps()
	if err != nil || !ok || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "macos", Arch: "arm64"}}) {
		t.Fatalf("launcher: metadata=%+v recognized=%v error=%v", m, ok, err)
	}
	a = deepAppFixture(map[string][]byte{
		"App.app/Contents/Info.plist":       deepAppPlist(t, "wrapper.sh", plist.XMLFormat),
		"App.app/Contents/MacOS/wrapper.sh": []byte("#!/bin/sh\njava -jar app.jar\n"),
	})
	if m, _, err := a.inspectMacApps(); err != nil || deepProven(m) {
		t.Fatalf("script without Mach-O was proven: %+v %v", m, err)
	}
}

// When an Inno installer's top-level executables disagree on CPU, the one
// named after AppName decides (ShareX.exe beside a 32-bit exiftool.exe).
func TestDeepInnoAppNamePicksMainExecutable(t *testing.T) {
	build := func(appName string) *deepArchive {
		a := deepAppFixture(map[string][]byte{"ShareX.exe": hybridPEFixture(0x8664, 0, false), "exiftool.exe": hybridPEFixture(0xaa64, 0, false)})
		a.kind, a.appName = "Inno 6.7.0", appName
		return a
	}
	m, ok, err := build("ShareX").inspectExecutables("setup.exe")
	if err != nil || !ok || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "windows", Arch: "amd64"}}) {
		t.Fatalf("AppName main: metadata=%+v recognized=%v error=%v", m, ok, err)
	}
	if m, _, err := build("Other").inspectExecutables("setup.exe"); err != nil || deepProven(m) {
		t.Fatalf("no AppName match must stay unknown: %+v %v", m, err)
	}
}
