"use client";

import { useRouter } from "next/navigation";
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
  const router = useRouter();

  const hasPrereleases = releases.some((r) => r.prerelease);
  if (!hasPrereleases && !isCurrentPrerelease) return null;

  function handleToggle(e: React.ChangeEvent<HTMLInputElement>) {
    if (e.target.checked) {
      const pre = releases.find((r) => r.prerelease);
      if (pre) router.push(`/p/${owner}/${repo}/${pre.tag_name}`);
    } else {
      router.push(`/p/${owner}/${repo}`);
    }
  }

  return (
    <label className="flex items-center gap-2 text-xs text-muted cursor-pointer select-none hover:text-foreground transition-colors">
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
