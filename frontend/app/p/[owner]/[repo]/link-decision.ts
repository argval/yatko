import type { Arch, Asset, Platform } from "./pick-asset";

export type LinkPick = {
  filename?: string;
  url?: string;
  size?: number;
  confidence?: string;
  reasons?: string[];
  status?: "selected" | "incompatible" | "unknown" | "ambiguous";
  alternatives?: { filename: string; url: string; size: number }[];
};

function pickerQuery(platform: Platform, arch: Arch): string {
  const params = new URLSearchParams({ platform });
  params.set("arch", arch || "unknown");
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

/** Pin a displayed filename while Go rechecks compatibility on click. */
export function downloadRedirectPath(
  owner: string,
  repo: string,
  tagName: string,
  platform: Platform,
  arch: Arch,
  filename?: string,
): string {
  const path = pickerPath("/dl", owner, repo, tagName, platform, arch);
  return filename ? `${path}&asset=${encodeURIComponent(filename)}` : path;
}

/** Key used by /api/release `picks` — prefer arch-specific, then platform-only. */
export function releasePickKey(platform: Platform, arch: Arch): string {
  return arch ? `${platform}/${arch}` : platform;
}

/**
 * Sync lookup against the release-page picks table.
 * Returns null when the table is present but has no auto-select for this visitor.
 */
export function pickFromReleaseTable(
  picks: Record<string, LinkPick> | undefined,
  platform: Platform,
  arch: Arch,
): LinkPick | null | undefined {
  if (!picks) return undefined;
  return picks[releasePickKey(platform, arch)] ?? null;
}

export function parseLinkPick(body: unknown): LinkPick | null {
  if (typeof body !== "object" || body === null) return null;
  const rec = body as Record<string, unknown>;
  const status = rec.status;
  if (status === "unknown" || status === "ambiguous" || status === "incompatible") {
    return {
      status,
      reasons: Array.isArray(rec.reasons) ? rec.reasons.filter((value): value is string => typeof value === "string") : [],
    };
  }
  if (status !== undefined && status !== "selected") return null;
  if (typeof rec.error === "string" && rec.error !== "") return null;
  if (rec.confidence === "low") return null;
  if (typeof rec.filename !== "string" || rec.filename === "") return null;
  if (typeof rec.url !== "string" || rec.url === "") return null;
  const size = typeof rec.size === "number" && Number.isFinite(rec.size) ? rec.size : 0;
  return {
    filename: rec.filename, url: rec.url, size,
    ...(status === "selected" ? {
      status,
      confidence: typeof rec.confidence === "string" ? rec.confidence : undefined,
      reasons: Array.isArray(rec.reasons) ? rec.reasons.filter((value): value is string => typeof value === "string") : [],
    } : {}),
  };
}

/** Map an /api/link payload onto the release's asset list. Never invents a URL. */
export function assetFromLinkPick(assets: Asset[], pick: LinkPick | null | undefined): Asset | null | undefined {
  if (pick === undefined) return undefined;
  if (pick === null || (pick.status && pick.status !== "selected") ||
      !pick.filename || !pick.url || pick.confidence === "low") return null;
  const match = assets.find((a) => a.name === pick.filename);
  if (match) return match;
  return {
    name: pick.filename,
    browser_download_url: pick.url,
    size: pick.size ?? 0,
    download_count: 0,
  };
}

export async function fetchLinkPick(path: string, signal?: AbortSignal): Promise<LinkPick | null> {
  const res = await fetch(path, { signal });
  if (!res.ok && res.status !== 404 && res.status !== 409) return null;
  try {
    const pick = parseLinkPick(await res.json());
    return res.ok || (pick?.status && pick.status !== "selected") ? pick : null;
  } catch {
    return null;
  }
}

/** Explain a server abstention without turning uncertain candidates into a recommendation. */
export function downloadDecisionMessage(decision: LinkPick | null | undefined, platformLabel: string): string {
  if (decision?.status === "ambiguous") return "Several downloads may fit. Choose a file below.";
  if (decision?.status === "unknown") return "The download target is not confirmed. Choose a file below.";
  return `No compatible download found for ${platformLabel}. See all downloads below.`;
}
