package github

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/ulikunitz/xz/lzma"
)

// inspectEXE separates the Windows launcher from an NSIS installer's payload.
// Layout: https://github.com/kichik/nsis/blob/master/Source/exehead/fileform.h
func inspectEXE(f *remoteFile, m *ArtifactMetadata) error {
	found, err := sniffBinary(f, m, "")
	if err != nil || !found {
		return err
	}
	head := make([]byte, min(f.size, inspectSpan))
	if _, err := f.ReadAt(head, 0); err != nil {
		return err
	}
	le := binary.LittleEndian
	offsets := make([]int64, 0, len(head)/512+1)
	for off := 512; off+28 <= len(head); off += 512 {
		offsets = append(offsets, int64(off))
	}
	if overlay := peOverlay(head, f.size); overlay >= int64(len(head)) && overlay+28 <= f.size {
		offsets = append(offsets, overlay)
	}
	for _, off := range offsets {
		h := make([]byte, 28)
		if _, err := f.ReadAt(h, off); err != nil {
			return err
		}
		if !bytes.Equal(h[4:20], []byte("\xef\xbe\xad\xdeNullsoftInst")) || le.Uint32(h)&^uint32(15) != 0 {
			continue
		}
		size, total := int64(le.Uint32(h[20:])), int64(le.Uint32(h[24:]))
		if size < 68 || total < 28 || total > f.size-off {
			return malformed("NSIS first header")
		}
		m.Targets = []ArtifactTarget{{Platform: "windows"}}
		m.Role = "installer"
		note(m, "", "nsis", "launcher CPU is not payload CPU")
		if size > inspectMaxInflated || le.Uint32(h)&1 != 0 {
			return nil
		}
		err := inspectNSIS(f, m, off+28, total-28, int(size))
		if err != nil && f.err == nil && !errors.Is(err, ErrMalformedArtifact) {
			// Custom NSIS codecs can share the same signature and block framing.
			note(m, "", "nsis.decoder", "payload unavailable: "+err.Error())
			return nil
		}
		return err
	}
	return nil
}

// peOverlay locates appended installer data after the PE image's raw sections.
// Invalid or incomplete section tables supply no additional probe location.
func peOverlay(head []byte, size int64) int64 {
	if len(head) < 64 || string(head[:2]) != "MZ" {
		return 0
	}
	le := binary.LittleEndian
	pe := uint64(le.Uint32(head[0x3c:]))
	if pe+24 > uint64(len(head)) || string(head[pe:pe+4]) != "PE\x00\x00" {
		return 0
	}
	count := uint64(le.Uint16(head[pe+6:]))
	start := pe + 24 + uint64(le.Uint16(head[pe+20:]))
	if count == 0 || count > 96 || start+count*40 > uint64(len(head)) {
		return 0
	}
	var end uint64
	for i := uint64(0); i < count; i++ {
		s := head[start+i*40:]
		end = max(end, uint64(le.Uint32(s[20:]))+uint64(le.Uint32(s[16:])))
	}
	end = (end + 511) &^ uint64(511)
	if end > uint64(size) {
		return 0
	}
	return int64(end)
}

// inspectNSIS reads the bounded header and, for one unambiguous app EXE,
// its PE header. It does not scan arbitrary compressed bytes for CPU markers.
func inspectNSIS(f *remoteFile, m *ArtifactMetadata, off, size int64, headerSize int) error {
	input := bufio.NewReader(io.NewSectionReader(f, off, size))
	prefix, err := input.Peek(5)
	if err != nil {
		return err
	}
	length := binary.LittleEndian.Uint32(prefix[:4])
	dict := binary.LittleEndian.Uint32(prefix[1:])
	// A stored block length can also end in 0x5d, the LZMA property byte.
	solid := prefix[0] == 0x5d && length != uint32(headerSize) && dict >= 4096 &&
		(dict <= decoderDictMax || dict&(dict-1) == 0)
	if !solid && ((length&0x80000000 == 0 && length != uint32(headerSize)) || int64(length&0x7fffffff) > size-4) {
		note(m, "", "nsis.decoder", "unsupported solid codec")
		return nil
	}
	var r io.Reader
	var dataOffset int64
	if solid {
		r, err = nsisLZMA(input)
		if err != nil {
			note(m, "", "nsis.decoder", err.Error())
			return nil
		}
		r = io.LimitReader(r, inspectMaxInflated)
		var length [4]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return err
		}
		if int(binary.LittleEndian.Uint32(length[:])) != headerSize {
			return malformed("NSIS solid header size")
		}
	} else {
		r, dataOffset, err = nsisBlock(f, off, size)
		if err != nil {
			return err
		}
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return err
	}
	files, err := nsisFiles(header)
	if err != nil {
		return err
	}
	var executables []nsisFile
	for _, file := range files {
		base := strings.ToLower(file.name[strings.LastIndex(file.name, "\\")+1:])
		arch := ""
		switch base {
		case "app-64.7z", "app-64.zip":
			arch = "amd64"
		case "app-arm64.7z", "app-arm64.zip":
			arch = "arm64"
		case "app-32.7z", "app-32.zip":
			arch = "386"
		}
		if arch != "" {
			if m.Targets[0].Arch == "" {
				m.Targets = nil
			}
			addTarget(m, ArtifactTarget{Platform: "windows", Arch: arch})
			note(m, file.name, "nsis.payload", arch)
		}
		if strings.HasSuffix(base, ".exe") && !strings.HasPrefix(base, "uninstall") {
			executables = append(executables, file)
		}
	}
	if m.Targets[0].Arch != "" || len(executables) != 1 {
		return nil
	}
	file := executables[0]
	if strings.ContainsAny(file.name, "\\/:\x01\x02\x03\x04") {
		return nil // A variable or subdirectory can name a plugin or helper tool.
	}
	if solid {
		// Offsets in a solid installer are relative to the decoded data block.
		if int64(headerSize)+4+file.offset+4+inspectHeadBytes > inspectMaxInflated {
			return nil
		}
		if _, err := io.CopyN(io.Discard, r, file.offset); err != nil {
			return err
		}
		var length [4]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return err
		}
		r = io.LimitReader(r, int64(binary.LittleEndian.Uint32(length[:])))
	} else {
		if file.offset >= size-dataOffset {
			return malformed("NSIS file offset")
		}
		r, _, err = nsisBlock(f, off+dataOffset+file.offset, size-dataOffset-file.offset)
		if err != nil {
			return err
		}
		if closer, ok := r.(io.Closer); ok {
			defer closer.Close()
		}
	}
	content, err := io.ReadAll(io.LimitReader(r, inspectHeadBytes))
	if err != nil && len(content) < 512 {
		return err
	}
	var payload ArtifactMetadata
	if len(content) >= 64 && string(content[:2]) == "MZ" {
		found, err := readPE(bytes.NewReader(content), &payload, file.name)
		if err == nil && found && payload.Role == "application" && len(payload.Targets) == 1 && payload.Targets[0].Platform == "windows" {
			m.Targets = payload.Targets
			m.Evidence = append(m.Evidence, payload.Evidence...)
			note(m, file.name, "nsis.payload", "PE executable")
		}
	}
	return nil
}

