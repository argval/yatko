package github

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
)

// fakeDownloads serves release assets by URL with Range support and fails the
// test if any download request carries the API token.
type fakeDownloads struct {
	t         *testing.T
	files     map[string][]byte
	redirects map[string]string
	status    int
	requests  int
	hosts     []string
}

func newFakeDownloads(t *testing.T) *fakeDownloads {
	return &fakeDownloads{t: t, files: map[string][]byte{}, redirects: map[string]string{}}
}

func (s *fakeDownloads) client() *Client {
	return &Client{token: "secret", httpClient: &http.Client{Transport: archiveTransport(s.serve)}}
}

func (s *fakeDownloads) add(name string, data []byte) Asset {
	u := "https://github.com/acme/app/releases/download/v1/" + name
	s.files[u] = data
	return Asset{Name: name, BrowserDownloadURL: u, Size: int64(len(data)), State: "uploaded"}
}

func (s *fakeDownloads) serve(r *http.Request) (*http.Response, error) {
	s.requests++
	s.hosts = append(s.hosts, r.URL.Host)
	if r.Header.Get("Authorization") != "" {
		s.t.Error("token sent to download URL")
	}
	u := r.URL.String()
	if to, ok := s.redirects[u]; ok {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {to}}, Body: http.NoBody}, nil
	}
	data, ok := s.files[u]
	if s.status != 0 || !ok {
		return &http.Response{StatusCode: max(s.status, http.StatusNotFound), Body: http.NoBody}, nil
	}
	var a, b int
	if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &a, &b); err != nil {
		s.t.Fatalf("bad Range %q", r.Header.Get("Range"))
	}
	b = min(b, len(data)-1)
	h := http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", a, b, len(data))}}
	return &http.Response{StatusCode: http.StatusPartialContent, Header: h, Body: io.NopCloser(bytes.NewReader(data[a : b+1]))}, nil
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// elfFixture builds a minimal x86-64 ELF with optional PT_INTERP and a GNU
// ABI-tag note (OS word 0 = Linux).
func elfFixture(osabi elf.OSABI, typ elf.Type, interp string, gnuNote bool) []byte {
	le := binary.LittleEndian
	type seg struct {
		typ  elf.ProgType
		data []byte
	}
	var segs []seg
	if interp != "" {
		segs = append(segs, seg{elf.PT_INTERP, append([]byte(interp), 0)})
	}
	if gnuNote {
		n := make([]byte, 32)
		le.PutUint32(n, 4)
		le.PutUint32(n[4:], 16)
		le.PutUint32(n[8:], 1)
		copy(n[12:], "GNU\x00")
		le.PutUint32(n[20:], 3) // desc: OS 0 (Linux), kernel 3.2.0
		le.PutUint32(n[24:], 2)
		segs = append(segs, seg{elf.PT_NOTE, n})
	}
	h := make([]byte, 64+56*len(segs))
	copy(h, "\x7fELF")
	h[4], h[5], h[6], h[7] = 2, 1, 1, byte(osabi)
	le.PutUint16(h[16:], uint16(typ))
	le.PutUint16(h[18:], uint16(elf.EM_X86_64))
	le.PutUint64(h[32:], 64)
	le.PutUint16(h[54:], 56)
	le.PutUint16(h[56:], uint16(len(segs)))
	off := len(h)
	var body []byte
	for i, s := range segs {
		p := h[64+56*i:]
		le.PutUint32(p, uint32(s.typ))
		le.PutUint64(p[8:], uint64(off))
		le.PutUint64(p[32:], uint64(len(s.data)))
		body = append(body, s.data...)
		off += len(s.data)
	}
	return append(append(h, body...), make([]byte, 1024)...)
}

// machoFixture builds a minimal 64-bit Mach-O; platform 0 omits LC_BUILD_VERSION.
func machoFixture(cpu, filetype, platform uint32) []byte {
	le := binary.LittleEndian
	b := make([]byte, 32, 1024)
	le.PutUint32(b, 0xfeedfacf)
	le.PutUint32(b[4:], cpu)
	le.PutUint32(b[12:], filetype)
	if platform != 0 {
		le.PutUint32(b[16:], 1)
		le.PutUint32(b[20:], 24)
		cmd := make([]byte, 24)
		le.PutUint32(cmd, 0x32)
		le.PutUint32(cmd[4:], 24)
		le.PutUint32(cmd[8:], platform)
		b = append(b, cmd...)
	}
	return append(b, make([]byte, 512)...)
}

