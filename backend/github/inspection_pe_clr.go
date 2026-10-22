package github

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"unicode/utf8"
)

// peCLR proves CPU-neutral IL from the COR20 header and bounded metadata headers.
// The required/preferred bit pair follows dotnet/runtime src/coreclr/inc/corhdr.h.
func peCLR(r io.ReaderAt, pe int64, coff, opt []byte, m *ArtifactMetadata, at string) bool {
	le := binary.LittleEndian
	reject := func(reason string) bool {
		note(m, at, "pe.clr.validation", reason)
		return false
	}
	dd := 96
	if le.Uint16(opt) == 0x20b {
		dd = 112
	} else if le.Uint16(opt) != 0x10b {
		return reject("unsupported CLR optional header")
	}
	if len(opt) < dd+15*8 || le.Uint32(opt[dd-4:]) < 15 {
		return reject("invalid CLR data directory")
	}
	rva, size := le.Uint32(opt[dd+14*8:]), le.Uint32(opt[dd+14*8+4:])
	count := int(le.Uint16(coff[6:]))
	headers, image := le.Uint32(opt[60:]), le.Uint32(opt[56:])
	sectionOffset := pe + 24 + int64(le.Uint16(coff[20:]))
	if size < 72 || count > 96 || sectionOffset+int64(count*40) > int64(headers) {
		return reject("invalid CLR section or directory bounds")
	}
	sections := make([]byte, count*40)
	if n, _ := r.ReadAt(sections, sectionOffset); n != len(sections) {
		return reject("unreadable CLR section headers")
	}
	offset, ok := peRVAOffset(rva, size, headers, image, sections)
	var cor [72]byte
	if !ok {
		return reject("invalid CLR directory bounds")
	}
	if n, _ := r.ReadAt(cor[:], offset); n != len(cor) {
		return reject("unreadable CLR header")
	}
	if le.Uint32(cor[:]) != 72 || le.Uint16(cor[4:]) != 2 || (le.Uint16(cor[6:]) != 0 && le.Uint16(cor[6:]) != 5) {
		return reject("unsupported CLR header version")
	}
	flags := le.Uint32(cor[16:])
	note(m, at, "pe.clr.flags", fmt.Sprintf("0x%08x", flags))
	if flags & ^uint32(0x3001f) != 0 {
		return reject("unsupported CLR runtime flags")
	}
	pair := flags & 0x20002
	if pair == 0x20000 {
		return reject("invalid CLR 32-bit preference flags")
	}
	// PE32+ and mixed/native images retain their declared machine requirement.
	if le.Uint16(coff[4:]) != 0x14c || le.Uint16(opt) != 0x10b || flags&1 == 0 || flags&0x10 != 0 || pair == 2 {
		return false
	}
	for _, start := range []int{40, 48, 56, 64} {
		if le.Uint64(cor[start:]) != 0 { // Native fixups, unmanaged exports, or ReadyToRun/NGEN.
			return false
		}
	}
	if pair == 0x20002 {
		if le.Uint16(coff[22:])&0x2000 != 0 {
			return false // A preferred-32-bit DLL cannot load in a 64-bit process.
		}
		note(m, at, "pe.clr.preference", "32-bit")
	}
	metaRVA, metaSize := le.Uint32(cor[8:]), le.Uint32(cor[12:])
	metaOffset, ok := peRVAOffset(metaRVA, metaSize, headers, image, sections)
	if !ok || metaSize < 24 {
		return reject("invalid CLR metadata bounds")
	}
	for _, start := range []int{24, 32} {
		dirRVA, dirSize := le.Uint32(cor[start:]), le.Uint32(cor[start+4:])
		if dirRVA == 0 && dirSize == 0 {
			continue
		}
		if _, ok := peRVAOffset(dirRVA, dirSize, headers, image, sections); !ok {
			return reject("invalid CLR resource or signature bounds")
		}
	}
	entry := le.Uint32(cor[20:])
	if entry != 0 && (entry>>24 != 6 || entry&0xffffff == 0) {
		return reject("unsupported CLR external or invalid managed entry point")
	}
	if entry == 0 && le.Uint16(coff[22:])&0x2000 == 0 {
		return reject("invalid CLR executable entry point")
	}
	if reason := peCLRMetadata(r, metaOffset, metaSize, entry&0xffffff); reason != "" {
		return reject(reason)
	}
	note(m, at, "pe.clr.architecture", "anycpu")
	return true
}

