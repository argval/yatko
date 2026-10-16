package github

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

// installerUnsupported preserves partial facts while naming the unsupported variant.
func installerUnsupported(m *ArtifactMetadata, reason string) error {
	m.Status = "unsupported"
	note(m, "", "inspection.unsupported", reason)
	return nil
}

// inspectRPM reads the v3 lead and bounded signature/package headers, never the payload.
// Layout and tags: rpm.org manual/format_header.html and manual/tags.html.
func inspectRPM(f *remoteFile, m *ArtifactMetadata) error {
	var lead [96]byte
	if _, err := f.ReadAt(lead[:], 0); err != nil {
		return err
	}
	if !bytes.Equal(lead[:4], []byte{0xed, 0xab, 0xee, 0xdb}) {
		return malformed("RPM lead")
	}
	if lead[4] != 3 {
		return installerUnsupported(m, "RPM lead version")
	}
	kind := binary.BigEndian.Uint16(lead[6:])
	if kind > 1 {
		return malformed("RPM package type")
	}
	_, end, err := rpmHeader(f, 96)
	if err != nil {
		return err
	}
	tags, _, err := rpmHeader(f, (end+7)&^7)
	if err != nil {
		return err
	}
	m.Role = "installer"
	if kind == 1 {
		m.Role = "metadata"
	}
	target := ArtifactTarget{}
	if os := tags[1021]; os != "" {
		note(m, "", "rpm.os", os)
		target.Platform = "rpm:" + strings.ToLower(os)
		if strings.EqualFold(os, "linux") {
			target.Platform = "linux"
		}
		if strings.EqualFold(os, "darwin") {
			target.Platform = "macos"
		}
	}
	if arch := tags[1022]; arch != "" {
		note(m, "", "rpm.arch", arch)
		target.Arch = installerArch(arch, "rpm:")
		if arch == "noarch" {
			target.Arch = ""
		}
	}
	if kind == 0 && (target.Platform != "" || target.Arch != "") {
		addTarget(m, target)
	}
	return nil
}

// rpmHeader validates index offsets before extracting the two scalar string tags.
func rpmHeader(r io.ReaderAt, off int64) (map[uint32]string, int64, error) {
	var h [16]byte
	if _, err := r.ReadAt(h[:], off); err != nil {
		return nil, 0, err
	}
	if !bytes.Equal(h[:4], []byte{0x8e, 0xad, 0xe8, 1}) {
		return nil, 0, malformed("RPM header")
	}
	n, size := binary.BigEndian.Uint32(h[8:]), binary.BigEndian.Uint32(h[12:])
	if n > 4096 || size > 64<<20 {
		return nil, 0, errInspectLimit
	}
	data := make([]byte, int(n)*16)
	if _, err := r.ReadAt(data, off+16); err != nil {
		return nil, 0, err
	}
	storeOffset := off + 16 + int64(n)*16
	tags := map[uint32]string{}
	for i := uint32(0); i < n; i++ {
		e := data[i*16:]
		tag, typ, pos, count := binary.BigEndian.Uint32(e), binary.BigEndian.Uint32(e[4:]), binary.BigEndian.Uint32(e[8:]), binary.BigEndian.Uint32(e[12:])
		if pos > size {
			return nil, 0, malformed("RPM index offset")
		}
		if tag != 1021 && tag != 1022 {
			continue
		}
		if typ != 6 || count != 1 || pos == size {
			return nil, 0, malformed("RPM target tag")
		}
		value := make([]byte, min(129, size-pos))
		if _, err := r.ReadAt(value, storeOffset+int64(pos)); err != nil {
			return nil, 0, err
		}
		end := bytes.IndexByte(value, 0)
		if end < 0 || end > 128 {
			return nil, 0, malformed("RPM target string")
		}
		if _, exists := tags[tag]; exists {
			return nil, 0, malformed("duplicate RPM target tag")
		}
		tags[tag] = string(value[:end])
	}
	return tags, storeOffset + int64(size), nil
}

// installerArch keeps unrecognized CPUs explicit so they cannot become wildcards.
func installerArch(s, prefix string) string {
	switch strings.ToLower(s) {
	case "x86_64", "amd64", "x64":
		return "amd64"
	case "i386", "i486", "i586", "i686", "x86", "intel":
		return "386"
	case "aarch64", "arm64":
		return "arm64"
	case "arm", "armv7hl", "armv7l", "armhfp", "armv7hnl", "armv6hl":
		return "arm"
	default:
		return prefix + strings.ToLower(s)
	}
}

