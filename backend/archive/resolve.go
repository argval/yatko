package archive

import (
	"context"
	"errors"

	"github.com/argval/yatko/github"
)

// Resolve applies archive overlay / stale-while-unavailable for an archived
// repo. Non-archived repos pass through unchanged.
//
// version empty = latest. When version is set and does not match the mirrored
// tag, the archive is not used (GitHub result or error stands).
func (s *Service) Resolve(ctx context.Context, owner, repo, version string, release *github.Release, fetchErr error) (*github.Release, error) {
	if s == nil || !s.Enabled() || s.FindGroup(owner, repo) == nil {
		return release, fetchErr
	}

	m, err := LoadManifest(ctx, s.cache, owner, repo)
	if err != nil {
		if fetchErr != nil {
			return nil, fetchErr
		}
		return release, nil
	}
	if m == nil {
		return release, fetchErr
	}

	if version != "" && version != m.Tag {
		return release, fetchErr
	}

	if fetchErr != nil || release == nil {
		return m.ToRelease(owner, repo), nil
	}

	return Overlay(release, m), nil
}

// IsArchived reports whether owner/repo is in a configured archive group.
func (s *Service) IsArchived(owner, repo string) bool {
	return s != nil && s.FindGroup(owner, repo) != nil
}

// ErrNotConfigured is returned when sync is attempted without ARCHIVE_GROUPS.
var ErrNotConfigured = errors.New("archive not configured")