type nsisFile struct {
	name   string
	offset int64
}

// nsisFiles supports the standard 32-bit NSIS block and instruction layout.
// Unknown layouts remain unproven instead of using incidental strings as facts.
func nsisFiles(h []byte) ([]nsisFile, error) {
	if len(h) < 68 {
		return nil, malformed("NSIS header size")
	}
	le := binary.LittleEndian
	entries, count := uint64(le.Uint32(h[20:])), uint64(le.Uint32(h[24:]))
	start, end := uint64(le.Uint32(h[28:])), uint64(le.Uint32(h[36:]))
	if entries < 68 || entries+count*28 > start || start+2 > end || end > uint64(len(h)) {
		return nil, malformed("NSIS block offsets")
	}
	stringsBlock := h[start:end]
	unicode := stringsBlock[0] == 0 && stringsBlock[1] == 0
	var files []nsisFile
	fileCount := 0
	for i := uint64(0); i < count; i++ {
		e := h[entries+i*28 : entries+(i+1)*28]
		if le.Uint32(e) != 20 { // EW_EXTRACTFILE, standard NSIS 2/3 opcode table
			continue
		}
		fileCount++
		if fileCount > 4096 {
			return nil, errInspectLimit
		}
		offset := uint64(le.Uint32(e[8:]))
		if unicode {
			offset *= 2
		}
		if offset >= uint64(len(stringsBlock)) {
			return nil, malformed("NSIS filename offset")
		}
		b := stringsBlock[offset:]
		name := ""
		if unicode {
			var units []uint16
			for j := 0; j+1 < len(b) && j < 2048; j += 2 {
				u := le.Uint16(b[j:])
				if u == 0 {
					name = string(utf16.Decode(units))
					break
				}
				units = append(units, u)
			}
		} else if n := bytes.IndexByte(b[:min(len(b), 1024)], 0); n >= 0 {
			name = string(b[:n])
		}
		if name != "" {
			files = append(files, nsisFile{name, int64(le.Uint32(e[12:]))})
		}
	}
	return files, nil
}

// nsisBlock opens one length-prefixed non-solid block, using NSIS's high-bit
// compression flag. LZMA and raw deflate cover the sampled standard installers.
func nsisBlock(f *remoteFile, off, size int64) (io.Reader, int64, error) {
	var length [4]byte
	if _, err := f.ReadAt(length[:], off); err != nil {
		return nil, 0, err
	}
	n := int64(binary.LittleEndian.Uint32(length[:]) & 0x7fffffff)
	if n == 0 || n > size-4 {
		return nil, 0, malformed("NSIS block size")
	}
	r := bufio.NewReader(io.NewSectionReader(f, off+4, n))
	if length[3]&0x80 == 0 {
		return r, n + 4, nil
	}
	b, err := r.Peek(1)
	if err != nil {
		return nil, 0, err
	}
	if b[0] == 0x5d {
		decoded, err := nsisLZMA(r)
		return decoded, n + 4, err
	}
	return flate.NewReader(r), n + 4, nil
}

// nsisLZMA adds the omitted LZMA output-size field and caps dictionary memory.
func nsisLZMA(r io.Reader) (io.Reader, error) {
	var h [13]byte
	if _, err := io.ReadFull(r, h[:5]); err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint32(h[1:]) > decoderDictMax {
		return nil, errUnsupportedCodec
	}
	for i := 5; i < len(h); i++ {
		h[i] = 0xff
	}
	// This decoder fills its entire dictionary before returning any bytes.
	// Clamp its read buffer; unsupported long-distance matches stay unproven.
	const readBuffer = 1 << 20
	binary.LittleEndian.PutUint32(h[1:], min(binary.LittleEndian.Uint32(h[1:]), readBuffer))
	return (lzma.ReaderConfig{DictCap: readBuffer}).NewReader(io.MultiReader(bytes.NewReader(h[:]), r))
}