// inspectDMG validates the UDIF resource trailer. Its format proves macOS, not CPU.
// It does not mount filesystems or guess CPU from compressed/raw byte patterns.
func inspectDMG(f *remoteFile, m *ArtifactMetadata) error {
	var first [8]byte
	if _, err := f.ReadAt(first[:], 0); err != nil {
		return err
	}
	if string(first[:]) == "encrcdsa" || string(first[:]) == "cdsaencr" {
		return installerUnsupported(m, "encrypted DMG")
	}
	if f.size < 512 {
		return malformed("UDIF trailer")
	}
	var h [512]byte
	if _, err := f.ReadAt(h[:], f.size-512); err != nil {
		return err
	}
	if string(h[:4]) != "koly" {
		return installerUnsupported(m, "DMG without UDIF trailer")
	}
	be := binary.BigEndian
	if be.Uint32(h[4:]) != 4 {
		return installerUnsupported(m, "UDIF version")
	}
	if be.Uint32(h[8:]) != 512 {
		return malformed("UDIF resource size")
	}
	for _, pos := range []int{24, 40, 216} {
		off, size := be.Uint64(h[pos:]), be.Uint64(h[pos+8:])
		if off > uint64(f.size-512) || size > uint64(f.size-512)-off {
			return malformed("UDIF fork bounds")
		}
	}
	note(m, "", "dmg.format", "UDIF v4")
	addTarget(m, ArtifactTarget{Platform: "macos"})
	// A disk image can contain documents; its container alone does not prove an installer role.
	if be.Uint32(h[60:]) > 1 || be.Uint32(h[56:]) > 1 {
		return installerUnsupported(m, "segmented UDIF image")
	}
	note(m, "", "dmg.cpu", "unknown: filesystem contents not inspected")
	return nil
}

const cfbEnd uint32 = 0xfffffffe

// msiCompound reads only bounded CFB allocation tables and named summary streams.
// Extended DIFAT is unsupported; FAT and miniFAT chains have size and cycle checks.
type msiCompound struct {
	f      *remoteFile
	sector int
	fat    []uint32
}

func (c *msiCompound) readSector(id uint32) ([]byte, error) {
	off := (int64(id) + 1) * int64(c.sector)
	if id >= 0xfffffffa || off > c.f.size-int64(c.sector) {
		return nil, malformed("CFB sector bounds")
	}
	b := make([]byte, c.sector)
	_, err := c.f.ReadAt(b, off)
	return b, err
}

// chain follows a regular allocation chain, including its terminating marker.
func (c *msiCompound) chain(start uint32, limit int) ([]byte, error) {
	out := []byte{}
	seen := map[uint32]bool{}
	for id := start; id != cfbEnd; {
		if seen[id] || uint64(id) >= uint64(len(c.fat)) {
			return nil, malformed("CFB allocation chain")
		}
		if len(out)+c.sector > limit {
			return nil, errInspectLimit
		}
		seen[id] = true
		b, err := c.readSector(id)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
		id = c.fat[id]
	}
	return out, nil
}

