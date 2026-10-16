package github

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"unicode/utf16"
)

// installerFile uses cached bytes so parser tests never make network requests.
func installerFile(b []byte) *remoteFile {
	return &remoteFile{size: int64(len(b)), spans: []span{{off: 0, data: b}}}
}

func rpmFixture(arch, os string) []byte {
	be := binary.BigEndian
	lead := make([]byte, 96)
	copy(lead, []byte{0xed, 0xab, 0xee, 0xdb, 3})
	header := func(values []string) []byte {
		size := 0
		for _, v := range values {
			size += len(v) + 1
		}
		h := make([]byte, 16+16*len(values)+size)
		copy(h, []byte{0x8e, 0xad, 0xe8, 1})
		be.PutUint32(h[8:], uint32(len(values)))
		be.PutUint32(h[12:], uint32(size))
		off := 0
		for i, v := range values {
			e := h[16+i*16:]
			be.PutUint32(e, uint32(1021+i))
			be.PutUint32(e[4:], 6)
			be.PutUint32(e[8:], uint32(off))
			be.PutUint32(e[12:], 1)
			copy(h[16+16*len(values)+off:], v)
			off += len(v) + 1
		}
		return h
	}
	return append(append(lead, header(nil)...), header([]string{os, arch})...)
}

func TestRPMInspection(t *testing.T) {
	for _, tc := range []struct{ arch, os, wantArch, wantOS string }{{"x86_64", "linux", "amd64", "linux"}, {"aarch64", "linux", "arm64", "linux"}, {"armhfp", "linux", "arm", "linux"}, {"riscv64", "freebsd", "rpm:riscv64", "rpm:freebsd"}, {"noarch", "linux", "", "linux"}} {
		t.Run(tc.arch, func(t *testing.T) {
			var m ArtifactMetadata
			if err := inspectRPM(installerFile(rpmFixture(tc.arch, tc.os)), &m); err != nil {
				t.Fatal(err)
			}
			if len(m.Targets) != 1 || m.Targets[0].Arch != tc.wantArch || m.Targets[0].Platform != tc.wantOS || m.Role != "installer" {
				t.Fatalf("%+v", m)
			}
		})
	}
	b := rpmFixture("amd64", "linux")
	binary.BigEndian.PutUint32(b[120:], 0xffffffff)
	if err := inspectRPM(installerFile(b), &ArtifactMetadata{}); !errors.Is(err, errInspectLimit) {
		t.Fatalf("huge count: %v", err)
	}
	b = rpmFixture("amd64", "linux")
	b = b[:len(b)-1]
	if err := inspectRPM(installerFile(b), &ArtifactMetadata{}); err == nil {
		t.Fatal("accepted truncated header")
	}
	b = rpmFixture("amd64", "linux")
	binary.BigEndian.PutUint32(b[136:], 0xffffffff)
	if err := inspectRPM(installerFile(b), &ArtifactMetadata{}); err == nil {
		t.Fatal("accepted invalid string offset")
	}
	b = rpmFixture("amd64", "linux")
	b[4] = 4
	var m ArtifactMetadata
	if err := inspectRPM(installerFile(b), &m); err != nil || m.Status != "unsupported" {
		t.Fatalf("version: %+v %v", m, err)
	}
}

func dmgFixture() []byte {
	b := make([]byte, 1024)
	h := b[512:]
	copy(h, "koly")
	be := binary.BigEndian
	be.PutUint32(h[4:], 4)
	be.PutUint32(h[8:], 512)
	be.PutUint64(h[32:], 512)
	be.PutUint32(h[56:], 1)
	be.PutUint32(h[60:], 1)
	return b
}

