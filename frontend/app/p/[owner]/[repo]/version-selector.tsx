"use client";

import { useReleaseNav } from "./release-nav-shell";
import type { ReleaseSummary } from "./release-page";

export function VersionSelector({
  owner,
  repo,
  currentTag,
  showPrereleases,
  releases,
}: {
  owner: string;
  repo: string;
  currentTag: string;
  showPrereleases: boolean;
  releases: ReleaseSummary[];
}) {
  const { navigate, prefetch } = useReleaseNav();

  const visible = showPrereleases
    ? releases
    : releases.filter((r) => !r.prerelease);

  if (visible.length <= 1) return null;

  function hrefFor(tag: string) {
    const isLatest = visible[0]?.tag_name === tag && !visible[0]?.prerelease;
    return isLatest ? `/p/${owner}/${repo}` : `/p/${owner}/${repo}/${tag}`;
  }

  function handleChange(tag: string) {
    if (tag === currentTag) return;
    navigate(hrefFor(tag));
  }

  return (
    <select
      value={currentTag}
      onChange={(e) => handleChange(e.target.value)}
      onFocus={() => {
        for (const r of visible.slice(0, 8)) {
          if (r.tag_name !== currentTag) prefetch(hrefFor(r.tag_name));
        }
      }}
      className="px-3 py-1.5 text-sm rounded-lg border border-border bg-surface text-foreground focus:outline-none focus:ring-2 focus:ring-foreground/15 cursor-pointer"
      aria-label="Select version"
    >
      {visible.map((r) => (
        <option key={r.tag_name} value={r.tag_name}>
          {r.tag_name}
          {r.prerelease ? " (pre-release)" : ""}
        </option>
      ))}
    </select>
  );
}
