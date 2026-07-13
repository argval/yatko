"use client";

import { useRouter } from "next/navigation";
import { useReleases } from "./use-releases";

export function VersionSelector({
  owner,
  repo,
  currentTag,
  showPrereleases,
}: {
  owner: string;
  repo: string;
  currentTag: string;
  showPrereleases: boolean;
}) {
  const router = useRouter();
  const { releases, loading } = useReleases(owner, repo);

  const visible = showPrereleases
    ? releases
    : releases.filter((r) => !r.prerelease);

  if (loading || visible.length <= 1) return null;

  function handleChange(tag: string) {
    if (tag === currentTag) return;
    const isLatest = visible[0]?.tag_name === tag && !visible[0]?.prerelease;
    router.push(isLatest ? `/p/${owner}/${repo}` : `/p/${owner}/${repo}/${tag}`);
  }

  return (
    <select
      value={currentTag}
      onChange={(e) => handleChange(e.target.value)}
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
