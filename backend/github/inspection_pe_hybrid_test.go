package github

import (
	"bytes"
	"encoding/binary"
	"io"
	"slices"
	"testing"
)

// hybridPEFixture puts the load config and CHPE metadata in a file-backed section.
// Its x64 header deliberately stays identical for native x64 and ARM64EC images.
func hybridPEFixture(machine uint16, version uint32, hybrid bool) []byte {
	b := make([]byte, 0x800)
	le := binary.LittleEndian
	copy(b, "MZ")
	le.PutUint32(b[0x3c:], 0x80)
	copy(b[0x80:], "PE\x00\x00")
	le.PutUint16(b[0x84:], machine)
	le.PutUint16(b[0x86:], 1)
	le.PutUint16(b[0x94:], 240)
	le.PutUint16(b[0x96:], 2)
	opt := b[0x98:]
	le.PutUint16(opt, 0x20b)
	le.PutUint64(opt[24:], 0x140000000)
	le.PutUint32(opt[56:], 0x2000)
	le.PutUint32(opt[60:], 0x200)
	le.PutUint16(opt[68:], 2)
	le.PutUint32(opt[108:], 16)
	le.PutUint32(opt[192:], 0x1100)
	le.PutUint32(opt[196:], 208)
	section := b[0x188:]
	copy(section, ".rdata")
	le.PutUint32(section[8:], 0x600)
	le.PutUint32(section[12:], 0x1000)
	le.PutUint32(section[16:], 0x600)
	le.PutUint32(section[20:], 0x200)
	le.PutUint32(b[0x300:], 208)
	if hybrid {
		le.PutUint64(b[0x300+200:], 0x140001200)
		le.PutUint32(b[0x400:], version)
		le.PutUint32(b[0x404:], 0x1300)
		le.PutUint32(b[0x408:], 1)
		le.PutUint32(b[0x500:], 0x1001)
		le.PutUint32(b[0x504:], 8)
	}
	return b
}

func TestPEHybridArchitecture(t *testing.T) {
	for _, tc := range []struct {
		name    string
		machine uint16
		version uint32
		hybrid  bool
		arch    string
		kind    string
	}{
		{"native x64", 0x8664, 0, false, "amd64", ""},
		{"ARM64EC v1", 0x8664, 1, true, "arm64", "arm64ec"},
		{"ARM64EC v2", 0x8664, 2, true, "arm64", "arm64ec"},
		{"native ARM64", 0xaa64, 0, false, "arm64", ""},
		{"ARM64X v1", 0xaa64, 1, true, "arm64", "arm64x"},
		{"ARM64X v2", 0xaa64, 2, true, "arm64", "arm64x"},
		{"intermediate COFF is not final ARM64EC", 0xa641, 0, false, "pe:0xa641", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := ArtifactMetadata{Role: "unknown"}
			found, err := readPE(bytes.NewReader(hybridPEFixture(tc.machine, tc.version, tc.hybrid)), &m, "app.exe")
			if err != nil || !found || m.Role != "application" || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "windows", Arch: tc.arch}}) {
				t.Fatalf("found=%v err=%v metadata=%+v", found, err, m)
			}
			var kind string
			for _, e := range m.Evidence {
				if e.Field == "app.exe:pe.hybrid" {
					kind = e.Value
				}
			}
			if kind != tc.kind {
				t.Fatalf("hybrid=%q want=%q; %+v", kind, tc.kind, m)
			}
		})
	}
}

