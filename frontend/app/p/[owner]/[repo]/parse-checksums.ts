// SHA256 manifest parsing. Network fetch stays in backend.getChecksums.

import type { Asset } from "./pick-asset";

const SHA256_RE = /^[a-f0-9]{64}$/i;
const SHA256_NAME_RE = /sha[-_]?256(?:sums?)?/i;
const OTHER_HASH_NAME_RE = /(?:sha[-_]?(?:1|224|384|512)|md5)/i;
const SIDECAR_SUFFIX_RE = /\.sha256(?:sum)?(?:\.txt)?$/i;

export function checksumFilename(name: string): string {
  return name.replace(/^\.\//, "");
}

/** The target encoded by `archive.tar.gz.sha256`, if this is a sidecar. */
export function checksumSidecarTarget(name: string): string | undefined {
  const match = name.match(SIDECAR_SUFFIX_RE);
  if (!match) return undefined;
  const target = checksumFilename(name.slice(0, -match[0].length));
  return target || undefined;
}

function checksumAssetRank(name: string): number | undefined {
  if (checksumSidecarTarget(name)) return 1;
  if (SHA256_NAME_RE.test(name)) return 0;
  if (/checksums?/i.test(name) && !OTHER_HASH_NAME_RE.test(name)) return 2;
  return undefined;
}

/** True when an asset can plausibly publish SHA256 values. */
export function isChecksumAssetName(name: string): boolean {
  return checksumAssetRank(name) !== undefined;
}

/** SHA256 manifests first, then per-asset sidecars, then generic manifests. */
export function findChecksumAssets(assets: Asset[]): Asset[] {
  return assets
    .map((asset, index) => ({ asset, index, rank: checksumAssetRank(asset.name) }))
    .filter((candidate): candidate is { asset: Asset; index: number; rank: number } => candidate.rank !== undefined)
    .sort((a, b) => a.rank - b.rank || a.index - b.index)
    .map((candidate) => candidate.asset);
}

/**
 * Parse standard SHA256 manifests into filename → hash.
 * A bare hash is accepted only when a `.sha256` sidecar identifies its target.
 */
export function parseChecksumText(text: string, sidecarTarget?: string): Record<string, string> {
  const map: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const bsd = line.match(/^\s*SHA256\s*\((.+)\)\s*=\s*([a-f0-9]{64})\s*$/i);
    if (bsd) {
      map[checksumFilename(bsd[1]!)] = bsd[2]!.toLowerCase();
      continue;
    }

    const gnu = line.match(/^\s*([a-f0-9]{64})\s+\*?(.+?)\s*$/i);
    if (gnu) {
      map[checksumFilename(gnu[2]!)] = gnu[1]!.toLowerCase();
      continue;
    }

    const bare = line.trim();
    if (sidecarTarget && SHA256_RE.test(bare)) {
      map[sidecarTarget] = bare.toLowerCase();
    }
  }
  return map;
}
