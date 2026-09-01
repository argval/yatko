import type { ReleaseData, ReleaseSummary, RepoMeta } from "./release-page";
import {
  checksumFilename,
  checksumSidecarTarget,
  findChecksumAssets,
  parseChecksumText,
} from "./parse-checksums";
import type { Asset } from "./platform-utils";
import {
  BACKEND_FETCH_REVALIDATE_SECONDS,
  BACKEND_FETCH_TIMEOUT_MS,
  BACKEND_URL,
} from "@/lib/backend-env";

export type ReleaseResult =
  | { ok: true; data: ReleaseData }
  | { ok: false; notFound: true; repoExists: boolean }
  | { ok: false; notFound?: false; message: string };

const MAX_CHECKSUM_FILE_BYTES = 1 << 20;

function backendFetch(path: string): Promise<Response> {
  return fetch(`${BACKEND_URL}${path}`, {
    next: { revalidate: BACKEND_FETCH_REVALIDATE_SECONDS },
    signal: AbortSignal.timeout(BACKEND_FETCH_TIMEOUT_MS),
  });
}

// Returns a result instead of throwing for expected/recoverable failures:
// Next.js redacts thrown Server Component error messages in production
// (replaced with a generic "digest" page), so an inline result is the only
// way to surface the specific message to the user. 404 is also returned
// inline (rather than via notFound()) so the caller still has owner/repo
// to link to the GitHub repo page.
export async function getRelease(owner: string, repo: string, version?: string): Promise<ReleaseResult> {
  const path = version
    ? `/api/release/${owner}/${repo}/${version}`
    : `/api/release/${owner}/${repo}`;
  let res: Response;
  try {
    res = await backendFetch(path);
  } catch {
    return { ok: false, message: "Couldn't reach the download service. Try again in a moment." };
  }
  if (res.status === 404) {
    // Backend sets reason: "no_releases" when the repo exists but has never
    // published a release - distinct from the repo/owner not existing at all.
    let repoExists = false;
    try {
      const body: unknown = await res.json();
      repoExists =
        typeof body === "object" && body !== null && "reason" in body && body.reason === "no_releases";
    } catch {
      // keep repoExists false
    }
    return { ok: false, notFound: true, repoExists };
  }
  if (res.status === 403) return { ok: false, message: "This repository is private or you don't have access." };
  if (res.status === 429) {
    // Distinguish per-IP HTTP throttle vs GitHub quota using the backend
    // error body; fall back to a neutral message when the body isn't usable.
    let message = "Too many requests. Try again in a minute.";
    try {
      const body: unknown = await res.json();
      if (
        typeof body === "object" &&
        body !== null &&
        "error" in body &&
        typeof body.error === "string" &&
        body.error.toLowerCase().includes("github")
      ) {
        message = "GitHub API rate limit exceeded. Try again in a minute.";
      }
    } catch {
      // keep neutral message
    }
    return { ok: false, message };
  }
  if (!res.ok) return { ok: false, message: "Couldn't reach the download service. Try again in a moment." };
  try {
    return { ok: true, data: await res.json() };
  } catch {
    return { ok: false, message: "Couldn't reach the download service. Try again in a moment." };
  }
}

// Version list for the selector/pre-release toggle - non-critical, so a
// failure here degrades to "no other versions" instead of failing the page.
export async function getReleases(owner: string, repo: string): Promise<ReleaseSummary[]> {
  try {
    const res = await backendFetch(`/api/releases/${owner}/${repo}`);
    if (!res.ok) return [];
    return await res.json();
  } catch {
    return [];
  }
}

export async function getRepoMeta(owner: string, repo: string): Promise<RepoMeta | null> {
  try {
    const res = await backendFetch(`/api/repo/${owner}/${repo}`);
    if (!res.ok) return null;
    const body: unknown = await res.json();
    if (
      typeof body === "object" &&
      body !== null &&
      "description" in body &&
      typeof body.description === "string" &&
      "avatar_url" in body &&
      typeof body.avatar_url === "string"
    ) {
      return { description: body.description, avatar_url: body.avatar_url };
    }
    return null;
  } catch {
    return null;
  }
}

// README is fetched separately so /api/release (download CTA) isn't blocked by
// a large document. Empty string on any failure — install commands / About just hide.
export async function getReadme(owner: string, repo: string): Promise<string> {
  try {
    const res = await backendFetch(`/api/readme/${owner}/${repo}`);
    if (!res.ok) return "";
    const body: unknown = await res.json();
    if (typeof body === "object" && body !== null && "readme" in body && typeof body.readme === "string") {
      return body.readme;
    }
    return "";
  } catch {
    return "";
  }
}

// Resolves release SHA256 manifests and sidecars into a filename -> hash map.
export async function getChecksums(assets: Asset[]): Promise<Record<string, string>> {
  const candidates = findChecksumAssets(assets).filter((asset) => asset.size <= MAX_CHECKSUM_FILE_BYTES);
  const assetNames = new Set(assets.map((asset) => checksumFilename(asset.name)));
  const manifests = await Promise.all(
    candidates.map(async (asset) => {
      try {
        const res = await fetch(asset.browser_download_url, {
          next: { revalidate: BACKEND_FETCH_REVALIDATE_SECONDS },
          signal: AbortSignal.timeout(BACKEND_FETCH_TIMEOUT_MS),
        });
        if (!res.ok) return {};
        return parseChecksumText(await res.text(), checksumSidecarTarget(asset.name));
      } catch {
        return {};
      }
    }),
  );

  const checksums: Record<string, string> = {};
  for (const manifest of manifests) {
    for (const [filename, hash] of Object.entries(manifest)) {
      if (assetNames.has(filename) && checksums[filename] === undefined) {
        checksums[filename] = hash;
      }
    }
  }
  return checksums;
}