func TestPEHybridMalformedAndPartialMetadata(t *testing.T) {
	le := binary.LittleEndian
	for _, tc := range []struct {
		name string
		edit func([]byte) []byte
		kind string
	}{
		{"too many sections", func(b []byte) []byte { le.PutUint16(b[0x86:], 65535); return b }, "unreadable load configuration"},
		{"section table outside headers", func(b []byte) []byte { le.PutUint16(b[0x94:], 65535); return b }, "unreadable load configuration"},
		{"overlapping sections", func(b []byte) []byte {
			le.PutUint16(b[0x86:], 2)
			copy(b[0x188+40:], b[0x188:0x188+40])
			return b
		}, "unreadable load configuration"},
		{"load config RVA overflow", func(b []byte) []byte { le.PutUint32(b[0x98+192:], 0xfffffff0); return b }, "unreadable load configuration"},
		{"load config in virtual padding", func(b []byte) []byte { le.PutUint32(b[0x188+16:], 0x100); return b }, "unreadable load configuration"},
		{"load config prefix only", func(b []byte) []byte { return b[:0x350] }, "unreadable load configuration"},
		{"older load config", func(b []byte) []byte { le.PutUint32(b[0x300:], 200); return b }, ""},
		{"pointer below image base", func(b []byte) []byte { le.PutUint64(b[0x300+200:], 1); return b }, "unreadable CHPE metadata"},
		{"pointer exceeds RVA space", func(b []byte) []byte { le.PutUint64(b[0x300+200:], 0x240000000); return b }, "unreadable CHPE metadata"},
		{"metadata crosses section end", func(b []byte) []byte { le.PutUint64(b[0x300+200:], 0x1400015e0); return b }, "unreadable CHPE metadata"},
		{"metadata prefix only", func(b []byte) []byte { return b[:0x420] }, "unreadable CHPE metadata"},
		{"version two suffix only missing", func(b []byte) []byte { le.PutUint32(b[0x400:], 2); return b[:0x450] }, "unreadable CHPE metadata"},
		{"unknown metadata version", func(b []byte) []byte { le.PutUint32(b[0x400:], 99); return b }, "unsupported CHPE metadata version"},
		{"table length overflow", func(b []byte) []byte { le.PutUint32(b[0x408:], 0xffffffff); return b }, "unreadable CHPE metadata"},
		{"table RVA overflow", func(b []byte) []byte { le.PutUint32(b[0x404:], 0xfffffffc); return b }, "unreadable CHPE metadata"},
		{"missing table RVA", func(b []byte) []byte { le.PutUint32(b[0x404:], 0); return b }, "unreadable CHPE metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &peReadCounter{ReaderAt: bytes.NewReader(tc.edit(hybridPEFixture(0x8664, 1, true)))}
			m := ArtifactMetadata{Role: "unknown"}
			found, err := readPE(r, &m, "")
			if err != nil || !found || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "windows", Arch: "amd64"}}) {
				t.Fatalf("lost shallow header evidence: found=%v err=%v metadata=%+v", found, err, m)
			}
			var kind string
			for _, e := range m.Evidence {
				if e.Field == "pe.hybrid" {
					kind = e.Value
				}
			}
			if kind != tc.kind || r.bytes > 64+264+96*40+208+92 || r.calls > 8 {
				t.Fatalf("kind=%q want=%q read=%d calls=%d", kind, tc.kind, r.bytes, r.calls)
			}
		})
	}
}

func TestPEHybridReadsBeyondArchivePrefix(t *testing.T) {
	base := hybridPEFixture(0x8664, 1, true)
	data := make([]byte, len(base)+2*inspectHeadBytes)
	copy(data, base[:0x200])
	copy(data[0x200+2*inspectHeadBytes:], base[0x200:])
	binary.LittleEndian.PutUint32(data[0x188+20:], 0x200+2*inspectHeadBytes)
	for _, tc := range []struct {
		name string
		data []byte
		arch string
		kind string
	}{
		{"full file", data, "arm64", "arm64ec"},
		{"shallow archive prefix", data[:inspectHeadBytes], "amd64", "unreadable load configuration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &peReadCounter{ReaderAt: bytes.NewReader(tc.data)}
			m := ArtifactMetadata{Role: "unknown"}
			if _, err := readPE(r, &m, ""); err != nil || len(m.Targets) != 1 || m.Targets[0].Arch != tc.arch {
				t.Fatalf("%+v err=%v", m, err)
			}
			if !slices.Contains(m.Evidence, ArtifactEvidence{Source: "inspection", Field: "pe.hybrid", Value: tc.kind}) || r.bytes > 64+264+96*40+208+92 {
				t.Fatalf("hybrid evidence missing or read exceeded bound: %+v bytes=%d", m, r.bytes)
			}
		})
	}
}

func TestPEHybridKeepsEFITargetAndLibraryRole(t *testing.T) {
	data := hybridPEFixture(0x8664, 1, true)
	binary.LittleEndian.PutUint16(data[0x98+68:], 10)
	m := ArtifactMetadata{Role: "unknown"}
	if _, err := readPE(bytes.NewReader(data), &m, ""); err != nil || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "uefi", Arch: "amd64"}}) {
		t.Fatalf("EFI target changed: %+v err=%v", m, err)
	}
	data = hybridPEFixture(0x8664, 1, true)
	binary.LittleEndian.PutUint16(data[0x96:], 0x2002)
	m = ArtifactMetadata{Role: "unknown"}
	if err := inspectArchiveEntry(data, &m, "helper.dll", "helper.dll", ""); err != nil || len(m.Targets) != 0 || m.Role != "metadata" {
		t.Fatalf("hybrid DLL proved an application: %+v err=%v", m, err)
	}
}

type peReadCounter struct {
	io.ReaderAt
	bytes, calls int
}

func (r *peReadCounter) ReadAt(p []byte, off int64) (int, error) {
	r.bytes += len(p)
	r.calls++
	return r.ReaderAt.ReadAt(p, off)
}