// peCLRMetadata validates the BSJB root, stream bounds, and entry-point row count.
// Only metadata headers are read; IL bodies and application resources are not executed.
func peCLRMetadata(r io.ReaderAt, offset int64, size, entryRow uint32) string {
	le := binary.LittleEndian
	head := make([]byte, min(size, 4096))
	if n, _ := r.ReadAt(head, offset); n != len(head) {
		return "unreadable CLR metadata"
	}
	if len(head) < 20 || string(head[:4]) != "BSJB" || le.Uint16(head[4:]) != 1 || le.Uint16(head[6:]) != 1 || le.Uint32(head[8:]) != 0 {
		return "invalid CLR metadata root"
	}
	versionSize := le.Uint32(head[12:])
	if versionSize == 0 || versionSize > 256 || versionSize%4 != 0 || 16+versionSize+4 > uint32(len(head)) {
		return "invalid CLR metadata version"
	}
	version := head[16 : 16+versionSize]
	end := bytes.IndexByte(version, 0)
	if end <= 0 || !utf8.Valid(version[:end]) || !bytes.Equal(version[end:], make([]byte, len(version)-end)) {
		return "invalid CLR metadata version"
	}
	cursor := int(16 + versionSize)
	count := int(le.Uint16(head[cursor+2:]))
	if le.Uint16(head[cursor:]) != 0 || count == 0 || count > 64 {
		return "unsupported CLR metadata stream header"
	}
	cursor += 4
	type stream struct{ start, size uint32 }
	streams := make([]stream, 0, count)
	names := map[string]bool{}
	var tables stream
	for i := 0; i < count; i++ {
		if cursor+9 > len(head) {
			return "invalid CLR metadata stream framing"
		}
		s := stream{le.Uint32(head[cursor:]), le.Uint32(head[cursor+4:])}
		cursor += 8
		nameEnd := bytes.IndexByte(head[cursor:min(cursor+32, len(head))], 0)
		if nameEnd <= 0 || !utf8.Valid(head[cursor:cursor+nameEnd]) {
			return "invalid CLR metadata stream name"
		}
		name := string(head[cursor : cursor+nameEnd])
		if names[name] || uint64(s.start)+uint64(s.size) > uint64(size) {
			return "invalid CLR metadata stream bounds"
		}
		names[name] = true
		if name == "#~" || name == "#-" {
			if tables.size != 0 {
				return "invalid CLR duplicate metadata tables"
			}
			tables = s
		}
		streams = append(streams, s)
		cursor += (nameEnd + 1 + 3) &^ 3
	}
	for i, s := range streams {
		if s.size == 0 {
			continue
		}
		if s.start < uint32(cursor) {
			return "invalid CLR overlapping metadata headers"
		}
		for _, previous := range streams[:i] {
			if previous.size > 0 && uint64(s.start) < uint64(previous.start)+uint64(previous.size) && uint64(previous.start) < uint64(s.start)+uint64(s.size) {
				return "invalid CLR overlapping metadata streams"
			}
		}
	}
	if tables.size < 24 {
		return "invalid CLR missing metadata tables"
	}
	tableHead := make([]byte, min(tables.size, 24+64*4))
	if n, _ := r.ReadAt(tableHead, offset+int64(tables.start)); n != len(tableHead) {
		return "unreadable CLR metadata tables"
	}
	valid := le.Uint64(tableHead[8:])
	rowOffset := 24
	if le.Uint32(tableHead) != 0 || valid>>45 != 0 || 24+4*bits.OnesCount64(valid) > len(tableHead) {
		return "invalid CLR metadata table header"
	}
	var rows [64]uint32
	for i := range rows {
		if valid&(uint64(1)<<i) != 0 {
			rows[i] = le.Uint32(tableHead[rowOffset:])
			rowOffset += 4
		}
	}
	minimum := uint64(rowOffset) + uint64(rows[0])*10 + uint64(rows[2])*14 + uint64(rows[6])*14 + uint64(rows[32])*22
	if rows[0] != 1 || rows[2] == 0 || rows[6] == 0 || rows[32] != 1 || entryRow > rows[6] || minimum > uint64(tables.size) {
		return "invalid CLR assembly or entry-point table bounds"
	}
	return ""
}
