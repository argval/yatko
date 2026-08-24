"use server";

import { revalidatePath } from "next/cache";

const SLUG_RE = /^[a-zA-Z0-9._-]+$/;

/**
 * Drop a Full Route Cache entry for a release page. Used after transient
 * upstream failures so ISR cannot pin an error card for up to `revalidate`.
 */
export async function bustReleasePageCache(
  owner: string,
  repo: string,
  version?: string,
): Promise<void> {
  if (!SLUG_RE.test(owner) || !SLUG_RE.test(repo)) return;
  if (version !== undefined) {
    // Tags may contain `/` (joined catch-all); reject anything outside
    // slug + slash so this cannot revalidate arbitrary paths.
    if (version.length === 0 || version.length > 200) return;
    if (!/^[a-zA-Z0-9._/-]+$/.test(version)) return;
    revalidatePath(`/p/${owner}/${repo}/${version}`);
    return;
  }
  revalidatePath(`/p/${owner}/${repo}`);
}