func TestDMGInspection(t *testing.T) {
	b := dmgFixture()
	copy(b, "\xcf\xfa\xed\xfe") // Raw bytes are not evidence of an embedded executable.
	var m ArtifactMetadata
	if err := inspectDMG(installerFile(b), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Targets) != 1 || m.Targets[0].Platform != "macos" || m.Targets[0].Arch != "" || m.Role == "installer" {
		t.Fatalf("%+v", m)
	}
	b = dmgFixture()
	binary.BigEndian.PutUint64(b[512+224:], ^uint64(0))
	if err := inspectDMG(installerFile(b), &ArtifactMetadata{}); err == nil {
		t.Fatal("accepted overflowing fork")
	}
	b = dmgFixture()
	binary.BigEndian.PutUint32(b[512+60:], 2)
	m = ArtifactMetadata{}
	if err := inspectDMG(installerFile(b), &m); err != nil || m.Status != "unsupported" {
		t.Fatalf("segmented: %+v %v", m, err)
	}
	b = dmgFixture()
	copy(b, "encrcdsa")
	m = ArtifactMetadata{}
	if err := inspectDMG(installerFile(b), &m); err != nil || m.Status != "unsupported" || len(m.Targets) != 0 {
		t.Fatalf("encrypted: %+v %v", m, err)
	}
}

// msiFixture makes a valid v3 CFB with a summary in either normal or mini sectors.
func msiFixture(template string, mini bool) []byte {
	le := binary.LittleEndian
	size := 4096
	if mini {
		size = 256
	}
	b := make([]byte, 512+512+512+4096)
	h := b[:512]
	copy(h, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})
	le.PutUint16(h[26:], 3)
	le.PutUint16(h[28:], 0xfffe)
	le.PutUint16(h[30:], 9)
	le.PutUint16(h[32:], 6)
	le.PutUint32(h[44:], 1)
	le.PutUint32(h[48:], 1)
	le.PutUint32(h[56:], 4096)
	le.PutUint32(h[60:], cfbEnd)
	le.PutUint32(h[68:], cfbEnd)
	for i := 0; i < 109; i++ {
		le.PutUint32(h[76+i*4:], 0xffffffff)
	}
	le.PutUint32(h[76:], 0)
	fat := b[512:1024]
	for i := 0; i < 128; i++ {
		le.PutUint32(fat[i*4:], 0xffffffff)
	}
	le.PutUint32(fat, 0xfffffffd)
	le.PutUint32(fat[4:], cfbEnd)
	dir := b[1024:1536]
	dir[66] = 5
	copy(dir[80:], []byte{0x84, 0x10, 0x0c, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46})
	le.PutUint32(dir[116:], cfbEnd)
	e := dir[128:]
	name := utf16.Encode([]rune("\x05SummaryInformation\x00"))
	for i, v := range name {
		le.PutUint16(e[i*2:], v)
	}
	le.PutUint16(e[64:], uint16(len(name)*2))
	e[66] = 2
	le.PutUint32(e[116:], 2)
	le.PutUint64(e[120:], uint64(size))
	for i := 2; i < 10; i++ {
		le.PutUint32(fat[i*4:], uint32(i+1))
	}
	le.PutUint32(fat[9*4:], cfbEnd)
	s := b[1536 : 1536+size]
	if mini {
		// sector 2 is miniFAT, sector 3 stores four 64-byte mini sectors.
		le.PutUint32(h[60:], 2)
		le.PutUint32(h[64:], 1)
		le.PutUint32(fat[8:], cfbEnd)
		le.PutUint32(fat[12:], cfbEnd)
		le.PutUint32(dir[116:], 3)
		le.PutUint64(dir[120:], 512)
		le.PutUint32(e[116:], 0)
		mf := b[1536:2048]
		for i := 0; i < 128; i++ {
			le.PutUint32(mf[i*4:], 0xffffffff)
		}
		for i := 0; i < 3; i++ {
			le.PutUint32(mf[i*4:], uint32(i+1))
		}
		le.PutUint32(mf[12:], cfbEnd)
		s = b[2048:2304]
	}
	le.PutUint16(s, 0xfffe)
	le.PutUint32(s[24:], 1)
	copy(s[28:], []byte{0xe0, 0x85, 0x9f, 0xf2, 0xf9, 0x4f, 0x68, 0x10, 0xab, 0x91, 0x08, 0, 0x2b, 0x27, 0xb3, 0xd9})
	le.PutUint32(s[44:], 48)
	section := s[48:]
	le.PutUint32(section, uint32(len(section)))
	le.PutUint32(section[4:], 1)
	le.PutUint32(section[8:], 7)
	le.PutUint32(section[12:], 16)
	le.PutUint32(section[16:], 30)
	le.PutUint32(section[20:], uint32(len(template)+1))
	copy(section[24:], template)
	return b
}

