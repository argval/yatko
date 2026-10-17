package github

import (
	"encoding/binary"
	"io"
)

// peHybrid follows the PE64 load-config pointer to CHPE metadata without reading code.
// ARM64EC keeps an x64 machine header; ARM64X keeps an ARM64 header. See Microsoft's
// https://learn.microsoft.com/en-us/windows/arm/arm64ec and LLVM's Object/COFF.h.
func peHybrid(r io.ReaderAt, pe int64, coff, opt []byte) string {
	le := binary.LittleEndian
	if len(opt) < 200 || le.Uint16(opt) != 0x20b || le.Uint32(opt[108:]) <= 10 {
		return ""
	}
	rva, size := le.Uint32(opt[192:]), le.Uint32(opt[196:])
	if rva == 0 || size < 208 { // CHPEMetadataPointer ends at load-config byte 208.
		return ""
	}
	count := int(le.Uint16(coff[6:]))
	if count > 96 { // The Windows loader accepts at most 96 sections.
		return "unreadable load configuration"
	}
	headers, image := le.Uint32(opt[60:]), le.Uint32(opt[56:])
	sectionOffset := pe + 24 + int64(le.Uint16(coff[20:]))
	sections := make([]byte, count*40)
	if sectionOffset+int64(len(sections)) > int64(headers) {
		return "unreadable load configuration"
	}
	if n, _ := r.ReadAt(sections, sectionOffset); n != len(sections) {
		return "unreadable load configuration"
	}
	offset, ok := peRVAOffset(rva, 208, headers, image, sections)
	var config [208]byte
	if !ok {
		return "unreadable load configuration"
	}
	if n, _ := r.ReadAt(config[:], offset); n != len(config) {
		return "unreadable load configuration"
	}
	if le.Uint32(config[:]) < 208 {
		return ""
	}
	pointer := le.Uint64(config[200:])
	if pointer == 0 {
		return ""
	}
	base := le.Uint64(opt[24:])
	if pointer < base || pointer-base > 0xffffffff {
		return "unreadable CHPE metadata"
	}
	metadataRVA := uint32(pointer - base)
	offset, ok = peRVAOffset(metadataRVA, 80, headers, image, sections)
	var metadata [92]byte
	if !ok {
		return "unreadable CHPE metadata"
	}
	if n, _ := r.ReadAt(metadata[:80], offset); n != 80 {
		return "unreadable CHPE metadata"
	}
	switch le.Uint32(metadata[:]) {
	case 1:
	case 2:
		if _, ok := peRVAOffset(metadataRVA, 92, headers, image, sections); !ok {
			return "unreadable CHPE metadata"
		}
		if n, _ := r.ReadAt(metadata[80:], offset+80); n != 12 {
			return "unreadable CHPE metadata"
		}
	default:
		return "unsupported CHPE metadata version"
	}
	// Validate each declared table's extent without allocating or reading its entries.
	for _, table := range [][3]uint32{{4, 8, 8}, {12, 48, 12}, {16, 52, 8}} {
		rva, count := le.Uint32(metadata[table[0]:]), le.Uint32(metadata[table[1]:])
		if count == 0 {
			continue
		}
		length := uint64(count) * uint64(table[2])
		if length > 0xffffffff {
			return "unreadable CHPE metadata"
		}
		if _, ok := peRVAOffset(rva, uint32(length), headers, image, sections); !ok {
			return "unreadable CHPE metadata"
		}
	}
	if le.Uint16(coff[4:]) == 0x8664 {
		return "arm64ec"
	}
	return "arm64x"
}

// peRVAOffset maps bytes in the loaded image to one file-backed section or header.
// Arithmetic uses 64 bits so crafted 32-bit RVAs and sizes cannot wrap.
func peRVAOffset(rva, size, headers, image uint32, sections []byte) (int64, bool) {
	end := uint64(rva) + uint64(size)
	if rva == 0 || size == 0 || end > uint64(image) {
		return 0, false
	}
	if rva < headers {
		return int64(rva), end <= uint64(headers)
	}
	le := binary.LittleEndian
	var offset int64
	found := false
	for len(sections) >= 40 {
		section := sections[:40]
		sections = sections[40:]
		start := uint64(le.Uint32(section[12:]))
		rawSize, raw := uint64(le.Uint32(section[16:])), uint64(le.Uint32(section[20:]))
		if uint64(rva) < start || end > start+rawSize || raw < uint64(headers) {
			continue
		}
		if found { // Overlapping sections make this mapping ambiguous.
			return 0, false
		}
		offset, found = int64(raw+uint64(rva)-start), true
	}
	return offset, found
}