func TestSniffBinaryHeaders(t *testing.T) {
	fat := make([]byte, 4096)
	be := binary.BigEndian
	be.PutUint32(fat, 0xcafebabe)
	be.PutUint32(fat[4:], 2)
	slice := machoFixture(0x0100000c, 2, 1)
	for i, a := range [][3]uint32{{0x0100000c, 4096, uint32(len(slice))}, {0x01000007, 1 << 20, 4096}} {
		e := fat[8+20*i:]
		be.PutUint32(e, a[0])
		be.PutUint32(e[8:], a[1])
		be.PutUint32(e[12:], a[2])
	}
	fat = append(fat, slice...) // second slice lies beyond the readable bytes

	for _, tt := range []struct {
		name      string
		data      []byte
		found     bool
		targets   []ArtifactTarget
		role      string
		conflicts bool
	}{
		// Real Go toolchain output (first 8 KiB). Static Go ELF has no OS ABI,
		// interpreter, or ABI note, so the OS must stay unknown.
		{"go linux arm64", fixture(t, "go-linux-arm64.head"), true, []ArtifactTarget{{Arch: "arm64"}}, "application", false},
		{"go darwin arm64", fixture(t, "go-darwin-arm64.head"), true, []ArtifactTarget{{Platform: "macos", Arch: "arm64"}}, "application", false},
		{"go darwin amd64", fixture(t, "go-darwin-amd64.head"), true, []ArtifactTarget{{Platform: "macos", Arch: "amd64"}}, "application", false},
		{"go windows amd64", fixture(t, "go-windows-amd64.head"), true, []ArtifactTarget{{Platform: "windows", Arch: "amd64"}}, "application", false},
		{"glibc interp", elfFixture(0, elf.ET_DYN, "/lib64/ld-linux-x86-64.so.2", false), true, []ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "gnu"}}, "application", false},
		{"musl interp", elfFixture(0, elf.ET_DYN, "/lib/ld-musl-x86_64.so.1", false), true, []ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "musl"}}, "application", false},
		{"static with GNU note", elfFixture(0, elf.ET_EXEC, "", true), true, []ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "static"}}, "application", false},
		{"freebsd osabi", elfFixture(elf.ELFOSABI_FREEBSD, elf.ET_EXEC, "", false), true, []ArtifactTarget{{Platform: "freebsd", Arch: "amd64"}}, "application", false},
		{"contradictory ELF", elfFixture(elf.ELFOSABI_FREEBSD, elf.ET_DYN, "/lib64/ld-linux-x86-64.so.2", false), true, []ArtifactTarget{{Arch: "amd64"}}, "application", true},
		{"shared object", elfFixture(0, elf.ET_DYN, "", false), true, []ArtifactTarget{{Arch: "amd64"}}, "metadata", false},
		{"ios mach-o", machoFixture(0x0100000c, 2, 2), true, []ArtifactTarget{{Platform: "ios", Arch: "arm64"}}, "application", false},
		{"dsym", machoFixture(0x0100000c, 0xa, 1), true, []ArtifactTarget{{Platform: "macos", Arch: "arm64"}}, "debug", false},
		{"mach-o without version", machoFixture(0x01000007, 2, 0), true, []ArtifactTarget{{Arch: "amd64"}}, "application", false},
		{"fat partial", fat, true, []ArtifactTarget{{Platform: "macos", Arch: "arm64"}, {Arch: "amd64"}}, "application", false},
		{"java class", []byte("\xca\xfe\xba\xbe\x00\x00\x00\x34rest"), false, nil, "unknown", false},
		{"MZ text", []byte("MZ is not an executable"), false, nil, "unknown", false},
		{"text", []byte("#!/bin/sh\necho hi\n"), false, nil, "unknown", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := ArtifactMetadata{Role: "unknown"}
			found, err := sniffBinary(bytes.NewReader(tt.data), &m, "")
			if err != nil || found != tt.found {
				t.Fatalf("found=%v err=%v", found, err)
			}
			if !slices.Equal(m.Targets, tt.targets) || m.Role != tt.role || (len(m.Conflicts) > 0) != tt.conflicts {
				t.Fatalf("targets=%v role=%s conflicts=%v", m.Targets, m.Role, m.Conflicts)
			}
		})
	}
}

