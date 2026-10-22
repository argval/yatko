package github

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz/lzma"
)

// errUnsupportedCodec marks content the bounded decoders refuse: a filter or
// coder they do not implement, encryption, or a window above the memory cap.
// It wraps errInspectLimit, so callers report missing evidence, not malformed input.
var errUnsupportedCodec = fmt.Errorf("%w: unsupported codec", errInspectLimit)

// decoderDictMax caps any LZMA/LZMA2 dictionary or zstd window we allocate.
// Inspection never reads more than inspectMaxInflated output, so a larger
// dictionary could only be needed for bytes we would not read anyway.
const decoderDictMax = inspectMaxInflated

var (
	inspectXZTar   = boundedTar("xz", newXZReader)
	inspectZstdTar = boundedTar("zstd", newZstdReader)
	inspectXZ      = boundedBinary("xz", newXZReader)
	inspectZstd    = boundedBinary("zstd", newZstdReader)
)

// boundedTar opens the decoder first so a refused codec becomes a note, then
// lists the tar like tarInspector. Mid-stream refusals mark the listing partial.
func boundedTar(kind string, open func(io.Reader) (io.Reader, error)) func(*remoteFile, *ArtifactMetadata) error {
	return func(f *remoteFile, m *ArtifactMetadata) error {
		r, err := open(io.NewSectionReader(f, 0, f.size))
		if err != nil {
			if decoderRefused(f, m, kind, err) {
				return nil
			}
			return err
		}
		return tarInspector(func(io.Reader) (io.Reader, error) { return r, nil })(f, m)
	}
}

// boundedBinary sniffs the decompressed head of a single compressed file.
func boundedBinary(kind string, open func(io.Reader) (io.Reader, error)) func(*remoteFile, *ArtifactMetadata) error {
	return func(f *remoteFile, m *ArtifactMetadata) error {
		r, err := open(io.NewSectionReader(f, 0, f.size))
		var head []byte
		if err == nil {
			if closer, ok := r.(io.Closer); ok {
				defer closer.Close()
			}
			head, err = io.ReadAll(io.LimitReader(r, inspectHeadBytes))
		}
		if err != nil && len(head) == 0 {
			if decoderRefused(f, m, kind, err) {
				return nil
			}
			return err
		}
		return sniffOrNote(bytes.NewReader(head), m, kind)
	}
}

// decoderRefused notes a decoder's own limit as missing evidence. Fetch budget
// and transport failures (f.err) stay errors so InspectArtifact reports them.
func decoderRefused(f *remoteFile, m *ArtifactMetadata, kind string, err error) bool {
	if f.err != nil || !errors.Is(err, errInspectLimit) {
		return false
	}
	note(m, "", kind+".decoder", err.Error())
	return true
}

// newZstdReader decodes zstd synchronously (no goroutines) and refuses frames
// whose window exceeds decoderDictMax before allocating history for them.
func newZstdReader(r io.Reader) (io.Reader, error) {
	d, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true),
		zstd.WithDecoderMaxWindow(decoderDictMax), zstd.WithDecoderMaxMemory(decoderDictMax))
	if err != nil {
		return nil, zstdErr(err)
	}
	return zstdReader{d}, nil
}

type zstdReader struct{ d *zstd.Decoder }

func (z zstdReader) Read(p []byte) (int, error) {
	n, err := z.d.Read(p)
	return n, zstdErr(err)
}

func (z zstdReader) Close() error {
	z.d.Close()
	return nil
}

func zstdErr(err error) error {
	if errors.Is(err, zstd.ErrWindowSizeExceeded) || errors.Is(err, zstd.ErrDecoderSizeExceeded) {
		return fmt.Errorf("%w: zstd window over %d bytes", errUnsupportedCodec, decoderDictMax)
	}
	return err
}

// newXZReader decodes an .xz stream's LZMA2-only blocks. It parses the
// container itself because the xz package allocates whatever dictionary the
// header declares; here the dictionary is clamped to decoderDictMax.
func newXZReader(r io.Reader) (io.Reader, error) {
	x := &xzReader{r: &countingReader{r: bufio.NewReader(r)}}
	var h [12]byte
	if _, err := io.ReadFull(x.r, h[:]); err != nil {
		return nil, fmt.Errorf("%w: xz stream header: %v", ErrMalformedArtifact, err)
	}
	if string(h[:6]) != "\xfd7zXZ\x00" || h[6] != 0 || h[7] > 0x0f || crc32.ChecksumIEEE(h[6:8]) != binary.LittleEndian.Uint32(h[8:]) {
		return nil, fmt.Errorf("%w: bad xz stream header", ErrMalformedArtifact)
	}
	if h[7] != 0 {
		x.check = 4 << ((h[7] - 1) / 3)
	}
	if err := x.nextBlock(); err != nil {
		if err != io.EOF { // a stream with no blocks is empty, not broken
			return nil, err
		}
		x.err = err
	}
	return x, nil
}

