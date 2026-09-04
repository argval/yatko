import type { Arch, Asset, Platform } from "./pick-asset";

export type LinkPick = {
  filename: string;
  url: string;
  size: number;
};

function pickerQuery(platform: Platform, arch: Arch): string {
  const params = new URLSearchParams({ platform });
  if (arch) params.set("arch", arch);
  return params.toString();
}

function pickerPath(
  prefix: "/api/link" | "/dl",
  owner: string,
  repo: string,
  tagName: string,
  platform: Platform,
  arch: Arch,
): string {
  return `${prefix}/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/${encodeURIComponent(tagName)}?${pickerQuery(platform, arch)}`;
}

/** Same-origin path for the Go picker JSON. Arch must be a query param — Mac ARM is often missing from the UA. */
export function downloadLinkPath(
  owner: string,
  repo: string,
  tagName: string,
  platform: Platform,
  arch: Arch,
): string {
  return pickerPath("/api/link", owner, repo, tagName, platform, arch);
}

/** Click-through URL: Go still decides the asset on request. */
export function downloadRedirectPath(
  owner: string,
  repo: string,
  tagName: string,
  platform: Platform,
  arch: Arch,
): string {
  return pickerPath("/dl", owner, repo, tagName, platform, arch);
}

export function parseLinkPick(body: unknown): LinkPick | null {
  if (typeof body !== "object" || body === null) return null;
  const rec = body as Record<string, unknown>;
  if (rec.confidence === "low") return null;
  if (typeof rec.filename !== "string" || rec.filename === "") return null;
  if (typeof rec.url !== "string" || rec.url === "") return null;
  const size = typeof rec.size === "number" && Number.isFinite(rec.size) ? rec.size : 0;
  return { filename: rec.filename, url: rec.url, size };
}

/** Map an /api/link payload onto the release's asset list. Never invents a URL. */
export function assetFromLinkPick(assets: Asset[], pick: LinkPick | null | undefined): Asset | null | undefined {
  if (pick === undefined) return undefined;
  if (pick === null) return null;
  const match = assets.find((a) => a.name === pick.filename);
  if (match) return match;
  return {
    name: pick.filename,
    browser_download_url: pick.url,
    size: pick.size,
    download_count: 0,
  };
}

export async function fetchLinkPick(path: string, signal?: AbortSignal): Promise<LinkPick | null> {
  const res = await fetch(path, { signal });
  if (!res.ok) return null;
  try {
    return parseLinkPick(await res.json());
  } catch {
    return null;
  }
}
