package github

import (
	"context"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
)

func TestZIPSampleSpansCPUDirectories(t *testing.T) {
	s := newFakeDownloads(t)
	arm := fixture(t, "go-windows-amd64.head")
	peOffset := binary.LittleEndian.Uint32(arm[0x3c:])
	binary.LittleEndian.PutUint16(arm[peOffset+4:], 0xaa64)
	data := zipFixture(t, map[string][]byte{
		"arm64/bin/a.exe": arm,
		"arm64/bin/b.exe": arm,
		"x64/bin/a.exe":   fixture(t, "go-windows-amd64.head"),
		"x86/bin/a.exe":   fixture(t, "go-windows-amd64.head"),
	}, 0)
	m, err := s.client().InspectArtifact(context.Background(), s.add("sdk-windows.zip", data))
	if err != nil || !m.TargetsPartial {
		t.Fatalf("sample coverage: %+v %v", m, err)
	}
	for _, arch := range []string{"arm64", "amd64"} {
		if !slices.Contains(m.Targets, ArtifactTarget{Platform: "windows", Arch: arch}) {
			t.Fatalf("CPU directory not sampled: %s, %+v", arch, m)
		}
	}
	if s.requests > inspectMaxRequests {
		t.Fatalf("request budget exceeded: %d", s.requests)
	}
}

func TestArchiveCoverageSurvivesSerialization(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []byte
		partial bool
	}{
		{"one.zip", zipFixture(t, map[string][]byte{"tool.exe": fixture(t, "go-windows-amd64.head")}, 0), false},
		{"many.tar.gz", tarGzFixture(t,
			tarEntry{"a", 0755, fixture(t, "go-linux-arm64.head")},
			tarEntry{"b", 0755, fixture(t, "go-linux-arm64.head")},
			tarEntry{"c", 0755, fixture(t, "go-linux-arm64.head")}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeDownloads(t)
			m, err := s.client().InspectArtifact(context.Background(), s.add(tc.name, tc.data))
			if err != nil || m.TargetsPartial != tc.partial {
				t.Fatalf("coverage: %+v %v", m, err)
			}
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			var restored ArtifactMetadata
			if err := json.Unmarshal(b, &restored); err != nil || restored.TargetsPartial != tc.partial {
				t.Fatalf("round trip: %+v %v", restored, err)
			}
		})
	}
}

func TestAppImageRecordsLauncherCPU(t *testing.T) {
	s := newFakeDownloads(t)
	m, err := s.client().InspectArtifact(context.Background(), s.add("tool-linux-arm64.AppImage", elfFixture(0, elf.ET_EXEC, "", true)))
	if err != nil || m.LauncherArch != "amd64" {
		t.Fatalf("launcher: %+v %v", m, err)
	}
	// Full-file header reads prefix evidence fields with the original filename.
	m.LauncherArch = ""
	m.Evidence[0].Field = "tool.AppImage:elf.machine"
	setAppImageLauncher(&m)
	if m.LauncherArch != "amd64" {
		t.Fatalf("prefixed launcher evidence lost: %+v", m)
	}
}

func TestMergePartialTargetsDoesNotInventConflict(t *testing.T) {
	x64 := ArtifactMetadata{Status: "inspected", Targets: []ArtifactTarget{{Platform: "windows", Arch: "amd64"}}}
	arm := ArtifactMetadata{Status: "inspected", Targets: []ArtifactTarget{{Platform: "windows", Arch: "arm64"}}, TargetsPartial: true}
	for _, pair := range [][2]ArtifactMetadata{{x64, arm}, {arm, x64}} {
		m := MergeObservations(pair[0], pair[1])
		if len(m.Conflicts) > 0 || m.TargetsPartial || !slices.Equal(m.Targets, x64.Targets) {
			t.Fatalf("complete target list narrowed by sample: %+v", m)
		}
	}
	x64.TargetsPartial = true
	m := MergeObservations(x64, arm)
	if !m.TargetsPartial || len(m.Targets) != 2 || len(m.Conflicts) > 0 {
		t.Fatalf("partial union: %+v", m)
	}
}

func TestDeepExecutableCapKeepsPartialTargets(t *testing.T) {
	for _, count := range []int{1, 65} {
		files := map[string][]byte{}
		for i := 0; i < count; i++ {
			files[fmt.Sprintf("bin/tool%02d", i)] = elfFixture(elf.ELFOSABI_LINUX, elf.ET_EXEC, "", false)
		}
		m, found, err := deepAppFixture(files).inspectExecutables("tool.zip")
		if err != nil || !found || len(m.Targets) != 1 || m.TargetsPartial != (count > 64) {
			t.Fatalf("count=%d: %+v found=%v err=%v", count, m, found, err)
		}
		if count > 64 && deepProven(m) {
			t.Fatal("truncated deep sample became authoritative")
		}
	}
}