// inspectMSI extracts the SummaryInformation Template property from CFB v3/v4.
// The MSI root CLSID prevents unrelated OLE documents from becoming Windows installers.
func inspectMSI(f *remoteFile, m *ArtifactMetadata) error {
	h := make([]byte, 512)
	if _, err := f.ReadAt(h, 0); err != nil {
		return err
	}
	le := binary.LittleEndian
	if !bytes.Equal(h[:8], []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}) {
		return malformed("MSI compound header")
	}
	ver, shift := le.Uint16(h[26:]), le.Uint16(h[30:])
	if le.Uint16(h[28:]) != 0xfffe || le.Uint16(h[32:]) != 6 || le.Uint32(h[56:]) != 4096 {
		return malformed("CFB header parameters")
	}
	if !((ver == 3 && shift == 9) || (ver == 4 && shift == 12)) {
		return installerUnsupported(m, "CFB version/sector size")
	}
	count := le.Uint32(h[44:])
	if count == 0 {
		return malformed("CFB empty FAT")
	}
	if count > 109 || le.Uint32(h[72:]) != 0 {
		return installerUnsupported(m, "CFB extended DIFAT")
	}
	c := msiCompound{f: f, sector: 1 << shift}
	seen := map[uint32]bool{}
	for i := uint32(0); i < count; i++ {
		id := le.Uint32(h[76+i*4:])
		if seen[id] {
			return malformed("CFB duplicate FAT")
		}
		seen[id] = true
		b, err := c.readSector(id)
		if err != nil {
			return err
		}
		for j := 0; j < len(b); j += 4 {
			c.fat = append(c.fat, le.Uint32(b[j:]))
		}
	}
	dir, err := c.chain(le.Uint32(h[48:]), inspectHeadBytes)
	if err != nil {
		return err
	}
	if len(dir) < 128 || dir[66] != 5 {
		return malformed("CFB root directory")
	}
	clsid := []byte{0x84, 0x10, 0x0c, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46}
	if !bytes.Equal(dir[80:96], clsid) {
		return installerUnsupported(m, "compound file is not an MSI database")
	}
	var entry []byte
	for i := 128; i+128 <= len(dir); i += 128 {
		e := dir[i : i+128]
		if e[66] != 2 {
			continue
		}
		n := int(le.Uint16(e[64:]))
		if n < 2 || n > 64 || n%2 != 0 {
			return malformed("CFB directory name")
		}
		units := make([]uint16, n/2-1)
		for j := range units {
			units[j] = le.Uint16(e[j*2:])
		}
		if string(utf16.Decode(units)) == "\x05SummaryInformation" {
			if entry != nil {
				return malformed("duplicate MSI summary")
			}
			entry = e
		}
	}
	if entry == nil {
		return malformed("MSI summary missing")
	}
	size := le.Uint64(entry[120:])
	if size > inspectHeadBytes {
		return installerUnsupported(m, "MSI summary exceeds inspection limit")
	}
	var summary []byte
	if size >= 4096 {
		summary, err = c.chain(le.Uint32(entry[116:]), inspectHeadBytes)
	} else {
		miniCount := le.Uint32(h[64:])
		if miniCount == 0 {
			return malformed("CFB missing miniFAT")
		}
		if uint64(miniCount)*uint64(c.sector) > inspectHeadBytes {
			return installerUnsupported(m, "CFB miniFAT exceeds inspection limit")
		}
		mini, err2 := c.chain(le.Uint32(h[60:]), int(miniCount)*c.sector)
		if err2 != nil {
			return err2
		}
		if len(mini) != int(miniCount)*c.sector {
			return malformed("CFB miniFAT length")
		}
		rootSize := le.Uint64(dir[120:])
		if rootSize > inspectMaxInflated {
			return installerUnsupported(m, "CFB mini stream exceeds inspection limit")
		}
		root, err2 := c.chain(le.Uint32(dir[116:]), inspectMaxInflated)
		if err2 != nil {
			return err2
		}
		if rootSize > uint64(len(root)) {
			return malformed("CFB mini stream size")
		}
		root = root[:rootSize]
		visited := map[uint32]bool{}
		for id := le.Uint32(entry[116:]); id != cfbEnd; {
			if visited[id] || uint64(id)*4+4 > uint64(len(mini)) || uint64(id)*64+64 > uint64(len(root)) {
				return malformed("CFB mini chain")
			}
			if len(summary)+64 > inspectHeadBytes {
				return errInspectLimit
			}
			visited[id] = true
			summary = append(summary, root[int(id)*64:int(id)*64+64]...)
			id = le.Uint32(mini[int(id)*4:])
		}
	}
	if err != nil {
		return err
	}
	if uint64(len(summary)) < size {
		return malformed("MSI summary length")
	}
	template, err := msiTemplate(summary[:size])
	if err != nil {
		return err
	}
	platform, _, ok := strings.Cut(template, ";")
	if !ok {
		return malformed("MSI template")
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		platform = "Intel"
	}
	note(m, "", "msi.template", template)
	addTarget(m, ArtifactTarget{Platform: "windows", Arch: installerArch(platform, "msi:")})
	m.Role = "installer"
	return nil
}

// msiTemplate reads PID_TEMPLATE (7), a VT_LPSTR, from the SummaryInformation set.
func msiTemplate(b []byte) (string, error) {
	le := binary.LittleEndian
	if len(b) < 48 || le.Uint16(b) != 0xfffe {
		return "", malformed("MSI property header")
	}
	n := le.Uint32(b[24:])
	if n == 0 || n > 2 || uint64(28)+uint64(n)*20 > uint64(len(b)) {
		return "", malformed("MSI property sets")
	}
	fmtid := []byte{0xe0, 0x85, 0x9f, 0xf2, 0xf9, 0x4f, 0x68, 0x10, 0xab, 0x91, 0x08, 0, 0x2b, 0x27, 0xb3, 0xd9}
	for i := uint32(0); i < n; i++ {
		d := b[28+i*20:]
		if !bytes.Equal(d[:16], fmtid) {
			continue
		}
		off := uint64(le.Uint32(d[16:]))
		if off+8 > uint64(len(b)) {
			return "", malformed("MSI property section")
		}
		s := b[off:]
		size, count := le.Uint32(s), le.Uint32(s[4:])
		if size < 8 || uint64(size) > uint64(len(s)) || uint64(count)*8+8 > uint64(size) {
			return "", malformed("MSI property count")
		}
		s = s[:size]
		for j := uint32(0); j < count; j++ {
			p := s[8+j*8:]
			if le.Uint32(p) != 7 {
				continue
			}
			pos := uint64(le.Uint32(p[4:]))
			if pos < 8+uint64(count)*8 || pos+8 > uint64(len(s)) {
				return "", malformed("MSI template offset")
			}
			v := s[pos:]
			if le.Uint32(v) != 30 {
				return "", malformed("MSI template type")
			}
			length := uint64(le.Uint32(v[4:]))
			if length < 1 || length > 1024 || length+8 > uint64(len(v)) || v[7+length] != 0 {
				return "", malformed("MSI template string")
			}
			value := v[8 : 7+length]
			if bytes.IndexByte(value, 0) >= 0 {
				return "", malformed("MSI template null")
			}
			return string(value), nil
		}
	}
	return "", fmt.Errorf("%w: MSI template missing", ErrMalformedArtifact)
}
