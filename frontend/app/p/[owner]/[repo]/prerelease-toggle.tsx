"use client";

import { useRouter } from "next/navigation";
import { useReleases } from "./use-releases";

export function PrereleaseToggle({
  owner,
  repo,
  isCurrentPrerelease,
}: {
  owner: string;
  repo: string;
  isCurrentPrerelease: boolean;
}) {
  const router = useRouter();
  const { releases } = useReleases(owner, repo);

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
