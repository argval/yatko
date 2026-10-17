package github

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// inspectFlatpakFile reads the typed metadata dictionary of an OSTree static delta.
// Framing follows GLib's GVariant serialization; compressed delta payloads are skipped.
// See flatpak_bundle_load in upstream common/flatpak-repo-utils.c.
func inspectFlatpakFile(r io.ReaderAt, size int64, m *ArtifactMetadata) error {
	if size <= 0 {
		return malformed("Flatpak superblock")
	}
	width := flatpakOffsetWidth(uint64(size))
	footer, err := flatpakRead(r, size, size-int64(6*width), 6*width)
	if err != nil {
		return err
	}
	var ends [6]uint64
	for i := range ends {
		ends[i] = flatpakOffset(footer[(5-i)*width : (6-i)*width])
	}
	dataEnd := uint64(size) - uint64(len(footer))
	for _, end := range ends {
		if end > dataEnd {
			return malformed("Flatpak superblock framing")
		}
	}
	// The tuple is (metadata, timestamp, from, to, commit, dependencies, parts, fallbacks).
	fromStart := flatpakAlign(ends[0], 8) + 8
	if ends[0] == 0 || ends[0] > dataEnd || fromStart > ends[1] ||
		(ends[1]-fromStart != 0 && ends[1]-fromStart != 32) ||
		ends[2] < ends[1] || ends[2]-ends[1] != 32 ||
		ends[3] <= flatpakAlign(ends[2], 8) || ends[4] < ends[3] ||
		(ends[4]-ends[3])%64 != 0 || ends[5] < flatpakAlign(ends[4], 8) ||
		flatpakAlign(ends[5], 8) > dataEnd {
		return malformed("Flatpak superblock framing")
	}
	fields, err := flatpakDictionary(r, size, ends[0])
	if err != nil {
		return err
	}
	magic, ok := fields["flatpak"]
	if !ok || len(magic) != 6 || string(magic[4:]) != "\x00u" ||
		(binary.LittleEndian.Uint32(magic) != 0xe5890001 && binary.BigEndian.Uint32(magic) != 0xe5890001) {
		return malformed("Flatpak magic metadata")
	}
	if endian, ok := fields["ostree.endianness"]; ok {
		if len(endian) != 3 || string(endian[1:]) != "\x00y" || (endian[0] != 'l' && endian[0] != 'B') ||
			(endian[0] == 'l') != (binary.LittleEndian.Uint32(magic) == 0xe5890001) {
			return malformed("Flatpak endianness metadata")
		}
	}
	ref, err := flatpakString(fields["ref"])
	if err != nil {
		return err
	}
	kind, name, arch, ok := flatpakReference(ref)
	if !ok {
		return malformed("Flatpak ref")
	}
	note(m, "", "flatpak.ref", ref)
	note(m, "", "flatpak.format", "OSTree static delta")
	conflicts := len(m.Conflicts)
	if raw, ok := fields["metadata"]; ok {
		text, err := flatpakString(raw)
		if err != nil {
			return err
		}
		if err := flatpakAppMetadata(text, kind, name, arch, m); err != nil {
			return err
		}
	}
	target := ArtifactTarget{Platform: "linux", Arch: flatpakArch(arch)}
	if len(m.Conflicts) != conflicts {
		target.Arch = ""
	}
	if kind == "app" {
		observeRole(m, "application")
	} else {
		observeRole(m, "metadata")
	}
	addTarget(m, target)
	return nil
}

// flatpakDictionary locates {string, variant} entries through GVariant array offsets.
// Each value is read only for a needed key, so inline compressed parts can be large.
func flatpakDictionary(r io.ReaderAt, fileSize int64, size uint64) (map[string][]byte, error) {
	width := flatpakOffsetWidth(size)
	last, err := flatpakRead(r, fileSize, int64(size)-int64(width), width)
	if err != nil {
		return nil, err
	}
	dataEnd := flatpakOffset(last)
	if dataEnd >= size || (size-dataEnd)%uint64(width) != 0 {
		return nil, malformed("Flatpak metadata array")
	}
	count := (size - dataEnd) / uint64(width)
	if count > 4096 {
		return nil, fmt.Errorf("%w: Flatpak metadata has %d entries", errInspectLimit, count)
	}
	offsets, err := flatpakRead(r, fileSize, int64(dataEnd), int(count)*width)
	if err != nil {
		return nil, err
	}
	fields := map[string][]byte{}
	seen := map[string]bool{}
	var previous uint64
	for i := 0; i < int(count); i++ {
		start := flatpakAlign(previous, 8)
		end := flatpakOffset(offsets[i*width : (i+1)*width])
		if end > dataEnd || end <= start {
			return nil, malformed("Flatpak metadata entry framing")
		}
		entryWidth := flatpakOffsetWidth(end - start)
		keyOffset, err := flatpakRead(r, fileSize, int64(end)-int64(entryWidth), entryWidth)
		if err != nil {
			return nil, err
		}
		keyEnd := flatpakOffset(keyOffset)
		valueStart, valueEnd := start+flatpakAlign(keyEnd, 8), end-uint64(entryWidth)
		if keyEnd == 0 || keyEnd > 1024 || valueStart >= valueEnd {
			return nil, malformed("Flatpak metadata key")
		}
		keyData, err := flatpakRead(r, fileSize, int64(start), int(flatpakAlign(keyEnd, 8)))
		if err != nil {
			return nil, err
		}
		key, ok := flatpakCString(keyData[:keyEnd])
		if !ok || !bytes.Equal(keyData[keyEnd:], make([]byte, len(keyData)-int(keyEnd))) || seen[key] {
			return nil, malformed("Flatpak metadata key")
		}
		seen[key] = true
		switch key {
		case "flatpak", "ref", "metadata", "ostree.endianness":
			if valueEnd-valueStart > 1<<20 {
				return nil, fmt.Errorf("%w: Flatpak %s metadata exceeds 1 MiB", errInspectLimit, key)
			}
			value, err := flatpakRead(r, fileSize, int64(valueStart), int(valueEnd-valueStart))
			if err != nil {
				return nil, err
			}
			fields[key] = value
		}
		previous = end
	}
	return fields, nil
}

