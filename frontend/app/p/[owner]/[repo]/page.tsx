import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ReleasePageBody, type ReleaseData, type ReleaseSummary } from "./release-page";
import type { Asset } from "./platform-utils";

const BACKEND_URL = process.env.BACKEND_URL || "http://localhost:8080";
const CHECKSUM_RE = /checksum|sha256sums|sha512sums|md5sums/i;

type Props = {
  params: Promise<{ owner: string; repo: string }>;
};

export async function getRelease(owner: string, repo: string, version?: string): Promise<ReleaseData> {
  const path = version
    ? `/api/release/${owner}/${repo}/${version}`
    : `/api/release/${owner}/${repo}`;
  let res: Response;
  try {
    res = await fetch(`${BACKEND_URL}${path}`, { next: { revalidate: 300 } });
  } catch {
    throw new Error("Couldn't reach the download service. Try again in a moment.");
  }
  if (res.status === 404) notFound();
  if (res.status === 403) throw new Error("This repository is private or you don't have access.");
  if (res.status === 429) throw new Error("GitHub API rate limit exceeded. Try again in a minute.");
  if (!res.ok) throw new Error("Couldn't reach the download service. Try again in a moment.");
  return res.json();
}

// Version list for the selector/pre-release toggle - non-critical, so a
// failure here degrades to "no other versions" instead of failing the page.
export async function getReleases(owner: string, repo: string): Promise<ReleaseSummary[]> {
  try {
    const res = await fetch(`${BACKEND_URL}/api/releases/${owner}/${repo}`, { next: { revalidate: 300 } });
    if (!res.ok) return [];
    return await res.json();
  } catch {
    return [];
  }
}

// Resolves the release's checksum file (if any) into a filename -> hash map,
// so the client can do a plain lookup instead of fetching it itself.
export async function getChecksums(assets: Asset[]): Promise<Record<string, string>> {
  const checksumAsset = assets.find(
    (a) =>
      CHECKSUM_RE.test(a.name) ||
      a.name.endsWith(".sha256") ||
      a.name.endsWith(".sha512") ||
      a.name.endsWith(".md5")
  );
  if (!checksumAsset) return {};

  try {
    const res = await fetch(checksumAsset.browser_download_url, { next: { revalidate: 300 } });
    if (!res.ok) return {};
    const map: Record<string, string> = {};
    for (const line of (await res.text()).split("\n")) {
      const parts = line.trim().split(/\s+/);
      if (parts.length >= 2) {
        map[parts[parts.length - 1].replace(/^\*/, "")] = parts[0];
      }
    }
    return map;
  } catch {
    return {};
  }
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { owner, repo } = await params;
  return {
    title: `${repo} - Download | Yoink`,
    description: `Download the latest release of ${owner}/${repo}`,
  };
}

export default async function ReleasePage({ params }: Props) {
  const { owner, repo } = await params;
  const release = await getRelease(owner, repo);
  const [releases, checksums] = await Promise.all([
    getReleases(owner, repo),
    getChecksums(release.assets),
  ]);
  return (
    <ReleasePageBody owner={owner} repo={repo} release={release} releases={releases} checksums={checksums} />
  );
}
