package archive

import (
	"fmt"
	"os"
	"strings"
)

// Group is a set of owner/repo aliases that share one mirrored latest release.
// Aliases[0] is the preferred sync source (usually upstream).
type Group struct {
	Aliases []string // "owner/repo"
}

// RepoKey returns the canonical "owner/repo" for alias index 0.
func (g Group) Canonical() string {
	if len(g.Aliases) == 0 {
		return ""
	}
	return g.Aliases[0]
}

// Contains reports whether owner/repo is in this group.
func (g Group) Contains(owner, repo string) bool {
	want := owner + "/" + repo
	for _, a := range g.Aliases {
		if a == want {
			return true
		}
	}
	return false
}

// ParseGroups parses ARCHIVE_GROUPS:
//
//	permissionlesstech/bitchat|argval/bitchat;permissionlesstech/bitchat-android|argval/bitchat-android
//
// Empty / unset → no groups (feature off). A single "-" disables explicitly.
func ParseGroups(s string) ([]Group, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil, nil
	}
	var groups []Group
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var aliases []string
		seen := map[string]bool{}
		for _, a := range strings.Split(part, "|") {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			if err := validateOwnerRepo(a); err != nil {
				return nil, err
			}
			if seen[a] {
				continue
			}
			seen[a] = true
			aliases = append(aliases, a)
		}
		if len(aliases) == 0 {
			continue
		}
		groups = append(groups, Group{Aliases: aliases})
	}
	return groups, nil
}

// GroupsFromEnv loads ARCHIVE_GROUPS from the environment.
func GroupsFromEnv() ([]Group, error) {
	return ParseGroups(os.Getenv("ARCHIVE_GROUPS"))
}

func validateOwnerRepo(s string) error {
	owner, repo, ok := strings.Cut(s, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return fmt.Errorf("invalid archive repo %q (want owner/repo)", s)
	}
	return nil
}

// SplitOwnerRepo splits "owner/repo".
func SplitOwnerRepo(s string) (owner, repo string, err error) {
	owner, repo, ok := strings.Cut(s, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("invalid owner/repo %q", s)
	}
	return owner, repo, nil
}
