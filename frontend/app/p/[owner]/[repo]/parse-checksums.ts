// Checksum Map module — pure filename→hash parsing for release checksum files.
// Network fetch stays in backend.getChecksums (thin adapter).

import type { Asset } from "./pick-asset";

const CHECKSUM_NAME_RE = /checksum|sha256sums|sha512sums|md5sums/i;

/** True when an asset name looks like a checksum manifest (not a binary). */
export function isChecksumAssetName(name: string): boolean {
  return (
    CHECKSUM_NAME_RE.test(name) ||
    name.endsWith(".sha256") ||
    name.endsWith(".sha512") ||
    name.endsWith(".md5")
  );
}

/** First matching checksum asset, if any. */
export function findChecksumAsset(assets: Asset[]): Asset | undefined {
  return assets.find((a) => isChecksumAssetName(a.name));
}

/**
 * Parse a checksum file body into filename → hash.
 * Accepts common `hash  name` / `hash *name` lines; ignores junk.
 */
export function parseChecksumText(text: string): Record<string, string> {
  const map: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const parts = line.trim().split(/\s+/);
    if (parts.length >= 2) {
      map[parts[parts.length - 1]!.replace(/^\*/, "")] = parts[0]!;
    }
  }
  return map;
}
