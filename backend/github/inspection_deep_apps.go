package github

import (
	"encoding/xml"
	"path"
	"strings"

	"howett.net/plist"
)

// inspectMacApps follows each top-level bundle's declared main executable.
// Nested helper apps cannot add CPU targets to their containing application.
func (a *deepArchive) inspectMacApps() (ArtifactMetadata, bool, error) {
	var infos []deepEntry
	appLayout := false
	for _, e := range a.entries {
		name := strings.ToLower(strings.ReplaceAll(e.path, "\\", "/"))
		if strings.Contains(name, ".app/contents/") {
			appLayout = true
		}
		if e.directory || !strings.HasSuffix(name, ".app/contents/info.plist") {
			continue
		}
		outer := strings.TrimSuffix(name, ".app/contents/info.plist")
		if !strings.Contains(outer, ".app/") {
			infos = append(infos, e)
		}
	}
	if len(infos) == 0 {
		if appLayout {
			return deepUnknown("application bundle has no top-level Info.plist"), true, nil
		}
		return ArtifactMetadata{}, false, nil
	}
	if len(infos) > 32 {
		return deepUnknown("more than thirty-two top-level application bundles"), true, nil
	}
	var results []ArtifactMetadata
	for _, info := range infos {
		if _, unique := a.appEntry(path.Clean(info.path)); !unique {
			return deepUnknown("application Info.plist path is ambiguous"), true, nil
		}
		if info.size <= 0 || info.size > deepMaxPrefix {
			return deepUnknown("application Info.plist exceeds its read limit"), true, nil
		}
		data, err := a.read(info, 0, info.size)
		if err != nil {
			return ArtifactMetadata{}, true, err
		}
		var bundle struct {
			Executable string `plist:"CFBundleExecutable"`
		}
		if _, err := plist.Unmarshal(data, &bundle); err != nil {
			return deepUnknown("application Info.plist is invalid"), true, nil
		}
		name := bundle.Executable
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00\r\n") {
			return deepUnknown("application has no valid CFBundleExecutable filename"), true, nil
		}
		main := path.Join(path.Dir(strings.ReplaceAll(info.path, "\\", "/")), "MacOS", name)
		e, ok := a.appEntry(main)
		if !ok {
			return deepUnknown("declared application executable is missing or ambiguous: " + main), true, nil
		}
		m, machO, err := a.macBinary(e)
		if err != nil {
			return m, true, err
		}
		if !machO {
			// A launcher script (Sniffnet's wrapper.sh) starts the real Mach-O
			// next to it in Contents/MacOS, so judge those binaries instead.
			head, err := a.read(e, 0, min(e.size, 2))
			if err != nil {
				return ArtifactMetadata{}, true, err
			}
			if string(head) != "#!" {
				return deepUnknown("declared Mac application is not a Mach-O executable"), true, nil
			}
			if m, err = a.macLauncherTargets(path.Dir(main)); err != nil || m.Status != "inspected" {
				return m, true, err
			}
			deepNote(&m, "macos.bundle.launcher", main)
		}
		if !deepProven(m) {
			return deepUnknown("declared application executable has no proven CPU target"), true, nil
		}
		deepNote(&m, "macos.bundle.executable", main)
		results = append(results, m)
	}
	return commonDeepTargets(results), true, nil
}

// macBinary reads one bundle executable and reports whether it is a Mach-O
// for macOS. Old headers without a platform take it from the bundle layout.
func (a *deepArchive) macBinary(e deepEntry) (ArtifactMetadata, bool, error) {
	m, err := a.binary(e)
	if err != nil {
		return m, false, err
	}
	machO := false
	for _, evidence := range m.Evidence {
		machO = machO || strings.HasSuffix(evidence.Field, "macho.cputype")
	}
	if !machO {
		return m, false, nil
	}
	for i := range m.Targets {
		if m.Targets[i].Platform == "" {
			m.Targets[i].Platform = "macos"
		}
		if m.Targets[i].Platform != "macos" {
			return deepUnknown("declared Mac application has a different executable platform"), true, nil
		}
	}
	return m, true, nil
}