func TestSniffBinaryMalformed(t *testing.T) {
	for _, data := range [][]byte{[]byte("\x7fELF\x02\x01"), []byte("\xcf\xfa\xed\xfe\x0c\x00"), []byte("\xca\xfe\xba\xbe\x00\x00\x00\x03")} {
		m := ArtifactMetadata{Role: "unknown"}
		if _, err := sniffBinary(bytes.NewReader(data), &m, ""); !errors.Is(err, ErrMalformedArtifact) {
			t.Errorf("%q: err = %v", data, err)
		}
	}
}

func zipFixture(t *testing.T, files map[string][]byte, padding int) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	if padding > 0 {
		f, _ := w.CreateHeader(&zip.FileHeader{Name: "padding.bin", Method: zip.Store})
		f.Write(make([]byte, padding))
	}
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for k := range files {
			if !yield(k) {
				return
			}
		}
	}) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o755)
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(files[name])
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type tarEntry struct {
	name string
	mode int64
	data []byte
}

func tarGzFixture(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write(e.data)
	}
	tw.Close()
	zw.Close()
	return b.Bytes()
}

func debFixture(t *testing.T, controlName string, control []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("!<arch>\n")
	member := func(name string, data []byte) {
		fmt.Fprintf(&b, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", name, 0, 0, 0, "100644", len(data))
		b.Write(data)
		if len(data)%2 == 1 {
			b.WriteByte('\n')
		}
	}
	member("debian-binary", []byte("2.0\n"))
	member(controlName, control)
	member("data.tar.gz", tarGzFixture(t))
	return b.Bytes()
}

func TestInspectArtifact(t *testing.T) {
	mac := fixture(t, "go-darwin-arm64.head")
	musl := elfFixture(0, elf.ET_DYN, "/lib/ld-musl-x86_64.so.1", false)
	control := tarGzFixture(t, tarEntry{"./control", 0o644, []byte("Package: app\nVersion: 1.0\nArchitecture: armhf\n")})
	for _, tt := range []struct {
		name     string
		asset    string
		data     []byte
		status   string
		targets  []ArtifactTarget
		role     string
		evidence string
	}{
		{"large mac zip", "App-mac.zip", zipFixture(t, map[string][]byte{
			"App.app/Contents/Info.plist": []byte("<plist/>"), "App.app/Contents/MacOS/App": mac,
		}, 3*archiveTailLimit), "inspected", []ArtifactTarget{{Platform: "macos", Arch: "arm64"}}, "application", "zip.app_bundle"},
		{"windows zip", "tool.zip", zipFixture(t, map[string][]byte{"tool/tool.exe": fixture(t, "go-windows-amd64.head"), "tool/README.md": []byte("# tool")}, 0),
			"inspected", []ArtifactTarget{{Platform: "windows", Arch: "amd64"}}, "application", "zip:tool/tool.exe:pe.machine"},
		{"source zip", "src.zip", zipFixture(t, map[string][]byte{"src/main.go": bytes.Repeat([]byte("package main\n"), 100)}, 0), "inspected", nil, "unknown", ""},
		{"musl tarball", "tool.tar.gz", tarGzFixture(t, tarEntry{"tool/README.md", 0o644, []byte("hi")}, tarEntry{"tool/tool", 0o755, musl}),
			"inspected", []ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "musl"}}, "application", "tar:tool/tool:elf.interp"},
		{"inflate cap", "bomb.tgz", tarGzFixture(t, tarEntry{"zeros", 0o644, make([]byte, 64<<20)}, tarEntry{"tool", 0o755, musl}),
			"inspected", nil, "unknown", "tar.listing"},
		{"deb", "app_1.0_armhf.deb", debFixture(t, "control.tar.gz", control), "inspected", []ArtifactTarget{{Platform: "linux", Arch: "arm"}}, "installer", "deb.architecture"},
		{"deb xz control", "app.deb", debFixture(t, "control.tar.xz", []byte("xz")), "inspected", nil, "installer", "deb.control_compression"},
		{"bad deb", "app.deb", []byte("!<arch>\n" + strings.Repeat("x", 70)), "failed", nil, "unknown", ""},
		{"standalone", "tool-linux", musl, "inspected", []ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "musl"}}, "application", "elf.interp"},
		{"standalone versioned", "tool-1.2.3", fixture(t, "go-linux-arm64.head"), "inspected", []ArtifactTarget{{Arch: "arm64"}}, "application", "elf.machine"},
		{"unrecognized", "notes", []byte("plain text"), "inspected", nil, "unknown", "format"},
		{"truncated ELF", "tool", []byte("\x7fELF\x02\x01\x01"), "failed", nil, "unknown", ""},
		{"corrupt zip", "x.zip", []byte("PK\x03\x04 nope"), "failed", nil, "unknown", ""},
		{"dmg skipped", "App.dmg", []byte("koly"), "unsupported", nil, "unknown", "inspection.skipped"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newFakeDownloads(t)
			asset := s.add(tt.asset, tt.data)
			m, err := s.client().InspectArtifact(context.Background(), asset)
			if m.Status != tt.status || (err != nil) != (tt.status == "failed") {
				t.Fatalf("status=%s err=%v", m.Status, err)
			}
			if tt.status == "failed" && !errors.Is(err, ErrMalformedArtifact) {
				t.Fatalf("err = %v, want malformed", err)
			}
			if !slices.Equal(m.Targets, tt.targets) || m.Role != tt.role {
				t.Fatalf("targets=%v role=%s evidence=%v", m.Targets, m.Role, m.Evidence)
			}
			if tt.evidence != "" && !slices.ContainsFunc(m.Evidence, func(e ArtifactEvidence) bool { return strings.HasPrefix(e.Field, tt.evidence) }) {
				t.Fatalf("missing %s evidence in %v", tt.evidence, m.Evidence)
			}
			if s.requests > inspectMaxRequests || (tt.status == "unsupported" && s.requests != 0) {
				t.Fatalf("requests = %d", s.requests)
			}
		})
	}
}

