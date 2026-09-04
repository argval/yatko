package picker

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed catalog.json
var catalogJSON []byte

type platformEntry struct {
	Aliases    []string `json:"aliases"`
	Extensions []string `json:"extensions"`
}

type formatEntry struct {
	Kind     string `json:"kind"`
	Platform string `json:"platform,omitempty"`
}

type catalogFile struct {
	Platforms          map[string]platformEntry `json:"platforms"`
	Architectures      map[string][]string      `json:"architectures"`
	Libc               map[string][]string      `json:"libc"`
	Variants           []string                 `json:"variants"`
	NonNative          []string                 `json:"nonNative"`
	Source             []string                 `json:"source"`
	AmbiguousArchives  []string                 `json:"ambiguousArchives"`
	Formats            map[string]formatEntry   `json:"formats"`
	PreferAliases      map[string]string        `json:"preferAliases"`
	LinuxRPMExtensions []string                 `json:"linuxRpmExtensions"`
	formatKeysByLength []string                 // longest first, for suffix match
}

var catalog catalogFile

func init() {
	if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
		panic("picker: catalog.json: " + err.Error())
	}
	if err := catalog.validate(); err != nil {
		panic("picker: catalog.json: " + err.Error())
	}
	catalog.formatKeysByLength = formatKeysLongestFirst(catalog.Formats)
}

func (c catalogFile) validate() error {
	for _, p := range []Platform{Windows, MacOS, Linux, Android, IOS} {
		if _, ok := c.Platforms[string(p)]; !ok {
			return fmt.Errorf("missing platform %q", p)
		}
	}
	for _, a := range []Arch{AMD64, ARM64, ARM, X86} {
		if _, ok := c.Architectures[string(a)]; !ok {
			return fmt.Errorf("missing architecture %q", a)
		}
	}
	if len(c.Formats) == 0 {
		return fmt.Errorf("no formats")
	}
	return nil
}

func formatKeysLongestFirst(formats map[string]formatEntry) []string {
	keys := make([]string, 0, len(formats))
	for k := range formats {
		keys = append(keys, k)
	}
	// Simple insertion sort by length descending — catalog is tiny.
	for i := 1; i < len(keys); i++ {
		j := i
		for j > 0 && len(keys[j-1]) < len(keys[j]) {
			keys[j-1], keys[j] = keys[j], keys[j-1]
			j--
		}
	}
	return keys
}

func platformExts(p Platform) ([]string, bool) {
	e, ok := catalog.Platforms[string(p)]
	if !ok || len(e.Extensions) == 0 {
		return nil, false
	}
	return e.Extensions, true
}

func platformAliases(p Platform) []string {
	e, ok := catalog.Platforms[string(p)]
	if !ok {
		return nil
	}
	return e.Aliases
}

func archAliases(a Arch) []string {
	return catalog.Architectures[string(a)]
}

func matchFormat(canonical string) (key string, entry formatEntry, ok bool) {
	for _, k := range catalog.formatKeysByLength {
		if strings.HasSuffix(canonical, "."+k) {
			return k, catalog.Formats[k], true
		}
	}
	return "", formatEntry{}, false
}