// macLauncherTargets judges the Mach-O files directly inside a bundle's
// Contents/MacOS when its declared executable is a script. They must agree on
// their CPU targets, so a helper binary cannot widen or narrow the app.
func (a *deepArchive) macLauncherTargets(dir string) (ArtifactMetadata, error) {
	var results []ArtifactMetadata
	for _, e := range a.entries {
		p := path.Clean(strings.ReplaceAll(e.path, "\\", "/"))
		if e.directory || path.Dir(p) != dir {
			continue
		}
		if len(results) >= 8 {
			return deepUnknown("launcher script has more than eight sibling executables"), nil
		}
		m, machO, err := a.macBinary(e)
		if err != nil {
			return m, err
		}
		if machO && deepProven(m) {
			results = append(results, m)
		}
	}
	if len(results) == 0 {
		return deepUnknown("launcher script has no Mach-O executable beside it"), nil
	}
	return commonDeepTargets(results), nil
}

type deepAppxManifest struct {
	XMLName  xml.Name `xml:"Package"`
	Identity []struct {
		Architecture string `xml:"ProcessorArchitecture,attr"`
	} `xml:"Identity"`
	Properties struct {
		ResourcePackage bool `xml:"ResourcePackage"`
		Framework       bool `xml:"Framework"`
	} `xml:"Properties"`
	Applications []struct {
		Executable string `xml:"Executable,attr"`
	} `xml:"Applications>Application"`
}

type deepAppxBundle struct {
	XMLName  xml.Name `xml:"Bundle"`
	Packages []struct {
		Type         string `xml:"Type,attr"`
		Architecture string `xml:"Architecture,attr"`
		Filename     string `xml:"FileName,attr"`
		Stub         bool   `xml:"IsStub,attr"`
	} `xml:"Packages>Package"`
}

// inspectMSIX uses package architecture constraints or declared application headers.
// Bundle resource packages never contribute CPU targets.
func (a *deepArchive) inspectMSIX() (ArtifactMetadata, bool, error) {
	counts := map[string]int{}
	for _, e := range a.entries {
		if !e.directory {
			counts[strings.ToLower(path.Clean(strings.ReplaceAll(e.path, "\\", "/")))]++
		}
	}
	if counts["appxmanifest.xml"] > 1 || counts["appxmetadata/appxbundlemanifest.xml"] > 1 {
		return deepUnknown("MSIX manifest path is ambiguous"), true, nil
	}
	manifest, haveManifest := a.appEntry("AppxManifest.xml")
	bundle, haveBundle := a.appEntry("AppxMetadata/AppxBundleManifest.xml")
	if !haveManifest && !haveBundle {
		return ArtifactMetadata{}, false, nil
	}
	if haveManifest && haveBundle {
		return deepUnknown("archive contains both package and bundle manifests"), true, nil
	}
	e := manifest
	if haveBundle {
		e = bundle
	}
	if e.size <= 0 || e.size > deepMaxPrefix {
		return deepUnknown("MSIX manifest exceeds its read limit"), true, nil
	}
	data, err := a.read(e, 0, e.size)
	if err != nil {
		return ArtifactMetadata{}, true, err
	}
	if haveBundle {
		m, err := a.inspectMSIXBundle(data)
		return m, true, err
	}
	var pkg deepAppxManifest
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return deepUnknown("MSIX application manifest is invalid"), true, nil
	}
	if len(pkg.Identity) != 1 {
		return deepUnknown("MSIX application identity is missing or ambiguous"), true, nil
	}
	if pkg.Properties.ResourcePackage || pkg.Properties.Framework || len(pkg.Applications) == 0 {
		return deepUnknown("MSIX contains no launchable application"), true, nil
	}
	if arch := deepMSIXArch(pkg.Identity[0].Architecture); arch != "" {
		m := ArtifactMetadata{Status: "inspected", Role: "installer", Targets: []ArtifactTarget{{Platform: "windows", Arch: arch}}}
		deepNote(&m, "msix.identity.architecture", pkg.Identity[0].Architecture)
		return m, true, nil
	}
	if architecture := strings.ToLower(pkg.Identity[0].Architecture); architecture != "" && architecture != "neutral" {
		return deepUnknown("MSIX package has an unsupported processor architecture"), true, nil
	}
	var results []ArtifactMetadata
	for _, application := range pkg.Applications {
		name, ok := deepRelativeAppPath(application.Executable)
		if !ok {
			return deepUnknown("neutral MSIX has no valid declared executable"), true, nil
		}
		e, ok := a.appEntry(name)
		if !ok {
			return deepUnknown("neutral MSIX executable is missing or ambiguous: " + name), true, nil
		}
		m, err := a.binary(e)
		if err != nil {
			return m, true, err
		}
		if !deepProven(m) {
			return deepUnknown("neutral MSIX executable has no proven CPU target"), true, nil
		}
		for _, target := range m.Targets {
			if target.Platform != "windows" {
				return deepUnknown("MSIX declares an executable for a different platform"), true, nil
			}
		}
		for _, evidence := range m.Evidence {
			if strings.HasSuffix(evidence.Field, "pe.clr") {
				return deepUnknown("neutral managed MSIX needs CLR runtime architecture evidence"), true, nil
			}
		}
		deepNote(&m, "msix.application.executable", name)
		results = append(results, m)
	}
	m := commonDeepTargets(results)
	if deepProven(m) {
		m.Role = "installer"
	}
	return m, true, nil
}