func TestInspectArtifactDownloadGuards(t *testing.T) {
	data := elfFixture(0, elf.ET_DYN, "/lib/ld-musl-x86_64.so.1", false)
	cdn := "https://release-assets.githubusercontent.com/github-production-release-asset/1/tool"

	t.Run("follows GitHub CDN redirect", func(t *testing.T) {
		s := newFakeDownloads(t)
		asset := s.add("tool", data)
		s.redirects[asset.BrowserDownloadURL] = cdn
		s.files[cdn] = data
		if m, err := s.client().InspectArtifact(context.Background(), asset); err != nil || m.Status != "inspected" || len(m.Targets) != 1 {
			t.Fatalf("m=%+v err=%v", m, err)
		}
	})
	t.Run("refuses foreign redirect", func(t *testing.T) {
		s := newFakeDownloads(t)
		asset := s.add("tool", data)
		s.redirects[asset.BrowserDownloadURL] = "https://evil.example/tool"
		m, err := s.client().InspectArtifact(context.Background(), asset)
		if err == nil || m.Status != "failed" || errors.Is(err, ErrMalformedArtifact) || slices.Contains(s.hosts, "evil.example") {
			t.Fatalf("m=%+v err=%v hosts=%v", m, err, s.hosts)
		}
	})
	t.Run("HTTP error is a transport failure", func(t *testing.T) {
		s := newFakeDownloads(t)
		s.status = http.StatusInternalServerError
		m, err := s.client().InspectArtifact(context.Background(), s.add("tool", data))
		if err == nil || m.Status != "failed" || errors.Is(err, ErrMalformedArtifact) {
			t.Fatalf("m=%+v err=%v", m, err)
		}
	})
	for _, asset := range []Asset{
		{Name: "tool", BrowserDownloadURL: "https://example.com/acme/app/releases/download/v1/tool", Size: 10},
		{Name: "tool", BrowserDownloadURL: "http://github.com/acme/app/releases/download/v1/tool", Size: 10},
		{Name: "tool", BrowserDownloadURL: "https://github.com/acme/app/archive/v1.zip", Size: 10},
		{Name: "tool", BrowserDownloadURL: "https://github.com/acme/app/releases/download/v1/tool", Size: 10, State: "starter"},
		{Name: "tool", BrowserDownloadURL: "https://github.com/acme/app/releases/download/v1/tool"},
	} {
		s := newFakeDownloads(t)
		if m, err := s.client().InspectArtifact(context.Background(), asset); err != nil || m.Status != "unsupported" || s.requests != 0 {
			t.Errorf("%+v: m=%+v err=%v requests=%d", asset, m, err, s.requests)
		}
	}
}

