package github

import (
	"path"
	"strings"
)

// inspectRuntimeLibraries distinguishes library-only downloads from JVM distributions
// whose bundled native runtime sets the host CPU. Cross-compilation libraries are excluded.
func (a *deepArchive) inspectRuntimeLibraries() (ArtifactMetadata, bool, error) {
	var libraries, launchers []deepEntry
	libraryOnly, jar := true, false
	for _, e := range a.entries {
		if e.directory || e.size == 0 {
			continue
		}
		name := strings.ToLower(path.Clean(e.path))
		isLibrary := hasAnySuffix(name, ".so", ".dylib", ".dll", ".prx")
		libraryOnly = libraryOnly && isLibrary
		if isLibrary && (strings.Contains(name, "/nativelib/") || strings.HasPrefix(name, "nativelib/")) {
			libraries = append(libraries, e)
		}
		if !strings.Contains(path.Base(name), ".") && strings.Contains(name, "/bin/") && e.size > 0 && e.size <= 64<<10 {
			launchers = append(launchers, e)
		}
		jar = jar || strings.HasSuffix(name, ".jar")
	}
	if libraryOnly {
		libraries = nil
		for _, e := range a.entries {
			if !e.directory && e.size != 0 {
				libraries = append(libraries, e)
			}
		}
	}
	if len(libraries) == 0 || len(libraries) > 128 {
		return ArtifactMetadata{}, false, nil
	}
	jvm := false
	if !libraryOnly && jar {
		for _, e := range launchers {
			data, err := a.read(e, 0, e.size)
			if err != nil {
				return ArtifactMetadata{}, true, err
			}
			text := string(data)
			if strings.HasPrefix(text, "#!") && (strings.Contains(text, "-cp ") || strings.Contains(text, "-jar ")) && strings.Contains(strings.ToLower(text), "java") {
				jvm = true
				break
			}
		}
	}
	if !libraryOnly && !jvm {
		return ArtifactMetadata{}, false, nil
	}
	var observed []ArtifactMetadata
	for _, e := range libraries {
		m, err := a.headers(e)
		if err != nil {
			return m, true, err
		}
		if m.Role != "metadata" || m.Status != "inspected" || len(m.Targets) == 0 || len(m.Conflicts) > 0 {
			return deepUnknown("native runtime or library archive contains an unrecognized executable"), true, nil
		}
		deepNote(&m, "runtime.library", e.path)
		observed = append(observed, m)
	}
	if libraryOnly {
		m := ArtifactMetadata{Status: "inspected", Role: "metadata"}
		deepNote(&m, "archive.role", "all file entries are shared libraries; no application launcher")
		return m, true, nil
	}
	m := commonDeepTargets(observed)
	if !deepProven(m) {
		return m, true, nil
	}
	m.Role = "application"
	deepNote(&m, "application.runtime", "JVM launcher with bundled native runtime libraries")
	return m, true, nil
}