// xzReader reads blocks of one xz stream; the index and any concatenated
// stream after it are not read.
// ponytail: single stream only; multi-stream .xz files end at the first index.
type xzReader struct {
	r        *countingReader
	check    int
	block    io.Reader
	start    int64 // offset of the current block's compressed data
	compSize int64 // -1 when the block header omits it
	clamped  bool
	err      error
}

func (x *xzReader) Read(p []byte) (int, error) {
	for x.err == nil {
		n, err := x.block.Read(p)
		switch {
		case err == io.EOF:
			x.err = x.endBlock()
		case err != nil && x.clamped:
			// A clamped dictionary cannot tell corrupt data from a match
			// distance we chose not to support.
			x.err = fmt.Errorf("%w: xz dictionary over %d bytes: %v", errUnsupportedCodec, decoderDictMax, err)
		case err != nil:
			x.err = err
		}
		if n > 0 || len(p) == 0 {
			return n, nil
		}
	}
	return 0, x.err
}

// nextBlock parses a block header and starts its LZMA2 decoder. It returns
// io.EOF at the index indicator.
func (x *xzReader) nextBlock() error {
	size, err := x.r.ReadByte()
	if err != nil {
		return fmt.Errorf("%w: xz block header: %v", ErrMalformedArtifact, err)
	}
	if size == 0 {
		return io.EOF
	}
	h := make([]byte, (int(size)+1)*4)
	h[0] = size
	if _, err := io.ReadFull(x.r, h[1:]); err != nil {
		return fmt.Errorf("%w: xz block header: %v", ErrMalformedArtifact, err)
	}
	body := h[:len(h)-4]
	if crc32.ChecksumIEEE(body) != binary.LittleEndian.Uint32(h[len(h)-4:]) || body[1]&0x3c != 0 {
		return fmt.Errorf("%w: bad xz block header", ErrMalformedArtifact)
	}
	flags, rest := body[1], body[2:]
	x.compSize, x.clamped = -1, false
	uncompressed := int64(-1)
	for _, bit := range []byte{0x40, 0x80} {
		if flags&bit == 0 {
			continue
		}
		v, n := binary.Uvarint(rest)
		if n <= 0 || v > 1<<62 {
			return fmt.Errorf("%w: bad xz block size", ErrMalformedArtifact)
		}
		rest = rest[n:]
		if bit == 0x40 {
			x.compSize = int64(v)
		} else {
			uncompressed = int64(v)
		}
	}
	// Only a lone LZMA2 filter (ID 0x21, one property byte). BCJ, delta and
	// other filter chains are refused rather than decoded wrongly.
	if flags&3 != 0 || len(rest) < 3 || rest[0] != 0x21 || rest[1] != 1 {
		return fmt.Errorf("%w: xz filter chain", errUnsupportedCodec)
	}
	dict, err := lzma.DecodeDictCap(rest[2])
	if err != nil {
		return fmt.Errorf("%w: xz: %v", ErrMalformedArtifact, err)
	}
	limit := clampDict(int64(dict), uncompressed)
	x.clamped = int64(dict) > limit && (uncompressed < 0 || uncompressed > limit)
	x.start = x.r.n
	x.block, err = lzma.Reader2Config{DictCap: int(limit)}.NewReader2(x.r)
	return err
}

// endBlock skips block padding and the integrity check, then opens the next block.
func (x *xzReader) endBlock() error {
	n := x.r.n - x.start
	if x.compSize >= 0 && n != x.compSize {
		return fmt.Errorf("%w: xz block size mismatch", ErrMalformedArtifact)
	}
	tail := make([]byte, (4-n%4)%4+int64(x.check))
	if _, err := io.ReadFull(x.r, tail); err != nil {
		return fmt.Errorf("%w: xz block: %v", ErrMalformedArtifact, err)
	}
	if pad := tail[:(4-n%4)%4]; !bytes.Equal(pad, make([]byte, len(pad))) {
		return fmt.Errorf("%w: xz block padding", ErrMalformedArtifact)
	}
	return x.nextBlock()
}