func TestMSIInspection(t *testing.T) {
	for _, mini := range []bool{false, true} {
		for _, tc := range []struct{ template, arch string }{{"x64;1033", "amd64"}, {"Arm64;1033", "arm64"}, {"Intel;1033", "386"}, {";1033", "386"}, {"Intel64;1033", "msi:intel64"}} {
			t.Run(tc.template+map[bool]string{true: "mini", false: "regular"}[mini], func(t *testing.T) {
				var m ArtifactMetadata
				if err := inspectMSI(installerFile(msiFixture(tc.template, mini)), &m); err != nil {
					t.Fatal(err)
				}
				if len(m.Targets) != 1 || m.Targets[0] != (ArtifactTarget{Platform: "windows", Arch: tc.arch}) || m.Role != "installer" {
					t.Fatalf("%+v", m)
				}
			})
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func([]byte)
	}{
		{"directory loop", func(b []byte) { binary.LittleEndian.PutUint32(b[512+4:], 1) }},
		{"regular stream loop", func(b []byte) { binary.LittleEndian.PutUint32(b[512+9*4:], 2) }},
		{"property count", func(b []byte) { binary.LittleEndian.PutUint32(b[1536+48+4:], 0xffffffff) }},
		{"property offset", func(b []byte) { binary.LittleEndian.PutUint32(b[1536+48+12:], 0xffffffff) }},
		{"string length", func(b []byte) { binary.LittleEndian.PutUint32(b[1536+48+20:], 0xffffffff) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := msiFixture("x64;1033", false)
			tc.mutate(b)
			if err := inspectMSI(installerFile(b), &ArtifactMetadata{}); err == nil {
				t.Fatal("accepted malformed CFB")
			}
		})
	}
	b := msiFixture("Arm64;1033", true)
	binary.LittleEndian.PutUint32(b[1536+12:], 0)
	if err := inspectMSI(installerFile(b), &ArtifactMetadata{}); err == nil {
		t.Fatal("accepted miniFAT loop")
	}
	b = msiFixture("x64;1033", false)
	binary.LittleEndian.PutUint32(b[44:], 110)
	var m ArtifactMetadata
	if err := inspectMSI(installerFile(b), &m); err != nil || m.Status != "unsupported" {
		t.Fatalf("extended DIFAT: %+v %v", m, err)
	}
	b = msiFixture("x64;1033", false)
	copy(b[1024+80:1024+96], bytes.Repeat([]byte{0}, 16))
	m = ArtifactMetadata{}
	if err := inspectMSI(installerFile(b), &m); err != nil || m.Status != "unsupported" || len(m.Targets) > 0 {
		t.Fatalf("non-MSI: %+v %v", m, err)
	}
	b = msiFixture("x64;1033", false)
	if err := inspectMSI(installerFile(b[:1800]), &ArtifactMetadata{}); err == nil {
		t.Fatal("accepted truncated CFB")
	}
}

// Large RPM stores are common; only the architecture strings need to be read.
func TestRPMReadsTargetFromLargeStore(t *testing.T) {
	b := rpmFixture("x86_64", "linux")
	const packageHeader = 112
	size := binary.BigEndian.Uint32(b[packageHeader+12:])
	binary.BigEndian.PutUint32(b[packageHeader+12:], size+1<<20)
	b = append(b, make([]byte, 1<<20)...)
	var m ArtifactMetadata
	if err := inspectRPM(installerFile(b), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Targets) != 1 || m.Targets[0].Arch != "amd64" {
		t.Fatalf("%+v", m)
	}
}
