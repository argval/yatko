// ponytail: the filename-matching helpers below (hasBoundedKeyword,
// mentionsOtherPlatform, isSource, platformExtensions) mirror
// backend/picker/asset.go so the browser can pick/label assets without a
// round trip. Keep both in sync when changing either.

export type Platform = "windows" | "macos" | "linux";
export type Arch = "amd64" | "arm64" | "";

export type Asset = {
  name: string;
  browser_download_url: string;
  size: number;
  download_count: number;
};

export const platformLabels: Record<Platform, string> = {
  windows: "Windows",
  macos: "macOS",
  linux: "Linux",
};

export function detectPlatformFromUA(userAgent: string): Platform {
  const ua = userAgent.toLowerCase();
  if (ua.includes("macintosh") || ua.includes("mac os") || ua.includes("darwin")) return "macos";
  if (ua.includes("linux") || ua.includes("ubuntu") || ua.includes("fedora") || ua.includes("debian")) {
    return "linux";
  }
  return "windows";
}

export function detectArchFromUA(userAgent: string): Arch {
  const ua = userAgent.toLowerCase();
  if (ua.includes("arm64") || ua.includes("aarch64")) return "arm64";
  if (ua.includes("x86_64") || ua.includes("amd64") || ua.includes("win64")) return "amd64";
  return "";
}

export function detectPlatform(): Platform {
  if (typeof navigator === "undefined") return "windows";
  return detectPlatformFromUA(navigator.userAgent);
}

export function detectArch(): Arch {
  if (typeof navigator === "undefined") return "";
  const fromUA = detectArchFromUA(navigator.userAgent);
  if (fromUA) return fromUA;
  const uad = (navigator as Navigator & { userAgentData?: { architecture?: string } }).userAgentData;
  if (uad?.architecture) {
    const arch = uad.architecture.toLowerCase();
    if (arch.includes("arm")) return "arm64";
    if (arch.includes("x86") || arch.includes("amd64")) return "amd64";
  }
  return "";
}

export const platformKeywords: Record<Platform, string[]> = {
  windows: ["windows", "win32", "win64", "win-"],
  macos: ["macos", "darwin", "osx", "mac-", "apple"],
  linux: ["linux", "ubuntu", "debian", "fedora", "appimage"],
};

export const platformExtensions: Record<Platform, string[]> = {
  windows: [".exe", ".msi", ".zip"],
  macos: [".dmg", ".pkg", ".zip", ".tar.gz"],
  linux: [".appimage", ".deb", ".rpm", ".tar.gz", ".tar.xz", ".zip"],
};

export function isSource(name: string): boolean {
  return name.includes("source") || name.includes("src");
}

function isLower(ch: string | undefined): boolean {
  return !!ch && ch >= "a" && ch <= "z";
}

// hasBoundedKeyword reports whether kw occurs in name as a standalone token,
// not glued onto an adjacent letter. Keywords like "win-" are meant to match
// "app-win-x64.zip", but plain String#includes also matches "win-" inside
// "darwin-arm64" - the standard macOS release-asset naming convention - which
// silently excluded every darwin asset from platform matching. A side is only
// checked when the keyword doesn't already end/start with its own delimiter
// (e.g. "win-" already asserts its right edge via the hyphen).
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

export function mentionsOtherPlatform(name: string, current: Platform): boolean {
  for (const [p, keywords] of Object.entries(platformKeywords) as [Platform, string[]][]) {
    if (p === current) continue;
    if (keywords.some((kw) => hasBoundedKeyword(name, kw))) return true;
  }
  return false;
}

// archScore ranks how well an asset filename matches the requested arch:
// 0 = explicit match, 1 = no arch signal, 2 = explicit wrong arch.
function archScore(name: string, arch: Arch): number {
  if (!arch) return 0;
  const isArm = name.includes("arm64") || name.includes("aarch64");
  const isAmd = name.includes("amd64") || name.includes("x86_64") || name.includes("x64");
  if (arch === "arm64") {
    if (isArm) return 0;
    if (isAmd) return 2;
    return 1;
  }
  if (isAmd) return 0;
  if (isArm) return 2;
  return 1;
}

// Secondary-build markers (profiling, debug symbols, CPU-feature fallbacks).
// Prefer the vanilla asset when both exist — e.g. bun-darwin-aarch64.zip over
// bun-darwin-aarch64-profile.zip. Keep in sync with backend/picker/asset.go.
const variantKeywords = ["profile", "debug", "symbols", "dbg", "baseline"];

function variantPenalty(name: string): number {
  let penalty = 0;
  for (const kw of variantKeywords) {
    if (hasBoundedKeyword(name, kw)) penalty++;
  }
  return penalty;
}

// pickBestAsset selects the single best release asset for a platform/arch,
// mirroring the Go backend's picker.PickAssetForArch so the download button and
// the /dl redirect always resolve to the same binary. Arch is a HARD filter:
// when any asset explicitly matches the requested arch, only those are ranked by
// extension - otherwise a wrong-arch asset with a nicer extension would win.
// Returns null when nothing matches the platform.
export function pickBestAsset(assets: Asset[], platform: Platform, arch: Arch): Asset | null {
  const exts = platformExtensions[platform];
  const keywords = platformKeywords[platform];
  const results: { asset: Asset; extRank: number; archRank: number; variant: number }[] = [];
  for (const asset of assets) {
    const name = asset.name.toLowerCase();
    if (isSource(name)) continue;
    if (mentionsOtherPlatform(name, platform)) continue;

    let extRank = exts.findIndex((ext) => name.endsWith(ext));
    if (extRank === -1) {
      // No recognized extension (e.g. bare goreleaser binaries) - fall back to
      // a platform keyword match, ranked below any extension match.
      if (!keywords.some((kw) => hasBoundedKeyword(name, kw))) continue;
      extRank = exts.length;
    }
    results.push({
      asset,
      extRank,
      archRank: archScore(name, arch),
      variant: variantPenalty(name),
    });
  }
  if (results.length === 0) return null;

  // Hard arch filter first, then lowest extension rank, then vanilla (non-
  // profile/debug/baseline) builds. archRank tiebreak only matters in the
  // no-exact-match fallback, preferring arch-neutral assets over wrong-arch ones.
  const archMatches = arch ? results.filter((r) => r.archRank === 0) : [];
  const pool = archMatches.length > 0 ? archMatches : results;
  pool.sort(
    (a, b) => a.extRank - b.extRank || a.variant - b.variant || a.archRank - b.archRank,
  );
  return pool[0].asset;
}

export function assetPlatformLabel(name: string): string | null {
  const lower = name.toLowerCase();
  if (platformKeywords.windows.some((kw) => hasBoundedKeyword(lower, kw)) || lower.endsWith(".exe") || lower.endsWith(".msi")) {
    return "Windows";
  }
  if (platformKeywords.macos.some((kw) => hasBoundedKeyword(lower, kw)) || lower.endsWith(".dmg") || lower.endsWith(".pkg")) {
    return "macOS";
  }
  if (platformKeywords.linux.some((kw) => hasBoundedKeyword(lower, kw)) || lower.endsWith(".deb") || lower.endsWith(".rpm")) {
    return "Linux";
  }
  return null;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
