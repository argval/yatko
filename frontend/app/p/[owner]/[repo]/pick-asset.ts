// Asset Picker browser adapter — ranking rules mirror backend/picker (Go).
// Shared golden fixtures: shared/picker/fixtures.json. Keep both in sync;
// see shared/picker/README.md.

export type Platform = "windows" | "macos" | "linux";
export type Arch = "amd64" | "arm64" | "arm" | "386" | "";

export type Asset = {
  name: string;
  browser_download_url: string;
  size: number;
  download_count: number;
};

export const platformKeywords: Record<Platform, string[]> = {
  windows: ["windows", "win32", "win64", "win-"],
  macos: ["macos", "darwin", "osx", "mac-", "apple"],
  linux: ["linux", "ubuntu", "debian", "fedora", "appimage"],
};

// Preferred extensions per platform, priority order (lower index wins).
export const platformExtensions: Record<Platform, string[]> = {
  windows: [".exe", ".msi", ".zip"],
  macos: [".dmg", ".pkg", ".zip", ".tar.gz"],
  linux: [".appimage", ".deb", ".rpm", ".tar.gz", ".tar.xz", ".zip"],
};

const archKeywords: Record<Exclude<Arch, "">, string[]> = {
  amd64: ["amd64", "x86_64", "x86-64", "x64"],
  arm64: ["arm64", "aarch64"],
  arm: ["armv7", "armv6", "armhf", "arm-"],
  "386": ["i386", "i686", "x86_32", "386"],
};

const variantKeywords = ["profile", "debug", "symbols", "dbg", "baseline"];

function isLower(ch: string | undefined): boolean {
  return !!ch && ch >= "a" && ch <= "z";
}

/** Standalone token match — mirrors picker.hasBoundedKeyword in Go. */
export function hasBoundedKeyword(name: string, kw: string): boolean {
  if (!kw) return false;
  const kwStartsWithLetter = isLower(kw[0]);
  const kwEndsWithLetter = isLower(kw[kw.length - 1]);
  let start = 0;
  for (;;) {
    const idx = name.indexOf(kw, start);
    if (idx === -1) return false;
    const beforeOK = !kwStartsWithLetter || idx === 0 || !isLower(name[idx - 1]);
    const afterIdx = idx + kw.length;
    const afterOK = !kwEndsWithLetter || afterIdx === name.length || !isLower(name[afterIdx]);
    if (beforeOK && afterOK) return true;
    start = idx + 1;
  }
}

const ambiguousTarballExts = [".tar.gz", ".tar.xz", ".tgz", ".txz"];

function isAmbiguousTarball(name: string): boolean {
  return ambiguousTarballExts.some((ext) => name.endsWith(ext));
}

function mentionsAnyPlatform(name: string): boolean {
  for (const keywords of Object.values(platformKeywords)) {
    if (keywords.some((kw) => hasBoundedKeyword(name, kw))) return true;
  }
  return false;
}

function mentionsAnyArch(name: string): boolean {
  for (const keywords of Object.values(archKeywords)) {
    if (keywords.some((kw) => hasBoundedKeyword(name, kw))) return true;
  }
  return false;
}

export function isSource(name: string): boolean {
  const lower = name.toLowerCase();
  if (lower.includes("source") || lower.includes("src")) return true;
  // Bare versioned tarballs (htop-3.5.2.tar.xz) are source dists, not binaries.
  if (isAmbiguousTarball(lower) && !mentionsAnyPlatform(lower) && !mentionsAnyArch(lower)) {
    return true;
  }
  return false;
}

export function mentionsOtherPlatform(name: string, current: Platform): boolean {
  for (const [p, keywords] of Object.entries(platformKeywords) as [Platform, string[]][]) {
    if (p === current) continue;
    if (keywords.some((kw) => hasBoundedKeyword(name, kw))) return true;
  }
  return false;
}

function mentionsArch(name: string, arch: Arch): boolean {
  if (!arch) return false;
  const keywords = archKeywords[arch];
  return keywords.some((kw) => hasBoundedKeyword(name, kw));
}

function variantPenalty(name: string): number {
  let penalty = 0;
  for (const kw of variantKeywords) {
    if (hasBoundedKeyword(name, kw)) penalty++;
  }
  return penalty;
}

/**
 * Selects the best release asset for platform/arch — same ranking as
 * picker.PickAssetForArch. No keyword-only fallback when extension is missing.
 */
export function pickBestAsset(assets: Asset[], platform: Platform, arch: Arch): Asset | null {
  if (assets.length === 0) return null;

  const exts = platformExtensions[platform];
  type Scored = { asset: Asset; extRank: number; archHit: boolean; variant: number };
  const candidates: Scored[] = [];

  for (const asset of assets) {
    const name = asset.name.toLowerCase();
    if (isSource(name)) continue;
    if (mentionsOtherPlatform(name, platform)) continue;

    const extRank = exts.findIndex((ext) => name.endsWith(ext));
    if (extRank === -1) continue;

    candidates.push({
      asset,
      extRank,
      archHit: arch !== "" && mentionsArch(name, arch),
      variant: variantPenalty(name),
    });
  }

  if (candidates.length === 0) return null;

  let pool = candidates;
  if (arch) {
    const archMatches = candidates.filter((c) => c.archHit);
    if (archMatches.length > 0) pool = archMatches;
  }

  let best = pool[0]!;
  for (const c of pool.slice(1)) {
    if (c.extRank < best.extRank || (c.extRank === best.extRank && c.variant < best.variant)) {
      best = c;
    }
  }
  return best.asset;
}