// inspectMSIXBundle unions independently installable application packages.
func (a *deepArchive) inspectMSIXBundle(data []byte) (ArtifactMetadata, error) {
	var bundle deepAppxBundle
	if err := xml.Unmarshal(data, &bundle); err != nil {
		return deepUnknown("MSIX bundle manifest is invalid"), nil
	}
	if len(bundle.Packages) > 256 {
		return deepUnknown("MSIX bundle exceeds 256 packages"), nil
	}
	out := ArtifactMetadata{Status: "inspected", Role: "installer"}
	seen := map[string]bool{}
	for _, pkg := range bundle.Packages {
		if !strings.EqualFold(pkg.Type, "application") || pkg.Stub {
			continue
		}
		name, valid := deepRelativeAppPath(pkg.Filename)
		e, exists := a.appEntry(name)
		if !valid || !exists {
			return deepUnknown("MSIX bundle application payload is missing or ambiguous"), nil
		}
		if seen[strings.ToLower(name)] {
			return deepUnknown("MSIX bundle repeats an application payload"), nil
		}
		seen[strings.ToLower(name)] = true
		if arch := deepMSIXArch(pkg.Architecture); arch != "" {
			addTarget(&out, ArtifactTarget{Platform: "windows", Arch: arch})
			deepNote(&out, "msix.bundle.architecture", pkg.Architecture)
			continue
		}
		if architecture := strings.ToLower(pkg.Architecture); architecture != "" && architecture != "neutral" {
			return deepUnknown("MSIX bundle application has an unsupported processor architecture"), nil
		}
		filename, err := a.stage(e)
		if err != nil {
			return out, err
		}
		m, err := a.session.inspect(filename, e.path, a.depth+1)
		if err != nil {
			return m, err
		}
		if !deepProven(m) {
			return m, nil
		}
		for _, target := range m.Targets {
			if target.Platform != "windows" {
				return deepUnknown("MSIX bundle contains a different application platform"), nil
			}
			addTarget(&out, target)
		}
		out.Evidence = append(out.Evidence, m.Evidence...)
	}
	if !deepProven(out) {
		return deepUnknown("MSIX bundle has no architecture-bound application package"), nil
	}
	return out, nil
}

// appEntry requires one matching archive entry, including case-insensitive aliases.
func (a *deepArchive) appEntry(name string) (deepEntry, bool) {
	var found deepEntry
	count := 0
	for _, e := range a.entries {
		candidate := path.Clean(strings.ReplaceAll(e.path, "\\", "/"))
		if !e.directory && strings.EqualFold(candidate, name) {
			found, count = e, count+1
		}
	}
	return found, count == 1
}

// deepRelativeAppPath rejects absolute paths and escapes from the package root.
func deepRelativeAppPath(value string) (string, bool) {
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" || strings.ContainsAny(value, ":\x00\r\n") || path.IsAbs(value) {
		return "", false
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return "", false
		}
	}
	value = path.Clean(value)
	return value, value != "."
}

// deepMSIXArch maps only explicit package CPU constraints to picker targets.
func deepMSIXArch(value string) string {
	switch strings.ToLower(value) {
	case "x64":
		return "amd64"
	case "x86":
		return "386"
	case "arm64":
		return "arm64"
	case "arm":
		return "arm"
	}
	return ""
}