func TestRemoteFileBudget(t *testing.T) {
	s := newFakeDownloads(t)
	asset := s.add("big", make([]byte, 64<<20))
	f := s.client().openAsset(context.Background(), asset.BrowserDownloadURL, asset.Size)
	buf := make([]byte, 16)
	var err error
	for i := range inspectMaxRequests + 1 {
		if _, err = f.ReadAt(buf, int64(i)<<20); err != nil {
			break
		}
	}
	if !errors.Is(err, errInspectLimit) || s.requests != inspectMaxRequests || f.bytes > inspectMaxBytes {
		t.Fatalf("err=%v requests=%d bytes=%d", err, s.requests, f.bytes)
	}
	// Cached spans are served without new requests.
	if _, err := f.ReadAt(buf, 0); err != nil || s.requests != inspectMaxRequests {
		t.Fatalf("cached read err=%v requests=%d", err, s.requests)
	}
}

func TestELFProgramHeaderBounds(t *testing.T) {
	for _, sizes := range [][2]uint16{{65535, 65535}, {0, 1}, {65535, 256}} {
		data := elfFixture(elf.ELFOSABI_LINUX, elf.ET_EXEC, "", false)
		binary.LittleEndian.PutUint16(data[54:], sizes[0])
		binary.LittleEndian.PutUint16(data[56:], sizes[1])
		m := ArtifactMetadata{Role: "unknown"}
		if _, err := sniffBinary(bytes.NewReader(data), &m, ""); err != nil {
			t.Fatalf("header sizes %v: %v", sizes, err)
		}
		if len(m.Targets) != 1 || m.Targets[0].Arch != "amd64" {
			t.Fatalf("lost bounded header evidence: %+v", m)
		}
	}
}

func TestUnsupportedHeaderConstraintsRemainExplicit(t *testing.T) {
	data := elfFixture(elf.ELFOSABI_SOLARIS, elf.ET_EXEC, "", false)
	binary.LittleEndian.PutUint16(data[18:], uint16(elf.EM_PPC64))
	m := ArtifactMetadata{Role: "unknown"}
	if _, err := sniffBinary(bytes.NewReader(data), &m, ""); err != nil {
		t.Fatal(err)
	}
	if len(m.Targets) != 1 || m.Targets[0].Arch != "elf:21" || m.Targets[0].Platform != "elf-osabi:6" {
		t.Fatalf("explicit incompatible constraints became wildcards: %+v", m)
	}
	for _, arch := range []string{"kfreebsd-amd64", "hurd-i386"} {
		control := tarGzFixture(t, tarEntry{"control", 0o644, []byte("Architecture: " + arch + "\n")})
		m := ArtifactMetadata{Role: "unknown"}
		if err := debControl(bytes.NewReader(control), ".gz", &m); err != nil {
			t.Fatal(err)
		}
		kernel, _, _ := strings.Cut(arch, "-")
		if len(m.Targets) != 1 || m.Targets[0].Platform != kernel || m.Targets[0].Arch == "" {
			t.Fatalf("lost non-Linux Debian target: %+v", m)
		}
	}
}

func TestPECLRAndLibraryConstraints(t *testing.T) {
	base := fixture(t, "go-windows-amd64.head")
	pe := int(binary.LittleEndian.Uint32(base[0x3c:]))
	for _, tc := range []struct {
		name       string
		machine    uint16
		dll, clr   bool
		arch, role string
	}{
		{"CLR x86 is not proven AnyCPU", 0x14c, false, true, "386", "application"},
		{"DLL is a helper", 0x8664, true, false, "amd64", "metadata"},
		{"unknown machine stays explicit", 0x9999, false, false, "pe:0x9999", "application"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := slices.Clone(base)
			binary.LittleEndian.PutUint16(data[pe+4:], tc.machine)
			if tc.dll {
				binary.LittleEndian.PutUint16(data[pe+22:], 0x2002)
			}
			if tc.clr {
				binary.LittleEndian.PutUint16(data[pe+24:], 0x10b)
				binary.LittleEndian.PutUint32(data[pe+24+96+14*8:], 0x1234)
			}
			m := ArtifactMetadata{Role: "unknown"}
			if _, err := sniffBinary(bytes.NewReader(data), &m, ""); err != nil {
				t.Fatal(err)
			}
			if len(m.Targets) != 1 || m.Targets[0].Arch != tc.arch || m.Role != tc.role {
				t.Fatalf("%+v", m)
			}
		})
	}
}