// clampDict bounds a declared dictionary by decoderDictMax and, when known,
// the output size (no match can reach further back than the output start).
func clampDict(dict, size int64) int64 {
	limit := min(dict, decoderDictMax)
	if size >= 0 {
		limit = min(limit, size)
	}
	return max(limit, lzma.MinDictCap)
}

// countingReader tracks the compressed offset; the LZMA2 decoder reads chunk
// payloads through it byte by byte, so the count is exact.
type countingReader struct {
	r *bufio.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (c *countingReader) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.n++
	}
	return b, err
}

// 7z bounds. Header sizes and counts come from untrusted bytes, so every
// allocation is checked against these before it happens.
const (
	sevenZipMaxHeader  = 1 << 20 // raw or decoded header bytes
	sevenZipMaxEntries = 1 << 16 // files, folders, streams, substreams
)

// inspect7z lists a 7z archive and sniffs up to inspectMaxBinaries entries.
// Supported: plain or LZMA/LZMA2-encoded headers, and folders with a single
// Copy, LZMA or LZMA2 coder (solid blocks included, up to the inflate cap).
// Encrypted headers and folders, BCJ/BCJ2/delta filters and other coders are
// noted and skipped: they add no evidence rather than guessed evidence.
func inspect7z(f *remoteFile, m *ArtifactMetadata) error {
	// Prefetch the tail: the next header and an encoded header's packed
	// stream are usually there.
	n := min(f.size, archiveTailLimit)
	if _, err := f.fetch(f.size-n, n, n); err != nil {
		return err
	}
	sig := make([]byte, 32)
	if _, err := f.ReadAt(sig, 0); err != nil {
		return fmt.Errorf("%w: 7z signature: %v", ErrMalformedArtifact, err)
	}
	le := binary.LittleEndian
	if string(sig[:6]) != "7z\xbc\xaf\x27\x1c" || sig[6] != 0 || crc32.ChecksumIEEE(sig[12:32]) != le.Uint32(sig[8:]) {
		return fmt.Errorf("%w: bad 7z signature header", ErrMalformedArtifact)
	}
	off, size := le.Uint64(sig[12:]), le.Uint64(sig[20:])
	if size == 0 {
		archiveStructure(m, "7z", nil)
		return nil
	}
	if off > uint64(f.size) || size > uint64(f.size) || 32+off+size > uint64(f.size) {
		return fmt.Errorf("%w: 7z header outside file", ErrMalformedArtifact)
	}
	if size > sevenZipMaxHeader {
		note(m, "", "7z.header", "exceeds inspection limits")
		return nil
	}
	raw := make([]byte, size)
	if _, err := f.ReadAt(raw, 32+int64(off)); err != nil {
		return err
	}
	if crc32.ChecksumIEEE(raw) != le.Uint32(sig[28:]) {
		return fmt.Errorf("%w: 7z header CRC", ErrMalformedArtifact)
	}
	var a sevenZip
	for range 4 { // encoded headers may nest; real archives use one level
		h := &szReader{b: raw}
		switch h.byte() {
		case 0x01:
			h.header(&a)
			if h.err != nil {
				return h.err
			}
			return a.inspect(f, m)
		case 0x17:
			var s szStreams
			h.streams(&s)
			if h.err != nil {
				return h.err
			}
			var err error
			if raw, err = s.decodeHeader(f); err != nil {
				if errors.Is(err, errInspectLimit) && f.err == nil {
					note(m, "", "7z.header", err.Error())
					return nil
				}
				return err
			}
		default:
			return fmt.Errorf("%w: 7z header type", ErrMalformedArtifact)
		}
	}
	return fmt.Errorf("%w: 7z header nesting", ErrMalformedArtifact)
}

type sevenZip struct {
	streams szStreams
	files   []szFile
}

type szFile struct {
	name   string
	mode   fs.FileMode
	stream bool // has data in a folder
	folder int
	off    int64 // within the folder's output
	size   int64
}

type szCoder struct {
	id    string
	props []byte
}

type szFolder struct {
	coders []szCoder
	packed int     // packed streams consumed
	size   int64   // final output size
	crc    *uint32 // of the final output, if recorded
	subs   []int64 // substream sizes
}

