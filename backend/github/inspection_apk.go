package github

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"io"
	"strings"
)

// inspectAPK distinguishes Android ZIPs from Alpine packages by their contents.
// Alpine v2 uses independent gzip tar segments; ADB/v3 remains unsupported.
func inspectAPK(f *remoteFile, m *ArtifactMetadata) error {
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], 0); err != nil {
		return err
	}
	switch {
	case string(magic[:3]) == "ADB":
		addTarget(m, ArtifactTarget{Platform: "linux"})
		note(m, "", "apk.alpine_format", "ADB")
		return installerUnsupported(m, "Alpine APK v3/ADB")
	case string(magic[:2]) == "PK":
		n := min(f.size, archiveTailLimit)
		tail := make([]byte, n)
		if _, err := f.ReadAt(tail, f.size-n); err != nil {
			return err
		}
		zr, err := boundedZIPReader(f, f.size)
		if err != nil {
			return err
		}
		if len(zr.File) > 4096 {
			return errInspectLimit
		}
		for _, entry := range zr.File {
			if entry.Name == "AndroidManifest.xml" && !entry.FileInfo().IsDir() {
				addTarget(m, ArtifactTarget{Platform: "android"})
				m.Role = "installer"
				note(m, "", "apk.android_manifest", "AndroidManifest.xml")
				return nil
			}
		}
		m.Role = "metadata"
		return installerUnsupported(m, "APK ZIP without root AndroidManifest.xml")
	case magic[0] == 0x1f && magic[1] == 0x8b:
		return inspectAlpineAPK(f, m)
	default:
		return installerUnsupported(m, "unrecognized APK container")
	}
}

// inspectAlpineAPK bounds members, entries, and total inflation across all segments.
// PKGINFO is in the control member, so payload contents need not be downloaded.
func inspectAlpineAPK(f *remoteFile, m *ArtifactMetadata) error {
	br := bufio.NewReader(io.NewSectionReader(f, 0, f.size))
	budget := &capReader{n: inspectMaxInflated}
	entries := 0
	for member := 0; member < 8; member++ {
		gz, err := gzip.NewReader(br)
		if err == io.EOF {
			return installerUnsupported(m, "gzip APK without .PKGINFO")
		}
		if err != nil {
			return err
		}
		gz.Multistream(false)
		budget.r = gz
		tr := tar.NewReader(budget)
		var pkginfo []byte
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				gz.Close()
				return err
			}
			entries++
			if entries > 256 {
				gz.Close()
				return errInspectLimit
			}
			name := strings.TrimPrefix(h.Name, "./")
			if name != ".PKGINFO" || h.Typeflag != tar.TypeReg {
				continue
			}
			if pkginfo != nil {
				gz.Close()
				return malformed("duplicate Alpine .PKGINFO")
			}
			if h.Size < 0 || h.Size > inspectHeadBytes {
				gz.Close()
				return errInspectLimit
			}
			pkginfo, err = io.ReadAll(tr)
			if err != nil {
				gz.Close()
				return err
			}
		}
		// Drain only this member to validate its gzip checksum and truncated trailers.
		_, err = io.Copy(io.Discard, budget)
		gz.Close()
		if err != nil {
			return err
		}
		if pkginfo != nil {
			return alpinePackageInfo(pkginfo, m)
		}
	}
	return errInspectLimit
}

// alpinePackageInfo reads the scalar architecture using apk-tools' " = " syntax.
func alpinePackageInfo(b []byte, m *ArtifactMetadata) error {
	arch := ""
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, " = ")
		if !ok {
			return malformed("Alpine .PKGINFO field")
		}
		if key != "arch" {
			continue
		}
		if arch != "" || value == "" || len(value) > 128 || strings.ContainsAny(value, " \t\r\x00") {
			return malformed("Alpine .PKGINFO architecture")
		}
		arch = value
	}
	if arch == "" {
		return malformed("Alpine .PKGINFO architecture missing")
	}
	note(m, "", "apk.alpine_arch", arch)
	switch arch {
	case "x86_64", "amd64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	case "x86", "386":
		arch = "386"
	case "armhf", "armv7":
		arch = "arm"
	case "noarch":
		arch = ""
	}
	addTarget(m, ArtifactTarget{Platform: "linux", Arch: arch})
	m.Role = "installer"
	return nil
}
