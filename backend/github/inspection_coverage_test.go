package github

import (
	"context"
	"debug/elf"
	"testing"
)

// Exercise suffix dispatch and status handling through the public inspector.
func TestExtendedInspectionFormats(t *testing.T) {
	binary := elfFixture(elf.ELFOSABI_LINUX, elf.ET_EXEC, "", false)
	tarball := plainTar(t, tarEntry{"tool", 0o755, binary})
	control := plainTar(t, tarEntry{"control", 0o644, []byte("Package: tool\nArchitecture: arm64\n")})
	for _, tc := range []struct {
		name           string
		data           []byte
		platform, arch string
	}{
		{"tool.tar.xz", xzFixture(t, tarball, 0), "linux", "amd64"},
		{"tool.txz", xzFixture(t, tarball, 0), "linux", "amd64"},
		{"tool.tar.zst", zstdFixture(t, tarball), "linux", "amd64"},
		{"tool.tzst", zstdFixture(t, tarball), "linux", "amd64"},
		{"tool.xz", xzFixture(t, binary, 0), "linux", "amd64"},
		{"tool.zst", zstdFixture(t, binary), "linux", "amd64"},
		{"tool.7z", sevenZipFixture(t, szOptions{}, tarEntry{"tool", 0o755, binary}), "linux", "amd64"},
		{"tool.rpm", rpmFixture("aarch64", "linux"), "linux", "arm64"},
		{"tool.msi", msiFixture("Arm64;1033", true), "windows", "arm64"},
		{"tool.dmg", dmgFixture(), "macos", ""},
		{"xz-control.deb", debFixture(t, "control.tar.xz", xzFixture(t, control, 0)), "linux", "arm64"},
		{"zstd-control.deb", debFixture(t, "control.tar.zst", zstdFixture(t, control)), "linux", "arm64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeDownloads(t)
			m, err := s.client().InspectArtifact(context.Background(), s.add(tc.name, tc.data))
			if err != nil || m.Status != "inspected" || len(m.Targets) != 1 || m.Targets[0].Platform != tc.platform || m.Targets[0].Arch != tc.arch {
				t.Fatalf("%+v, %v", m, err)
			}
		})
	}
}