type szStreams struct {
	packPos   uint64
	packSizes []uint64
	folders   []szFolder
}

// szReader parses 7z header bytes with a sticky error; every read past the
// end or count beyond the remaining bytes sets err and yields zeros.
type szReader struct {
	b   []byte
	err error
}

func (h *szReader) fail(what string) {
	if h.err == nil {
		h.err = fmt.Errorf("%w: 7z %s", ErrMalformedArtifact, what)
	}
	h.b = nil
}

func (h *szReader) byte() byte {
	if len(h.b) == 0 {
		h.fail("header truncated")
		return 0
	}
	b := h.b[0]
	h.b = h.b[1:]
	return b
}

func (h *szReader) bytes(n uint64) []byte {
	if n > uint64(len(h.b)) {
		h.fail("header truncated")
		return nil
	}
	b := h.b[:n]
	h.b = h.b[n:]
	return b
}

func (h *szReader) u32() uint32 {
	if b := h.bytes(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

// number reads 7z's variable-length UINT64: leading 1 bits in the first byte
// count the extra little-endian bytes.
func (h *szReader) number() uint64 {
	first := h.byte()
	var v uint64
	mask := byte(0x80)
	for i := range 8 {
		if first&mask == 0 {
			return v | uint64(first&(mask-1))<<(8*i)
		}
		v |= uint64(h.byte()) << (8 * i)
		mask >>= 1
	}
	return v
}

// count reads a number used to size a slice. Each counted item takes at least
// one header byte or bit, so the remaining length bounds it.
func (h *szReader) count() int {
	n := h.number()
	if n > sevenZipMaxEntries || n > uint64(len(h.b))*8+1 {
		h.fail("count out of range")
		return 0
	}
	return int(n)
}

func (h *szReader) expect(id byte) {
	if h.byte() != id {
		h.fail(fmt.Sprintf("expected property %#x", id))
	}
}

// bits reads an MSB-first bit vector of n entries.
func (h *szReader) bits(n int) []bool {
	b := h.bytes(uint64(n+7) / 8)
	if b == nil {
		return make([]bool, n)
	}
	v := make([]bool, n)
	for i := range v {
		v[i] = b[i/8]&(0x80>>(i%8)) != 0
	}
	return v
}

// defined reads "all defined" or a bit vector, as used by digests and attributes.
func (h *szReader) defined(n int) []bool {
	if h.byte() != 0 {
		v := make([]bool, n)
		for i := range v {
			v[i] = true
		}
		return v
	}
	return h.bits(n)
}

func (h *szReader) digests(n int) []*uint32 {
	d := make([]*uint32, n)
	for i, ok := range h.defined(n) {
		if ok {
			v := h.u32()
			d[i] = &v
		}
	}
	return d
}

func (h *szReader) header(a *sevenZip) {
	id := h.byte()
	if id == 0x02 { // archive properties
		for h.err == nil && h.byte() != 0 {
			h.bytes(h.number())
		}
		id = h.byte()
	}
	if id == 0x03 { // additional streams: parsed only to skip them
		var s szStreams
		h.streams(&s)
		id = h.byte()
	}
	if id == 0x04 {
		h.streams(&a.streams)
		id = h.byte()
	}
	if id == 0x05 {
		h.files(a)
		id = h.byte()
	}
	if id != 0 {
		h.fail("header property")
	}
}

func (h *szReader) streams(s *szStreams) {
	id := h.byte()
	if id == 0x06 {
		s.packPos = h.number()
		s.packSizes = make([]uint64, h.count())
		for id = h.byte(); id != 0 && h.err == nil; id = h.byte() {
			switch id {
			case 0x09:
				for i := range s.packSizes {
					s.packSizes[i] = h.number()
				}
			case 0x0a:
				h.digests(len(s.packSizes))
			default:
				h.fail("pack info property")
			}
		}
		id = h.byte()
	}
	if id == 0x07 {
		h.folders(s)
		id = h.byte()
	}
	if id == 0x08 {
		h.substreams(s)
		id = h.byte()
	}
	if id != 0 {
		h.fail("streams property")
	}
	packed := 0
	for i := range s.folders {
		if s.folders[i].subs == nil {
			s.folders[i].subs = []int64{s.folders[i].size}
		}
		packed += s.folders[i].packed
	}
	if packed > len(s.packSizes) {
		h.fail("folders use missing pack streams")
	}
}

func (h *szReader) folders(s *szStreams) {
	h.expect(0x0b)
	s.folders = make([]szFolder, h.count())
	if h.byte() != 0 {
		h.fail("external folders")
	}
	outs := make([][]int64, len(s.folders))
	bound := make([][]bool, len(s.folders))
	for i := range s.folders {
		if h.err != nil {
			return
		}
		f := &s.folders[i]
		f.coders = make([]szCoder, h.count())
		ins, nOut := 0, 0
		for j := range f.coders {
			flags := h.byte()
			if flags&0xc0 != 0 {
				h.fail("coder flags")
			}
			f.coders[j].id = string(h.bytes(uint64(flags & 0x0f)))
			in, out := 1, 1
			if flags&0x10 != 0 {
				in, out = h.count(), h.count()
			}
			if flags&0x20 != 0 {
				f.coders[j].props = h.bytes(h.number())
			}
			ins, nOut = ins+in, nOut+out
		}
		if nOut == 0 || nOut > 64 || ins > 64 {
			h.fail("coder streams")
			return
		}
		bound[i] = make([]bool, nOut)
		for range nOut - 1 { // bind pairs: in index, out index
			h.number()
			out := h.number()
			if out >= uint64(nOut) {
				h.fail("bind pair")
				return
			}
			bound[i][out] = true
		}
		f.packed = ins - (nOut - 1)
		if f.packed < 1 {
			h.fail("packed streams")
			return
		}
		if f.packed > 1 {
			for range f.packed {
				h.number()
			}
		}
		outs[i] = make([]int64, nOut)
	}
	h.expect(0x0c)
	for i := range s.folders {
		for j := range outs[i] {
			v := h.number()
			if v > 1<<62 {
				h.fail("unpack size")
			}
			outs[i][j] = int64(v)
		}
		// The folder's output is the one coder output not bound to another coder.
		if k := slices.Index(bound[i], false); k >= 0 {
			s.folders[i].size = outs[i][k]
		}
	}
	for id := h.byte(); id != 0 && h.err == nil; id = h.byte() {
		if id != 0x0a {
			h.fail("unpack info property")
			return
		}
		for i, d := range h.digests(len(s.folders)) {
			s.folders[i].crc = d
		}
	}
}

func (h *szReader) substreams(s *szStreams) {
	counts := make([]int, len(s.folders))
	for i := range counts {
		counts[i] = 1
	}
	id := h.byte()
	if id == 0x0d {
		total := 0
		for i := range counts {
			counts[i] = h.count()
			total += counts[i]
		}
		if total > sevenZipMaxEntries {
			h.fail("too many substreams")
			return
		}
		id = h.byte()
	}
	for i := range s.folders {
		f := &s.folders[i]
		f.subs = make([]int64, 0, counts[i])
		if counts[i] == 0 {
			continue
		}
		var sum int64
		for range counts[i] - 1 {
			if id != 0x09 {
				h.fail("substream sizes missing")
				return
			}
			v := h.number()
			if v > uint64(f.size-sum) {
				h.fail("substream sizes exceed folder")
				return
			}
			f.subs = append(f.subs, int64(v))
			sum += int64(v)
		}
		f.subs = append(f.subs, f.size-sum)
	}
	if id == 0x09 {
		id = h.byte()
	}
	for ; id != 0 && h.err == nil; id = h.byte() {
		if id != 0x0a {
			h.fail("substreams property")
			return
		}
		unknown := 0
		for i, f := range s.folders {
			if counts[i] != 1 || f.crc == nil {
				unknown += counts[i]
			}
		}
		h.digests(unknown)
	}
}

// files reads names, empty-stream flags and attributes, then assigns each
// non-empty file to the next substream in folder order.
func (h *szReader) files(a *sevenZip) {
	files := make([]szFile, h.count())
	var empty, emptyFile []bool
	for h.err == nil {
		kind := h.byte()
		if kind == 0 {
			break
		}
		p := &szReader{b: h.bytes(h.number())}
		switch kind {
		case 0x0e:
			empty = p.bits(len(files))
		case 0x0f:
			emptyFile = p.bits(countTrue(empty))
		case 0x11:
			if p.byte() != 0 || len(p.b)%2 != 0 {
				h.fail("file names")
				break
			}
			u := make([]uint16, len(p.b)/2)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(p.b[2*i:])
			}
			for i := range files {
				end := slices.Index(u, 0)
				if end < 0 {
					h.fail("file names")
					break
				}
				files[i].name = strings.TrimPrefix(strings.ReplaceAll(string(utf16.Decode(u[:end])), `\`, "/"), "./")
				u = u[end+1:]
			}
		case 0x15:
			def := p.defined(len(files))
			if p.byte() != 0 {
				h.fail("external attributes")
				break
			}
			for i, ok := range def {
				if ok {
					files[i].mode = szMode(p.u32())
				}
			}
		}
		if p.err != nil {
			h.err = p.err
		}
	}
	fi, sub, e := 0, 0, 0
	var off int64
	for i := range files {
		f := &files[i]
		if i < len(empty) && empty[i] {
			if e >= len(emptyFile) || !emptyFile[e] {
				f.mode |= fs.ModeDir
			}
			e++
			continue
		}
		for fi < len(a.streams.folders) && sub >= len(a.streams.folders[fi].subs) {
			fi, sub, off = fi+1, 0, 0
		}
		if fi == len(a.streams.folders) {
			h.fail("more files than streams")
			return
		}
		f.stream, f.folder, f.off, f.size = true, fi, off, a.streams.folders[fi].subs[sub]
		off += f.size
		sub++
	}
	a.files = files
}

func countTrue(v []bool) int {
	n := 0
	for _, b := range v {
		if b {
			n++
		}
	}
	return n
}

// szMode maps Windows attributes, with the p7zip/7-Zip Unix extension (0x8000)
// carrying st_mode in the high 16 bits, to an fs.FileMode.
func szMode(attr uint32) fs.FileMode {
	if attr&0x10 != 0 {
		return fs.ModeDir
	}
	if attr&0x8000 == 0 {
		return 0o644
	}
	unix := attr >> 16
	mode := fs.FileMode(unix & 0o777)
	if t := unix & 0o170000; t != 0 && t != 0o100000 {
		mode |= fs.ModeIrregular // symlinks, devices: never sniffed
	}
	return mode
}

// decodeHeader decodes folder 0 of an encoded header's streams, verifying its CRC.
func (s *szStreams) decodeHeader(f *remoteFile) ([]byte, error) {
	if len(s.folders) == 0 {
		return nil, fmt.Errorf("%w: 7z encoded header without folder", ErrMalformedArtifact)
	}
	fo := s.folders[0]
	if fo.size > sevenZipMaxHeader {
		return nil, fmt.Errorf("%w: 7z header over %d bytes", errInspectLimit, sevenZipMaxHeader)
	}
	r, err := s.open(f, 0)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(r, fo.size))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) != fo.size || (fo.crc != nil && crc32.ChecksumIEEE(raw) != *fo.crc) {
		return nil, fmt.Errorf("%w: 7z encoded header", ErrMalformedArtifact)
	}
	return raw, nil
}

// open returns a decoder for folder i's output. LZMA/LZMA2 dictionaries are
// clamped like xz; errors past a clamp report the limit, not corruption.
func (s *szStreams) open(f *remoteFile, i int) (io.Reader, error) {
	fo := s.folders[i]
	if slices.ContainsFunc(fo.coders, func(c szCoder) bool { return c.id == "\x06\xf1\x07\x01" }) {
		return nil, fmt.Errorf("%w: 7z encrypted", errUnsupportedCodec)
	}
	if len(fo.coders) != 1 || fo.packed != 1 {
		return nil, fmt.Errorf("%w: 7z coder chain", errUnsupportedCodec)
	}
	idx := 0
	for _, prev := range s.folders[:i] {
		idx += prev.packed
	}
	size := uint64(f.size)
	pos := 32 + min(s.packPos, size)
	for _, n := range s.packSizes[:idx] {
		pos += min(n, size) // each term stays below the asset size, so no overflow
	}
	n := s.packSizes[idx]
	if s.packPos > size || pos > size || n > size-pos {
		return nil, fmt.Errorf("%w: 7z pack stream outside file", ErrMalformedArtifact)
	}
	packed := io.NewSectionReader(f, int64(pos), int64(n))
	c := fo.coders[0]
	switch {
	case c.id == "\x00":
		return packed, nil
	case c.id == "\x03\x01\x01" && len(c.props) == 5:
		dict := int64(binary.LittleEndian.Uint32(c.props[1:]))
		limit := clampDict(dict, fo.size)
		// Raw 7z LZMA has no .lzma header; supply one with the clamped size.
		hdr := make([]byte, lzma.HeaderLen)
		hdr[0] = c.props[0]
		binary.LittleEndian.PutUint32(hdr[1:], uint32(limit))
		binary.LittleEndian.PutUint64(hdr[5:], uint64(fo.size))
		r, err := lzma.ReaderConfig{DictCap: int(limit)}.NewReader(io.MultiReader(bytes.NewReader(hdr), packed))
		if err != nil {
			return nil, fmt.Errorf("%w: 7z lzma: %v", ErrMalformedArtifact, err)
		}
		return clampErrors(r, dict > limit && fo.size > limit), nil
	case c.id == "\x21" && len(c.props) == 1:
		dict, err := lzma.DecodeDictCap(c.props[0])
		if err != nil {
			return nil, fmt.Errorf("%w: 7z lzma2: %v", ErrMalformedArtifact, err)
		}
		limit := clampDict(int64(dict), fo.size)
		r, err := lzma.Reader2Config{DictCap: int(limit)}.NewReader2(packed)
		if err != nil {
			return nil, fmt.Errorf("%w: 7z lzma2: %v", ErrMalformedArtifact, err)
		}
		return clampErrors(r, int64(dict) > limit && fo.size > limit), nil
	}
	return nil, fmt.Errorf("%w: 7z coder %x", errUnsupportedCodec, c.id)
}

// clampErrors reports decode errors as a limit when the dictionary was clamped.
func clampErrors(r io.Reader, clamped bool) io.Reader {
	if !clamped {
		return r
	}
	return clampedReader{r}
}

type clampedReader struct{ r io.Reader }

func (c clampedReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if err != nil && err != io.EOF {
		err = fmt.Errorf("%w: dictionary over %d bytes: %v", errUnsupportedCodec, decoderDictMax, err)
	}
	return n, err
}

// inspect records the listing, then reads the heads of the best candidates in
// folder order, sharing one inflate budget across folders.
func (a *sevenZip) inspect(f *remoteFile, m *ArtifactMetadata) error {
	var names []string
	var candidates []*szFile
	for i := range a.files {
		file := &a.files[i]
		if file.mode.IsDir() || file.name == "" {
			continue
		}
		names = append(names, file.name)
		if file.stream && binaryCandidate(file.name, file.mode, file.size) {
			candidates = append(candidates, file)
		}
	}
	archiveStructure(m, "7z", names)
	slices.SortStableFunc(candidates, func(x, y *szFile) int { return bundleRank(x.name) - bundleRank(y.name) })
	m.TargetsPartial = len(candidates) > inspectMaxBinaries
	candidates = candidates[:min(len(candidates), inspectMaxBinaries)]
	slices.SortStableFunc(candidates, func(x, y *szFile) int {
		if x.folder != y.folder {
			return x.folder - y.folder
		}
		return cmp.Compare(x.off, y.off)
	})
	bundle := macAppBundle(names)
	budget := &capReader{n: inspectMaxInflated}
	folder, pos := -1, int64(0)
	for _, c := range candidates {
		if c.folder != folder {
			r, err := a.streams.open(f, c.folder)
			if err != nil {
				if errors.Is(err, errInspectLimit) && f.err == nil {
					m.TargetsPartial = true
					note(m, "7z:"+c.name, "decoder", err.Error())
					continue
				}
				return err
			}
			budget.r, folder, pos = r, c.folder, 0
		}
		want := min(c.size, inspectHeadBytes)
		_, err := io.CopyN(io.Discard, budget, c.off-pos)
		var head []byte
		if err == nil {
			head, err = io.ReadAll(io.LimitReader(budget, want))
		}
		pos = c.off + int64(len(head))
		switch {
		case f.err != nil:
			m.TargetsPartial = true
			return nil // budget or transport; InspectArtifact reports transport errors
		case errors.Is(err, errInspectLimit):
			m.TargetsPartial = true
			note(m, "7z:"+c.name, "decoder", err.Error())
			folder = -1 // the decoder is unusable after an error
			continue
		case err == io.EOF || (err == nil && int64(len(head)) < want):
			return fmt.Errorf("%w: 7z folder shorter than its entries", ErrMalformedArtifact)
		case err != nil:
			return fmt.Errorf("%w: 7z: %v", ErrMalformedArtifact, err)
		}
		if err := inspectArchiveEntry(head, m, "7z:"+c.name, c.name, bundle); err != nil {
			return err
		}
	}
	return nil
}