// flatpakAppMetadata compares the ref against identity and CPU facts in its keyfile.
func flatpakAppMetadata(text, kind, name, arch string, m *ArtifactMetadata) error {
	group := "Application"
	if kind == "runtime" {
		group = "Runtime"
	}
	section := ""
	found := false
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return malformed("Flatpak application metadata")
			}
			section = line[1 : len(line)-1]
			if section == group {
				found = true
			} else if section == "Application" || section == "Runtime" {
				m.Conflicts = append(m.Conflicts, "Flatpak ref kind conflicts with application metadata")
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return malformed("Flatpak application metadata")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if section != group || (key != "name" && key != "runtime" && key != "sdk" && key != "arch" && key != "architecture") {
			continue
		}
		if previous, ok := values[key]; ok && previous != value {
			m.Conflicts = append(m.Conflicts, "Flatpak application metadata repeats conflicting "+key)
		}
		values[key] = value
	}
	if scanner.Err() != nil || !found || values["name"] == "" {
		return malformed("Flatpak application metadata")
	}
	if values["name"] != name {
		m.Conflicts = append(m.Conflicts, "Flatpak ref name conflicts with application metadata")
	}
	for _, key := range []string{"runtime", "sdk", "arch", "architecture"} {
		value, ok := values[key]
		if !ok {
			continue
		}
		observed := value
		if key == "runtime" || key == "sdk" {
			_, _, parsed, ok := flatpakReference("runtime/" + value)
			if !ok {
				return malformed("Flatpak application " + key)
			}
			observed = parsed
		}
		note(m, "", "flatpak.metadata."+key, value)
		if observed != arch {
			m.Conflicts = append(m.Conflicts, "Flatpak ref architecture conflicts with "+group+"."+key)
		}
	}
	return nil
}

func flatpakReference(ref string) (kind, name, arch string, ok bool) {
	parts := strings.Split(ref, "/")
	if len(parts) != 4 || (parts[0] != "app" && parts[0] != "runtime") || strings.Count(parts[1], ".") < 2 {
		return "", "", "", false
	}
	for _, part := range parts[1:] {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".") || strings.Contains(part, "..") {
			return "", "", "", false
		}
		for _, c := range part {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_') {
				return "", "", "", false
			}
		}
	}
	return parts[0], parts[1], parts[2], true
}

func flatpakArch(arch string) string {
	switch arch {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	case "i386":
		return "386"
	case "arm", "riscv64":
		return arch
	}
	return "flatpak:" + arch
}

func flatpakString(value []byte) (string, error) {
	if len(value) < 3 || string(value[len(value)-2:]) != "\x00s" {
		return "", malformed("Flatpak string metadata")
	}
	text, ok := flatpakCString(value[:len(value)-2])
	if !ok {
		return "", malformed("Flatpak string metadata")
	}
	return text, nil
}

func flatpakCString(value []byte) (string, bool) {
	if len(value) == 0 || bytes.IndexByte(value, 0) != len(value)-1 || !utf8.Valid(value[:len(value)-1]) {
		return "", false
	}
	return string(value[:len(value)-1]), true
}

func flatpakRead(r io.ReaderAt, size, offset int64, length int) ([]byte, error) {
	if length < 0 || offset < 0 || offset > size || int64(length) > size-offset {
		return nil, malformed("Flatpak framing")
	}
	b := make([]byte, length)
	if n, _ := r.ReadAt(b, offset); n != length {
		return nil, malformed("Flatpak metadata")
	}
	return b, nil
}

func flatpakOffsetWidth(size uint64) int {
	switch {
	case size <= 0xff:
		return 1
	case size <= 0xffff:
		return 2
	case size <= 0xffffffff:
		return 4
	}
	return 8
}

func flatpakOffset(b []byte) uint64 {
	var out uint64
	for i, v := range b {
		out |= uint64(v) << (8 * i)
	}
	return out
}

func flatpakAlign(value, alignment uint64) uint64 {
	return (value + alignment - 1) &^ (alignment - 1)
}
