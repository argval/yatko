package github

import (
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

type burnContainer struct{ offset, size int64 }

type burnManifest struct {
	XMLName    xml.Name `xml:"BurnManifest"`
	Containers []struct {
		ID    string `xml:"Id,attr"`
		Index string `xml:"AttachedIndex,attr"`
	} `xml:"Container"`
	Payloads []struct {
		Path      string `xml:"FilePath,attr"`
		Source    string `xml:"SourcePath,attr"`
		Packaging string `xml:"Packaging,attr"`
		Container string `xml:"Container,attr"`
	} `xml:"Payload"`
}

// inspectBurn follows WiX's section table and manifest into attached application packages.
// The bootstrapper's UX executable is excluded. Layout: wixtoolset/wix BurnCommon.cs.
func (s *deepSession) inspectBurn(filename string, depth int) (ArtifactMetadata, bool, error) {
	f, err := os.Open(filename)
	if err != nil {
		return ArtifactMetadata{}, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ArtifactMetadata{}, false, err
	}
	containers, found, err := readBurnContainers(f, info.Size())
	if !found || err != nil {
		return ArtifactMetadata{}, found, err
	}
	if len(containers) < 2 {
		return deepUnknown("WiX bootstrapper has no attached application container"), true, nil
	}
	ux, err := s.stageSection(f, containers[0])
	if err != nil {
		return ArtifactMetadata{}, true, err
	}
	a, err := s.openArchive(ux, "ux.cab")
	if err != nil {
		return ArtifactMetadata{}, true, err
	}
	entry, ok := a.appEntry("0")
	if !ok || entry.size <= 0 || entry.size > deepMaxPrefix {
		return deepUnknown("WiX UX container has no bounded manifest"), true, nil
	}
	b, err := a.read(entry, 0, entry.size)
	if err != nil {
		return ArtifactMetadata{}, true, err
	}
	var manifest burnManifest
	if err := xml.Unmarshal(b, &manifest); err != nil {
		return deepUnknown("WiX application manifest is invalid"), true, nil
	}
	if manifest.XMLName.Space != "http://wixtoolset.org/schemas/v4/2008/Burn" && manifest.XMLName.Space != "http://schemas.microsoft.com/wix/2008/Burn" {
		return deepUnknown("WiX application manifest has an unknown namespace"), true, nil
	}
	byID := map[string]int{}
	for _, c := range manifest.Containers {
		index, err := strconv.Atoi(c.Index)
		if err != nil || index <= 0 || index >= len(containers) || c.ID == "" {
			return deepUnknown("WiX manifest container index is invalid"), true, nil
		}
		if _, duplicate := byID[c.ID]; duplicate {
			return deepUnknown("WiX manifest container identity is ambiguous"), true, nil
		}
		byID[c.ID] = index
	}
	opened := map[int]*deepArchive{}
	var results []ArtifactMetadata
	for _, p := range manifest.Payloads {
		if !hasAnySuffix(strings.ToLower(p.Path), ".msi", ".msix", ".exe") || strings.ToLower(p.Packaging) != "embedded" {
			continue
		}
		index, ok := byID[p.Container]
		if !ok {
			return deepUnknown("WiX application package is outside its attached containers"), true, nil
		}
		container := opened[index]
		if container == nil {
			file, err := s.stageSection(f, containers[index])
			if err != nil {
				return ArtifactMetadata{}, true, err
			}
			container, err = s.openArchive(file, "attached.cab")
			if err != nil {
				return ArtifactMetadata{}, true, err
			}
			opened[index] = container
		}
		e, ok := container.appEntry(p.Source)
		if !ok {
			return deepUnknown("WiX declared application payload is missing or ambiguous"), true, nil
		}
		file, err := container.stage(e)
		if err != nil {
			return ArtifactMetadata{}, true, err
		}
		m, err := s.inspect(file, path.Base(p.Path), depth+1)
		if err != nil || !deepProven(m) {
			return m, true, err
		}
		deepNote(&m, "wix.application.package", p.Path)
		results = append(results, m)
	}
	if len(results) == 0 {
		return deepUnknown("WiX manifest has no embedded application package"), true, nil
	}
	m := commonDeepTargets(results)
	m.Role = "installer"
	return m, true, nil
}

// readBurnContainers validates fixed-size PE/Burn framing before using file offsets.
func readBurnContainers(r io.ReaderAt, size int64) ([]burnContainer, bool, error) {
	le := binary.LittleEndian
	var dos [64]byte
	if n, _ := r.ReadAt(dos[:], 0); n != len(dos) || string(dos[:2]) != "MZ" {
		return nil, false, nil
	}
	pe := int64(le.Uint32(dos[60:]))
	var coff [24]byte
	if pe > size-24 {
		return nil, false, nil
	}
	if n, _ := r.ReadAt(coff[:], pe); n != len(coff) || string(coff[:4]) != "PE\x00\x00" {
		return nil, false, nil
	}
	count, optSize := int(le.Uint16(coff[6:])), int64(le.Uint16(coff[20:]))
	if count > 96 || optSize > 4096 || pe+24+optSize+int64(count*40) > size {
		return nil, false, nil
	}
	sections := make([]byte, count*40)
	if _, err := r.ReadAt(sections, pe+24+optSize); err != nil {
		return nil, false, err
	}
	var section []byte
	for i := 0; i < len(sections); i += 40 {
		if string(sections[i:i+8]) == ".wixburn" {
			if section != nil {
				return nil, true, malformed("ambiguous WiX section")
			}
			section = sections[i : i+40]
		}
	}
	if section == nil {
		return nil, false, nil
	}
	raw, off := int64(le.Uint32(section[16:])), int64(le.Uint32(section[20:]))
	if raw < 52 || raw > 4096 || off > size-raw {
		return nil, true, malformed("WiX section bounds")
	}
	header := make([]byte, raw)
	if _, err := r.ReadAt(header, off); err != nil {
		return nil, true, err
	}
	if le.Uint32(header) != 0x00f14300 || le.Uint32(header[4:]) != 2 || le.Uint32(header[40:]) != 1 {
		return nil, true, fmt.Errorf("%w: unsupported WiX section version or container codec", errInspectLimit)
	}
	containers := int(le.Uint32(header[44:]))
	if containers < 1 || containers > 16 || 48+containers*4 > len(header) {
		return nil, true, malformed("WiX container count")
	}
	stub := int64(le.Uint32(header[24:]))
	uxSize := int64(le.Uint32(header[48:]))
	engine := stub + uxSize
	if signature := int64(le.Uint32(header[32:])); signature > 0 {
		engine = signature + int64(le.Uint32(header[36:]))
	}
	if stub < off+raw || uxSize <= 0 || engine < stub+uxSize {
		return nil, true, malformed("WiX engine extents")
	}
	out := []burnContainer{{stub, uxSize}}
	for i := 1; i < containers; i++ {
		length := int64(le.Uint32(header[48+i*4:]))
		out = append(out, burnContainer{engine, length})
		engine += length
	}
	for _, c := range out {
		var magic [4]byte
		if c.size < 36 || c.offset > size-c.size || c.offset < 0 {
			return nil, true, malformed("WiX attached container bounds")
		}
		if _, err := r.ReadAt(magic[:], c.offset); err != nil || string(magic[:]) != "MSCF" {
			return nil, true, malformed("WiX cabinet magic")
		}
	}
	return out, true, nil
}

func (s *deepSession) stageSection(r io.ReaderAt, part burnContainer) (string, error) {
	if part.size > deepMaxDownload || s.staged+part.size > deepMaxStaged {
		return "", fmt.Errorf("%w: attached container exceeds disk budget", errInspectLimit)
	}
	f, err := os.CreateTemp(s.dir, "container-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, &deepContextReader{ctx: s.ctx, r: io.NewSectionReader(r, part.offset, part.size)}); err != nil {
		return "", err
	}
	s.staged += part.size
	return f.Name(), nil
}