func TestArchiveMainEntryRefinesOnlyItsOwnMacTarget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		main     []byte
		target   ArtifactTarget
		role     string
		conflict bool
	}{
		{"Mach-O without OS command", machoFixture(0x01000007, 2, 0), ArtifactTarget{Platform: "macos", Arch: "amd64"}, "application", false},
		{"ELF cannot inherit macOS", elfFixture(0, elf.ET_EXEC, "", false), ArtifactTarget{Arch: "amd64"}, "application", true},
		{"dylib is not an application", machoFixture(0x01000007, 6, 1), ArtifactTarget{Platform: "macos", Arch: "amd64"}, "metadata", false},
		{"unknown Mach-O platform", machoFixture(0x01000007, 2, 99), ArtifactTarget{Platform: "macho-platform:99", Arch: "amd64"}, "application", true},
		{"unsupported Mach-O CPU", machoFixture(0x01000012, 2, 1), ArtifactTarget{Platform: "macos", Arch: "macho:0x1000012"}, "application", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeDownloads(t)
			a := s.add("App.zip", zipFixture(t, map[string][]byte{"App.app/Contents/Info.plist": []byte("<plist/>"), "App.app/Contents/MacOS/App": tc.main}, 0))
			m, err := s.client().InspectArtifact(context.Background(), a)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(m.Targets, []ArtifactTarget{tc.target}) || m.Role != tc.role || (len(m.Conflicts) > 0) != tc.conflict {
				t.Fatalf("%+v", m)
			}
		})
	}
}

func TestApplicationRoleSurvivesBundledHelpersAndSymbols(t *testing.T) {
	app := machoFixture(0x01000007, 2, 1)
	for _, helper := range [][]byte{machoFixture(0x01000007, 6, 1), machoFixture(0x01000007, 0xa, 1)} {
		for _, entries := range [][][]byte{{app, helper}, {helper, app}} {
			m := ArtifactMetadata{Role: "unknown"}
			for _, data := range entries {
				if _, err := sniffBinary(bytes.NewReader(data), &m, ""); err != nil {
					t.Fatal(err)
				}
			}
			if m.Role != "application" {
				t.Fatalf("helper displaced proven application: %+v", m)
			}
		}
	}
}

func TestGenericArchiveLibraryTargetsDoNotBecomeApplicationTargets(t *testing.T) {
	app := elfFixture(elf.ELFOSABI_LINUX, elf.ET_EXEC, "", false)
	library := fixture(t, "go-windows-amd64.head")
	pe := int(binary.LittleEndian.Uint32(library[0x3c:]))
	binary.LittleEndian.PutUint16(library[pe+22:], 0x2002)
	for _, entries := range [][][]byte{{app, library}, {library, app}} {
		m := ArtifactMetadata{Role: "unknown"}
		for _, entry := range entries {
			if err := inspectArchiveEntry(entry, &m, "zip:entry", "entry", ""); err != nil {
				t.Fatal(err)
			}
		}
		if m.Role != "application" || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "static"}}) {
			t.Fatalf("library invented a Windows application: %+v", m)
		}
	}
}

func TestMacArchiveHelpersDoNotExpandMainCPUs(t *testing.T) {
	s := newFakeDownloads(t)
	a := s.add("App.zip", zipFixture(t, map[string][]byte{
		"App.app/Contents/Info.plist":        []byte("<plist/>"),
		"App.app/Contents/MacOS/App":         machoFixture(0x01000007, 2, 1),
		"App.app/Contents/Frameworks/helper": machoFixture(0x0100000c, 2, 1),
	}, 0))
	m, err := s.client().InspectArtifact(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "macos", Arch: "amd64"}}) {
		t.Fatalf("helper expanded main CPU targets: %+v", m)
	}
}

func TestTarAppCombinesOnlyMatchingMainHeader(t *testing.T) {
	s := newFakeDownloads(t)
	a := s.add("App.tar.gz", tarGzFixture(t,
		tarEntry{"App.app/Contents/Info.plist", 0o644, []byte("<plist/>")},
		tarEntry{"App.app/Contents/MacOS/App", 0o755, machoFixture(0x01000007, 2, 0)},
	))
	m, err := s.client().InspectArtifact(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "macos", Arch: "amd64"}}) || m.Role != "application" {
		t.Fatalf("%+v", m)
	}
}
