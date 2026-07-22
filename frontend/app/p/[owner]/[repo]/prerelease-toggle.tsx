"use client";

import { useReleaseNav } from "./release-nav-shell";
import type { ReleaseSummary } from "./release-page";

export function PrereleaseToggle({
  owner,
  repo,
  isCurrentPrerelease,
  releases,
}: {
  owner: string;
  repo: string;
  isCurrentPrerelease: boolean;
  releases: ReleaseSummary[];
}) {
  const { navigate, prefetch } = useReleaseNav();

  const hasPrereleases = releases.some((r) => r.prerelease);
  if (!hasPrereleases && !isCurrentPrerelease) return null;

  const pre = releases.find((r) => r.prerelease);
  const preHref = pre ? `/p/${owner}/${repo}/${pre.tag_name}` : null;
  const latestHref = `/p/${owner}/${repo}`;

  function handleToggle(e: React.ChangeEvent<HTMLInputElement>) {
    if (e.target.checked) {
      if (preHref) navigate(preHref);
    } else {
      navigate(latestHref);
    }
  }

  return (
    <label
      className="flex items-center gap-2 text-xs text-muted cursor-pointer select-none hover:text-foreground transition-colors"
      onMouseEnter={() => {
        if (preHref) prefetch(preHref);
        prefetch(latestHref);
      }}
    >
      <input
        type="checkbox"
        checked={isCurrentPrerelease}
        onChange={handleToggle}
        className="rounded border-foreground/20 accent-foreground"
      />
      Include pre-releases
    </label>
  );
}
