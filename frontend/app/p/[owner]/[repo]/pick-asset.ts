// Asset Picker browser adapter — ranking rules mirror backend/picker (Go).
// Shared golden fixtures: shared/picker/fixtures.json. Keep both in sync;
// see shared/picker/README.md.

export type Platform = "windows" | "macos" | "linux" | "android" | "ios";
export type Arch = "amd64" | "arm64" | "arm" | "386" | "";

export type Asset = {
  name: string;
  browser_download_url: string;
  size: number;
  download_count: number;
};

export const platformKeywords: Record<Platform, string[]> = {
  // Bare "win"/"mac" are safe with hasBoundedKeyword ("win" inside "darwin"
  // fails the leading-letter boundary). Needed for winX64 / -mac.jar names.
  windows: ["windows", "win32", "win64", "win-", "win"],
  macos: ["macos", "darwin", "osx", "mac-", "mac"],
  linux: ["linux", "ubuntu", "debian", "fedora", "appimage"],
  android: ["android", "apk"],
  ios: ["ios", "iphone", "ipad", "ipod"],
};

// Preferred extensions per platform, priority order (lower index wins).
export const platformExtensions: Record<Platform, string[]> = {
  windows: [".exe", ".msi", ".zip", ".jar"],
  macos: [".dmg", ".pkg", ".zip", ".tar.gz", ".jar"],
  linux: [".appimage", ".deb", ".rpm", ".tar.gz", ".tar.xz", ".zip", ".jar"],
  android: [".apk", ".aab"],
  ios: [".ipa"],
};

const archKeywords: Record<Exclude<Arch, "">, string[]> = {
  amd64: ["amd64", "x86_64", "x86-64", "x64", "win64", "intel"],
  arm64: ["arm64", "aarch64", "m1", "m2", "m3", "m4"],
  arm: ["armv7", "armv6", "armhf", "arm-"],
  "386": ["i386", "i686", "x86_32", "386", "win32"],
};

const variantKeywords = ["profile", "debug", "symbols", "dbg", "baseline", "mono", "pdb", "pdbs"];

const nonNativeKeywords = ["wasm", "wasi"];

export type Libc = "" | "musl" | "gnu" | "static";

export type PickOpts = {
  prefer?: string;
  libc?: Libc;
  /** Optional Linux deb/rpm tiebreak when prefer is unset. */
  userAgent?: string;
};

/** Normalize ?prefer= to an extension key (no leading dot). Unknown → "". */
export function resolvePrefer(param: string | undefined): string {
  let p = (param ?? "").trim().toLowerCase();
  if (p.startsWith(".")) p = p.slice(1);
  if (p === "app-image") p = "appimage";
  if (p === "tgz") p = "tar.gz";
  if (p === "txz") p = "tar.xz";
  switch (p) {
    case "exe":
    case "msi":
    case "zip":
    case "jar":
    case "dmg":
    case "pkg":
    case "appimage":
    case "deb":
    case "rpm":
    case "tar.gz":
    case "tar.xz":
    case "apk":
    case "aab":
    case "ipa":
      return p;
    default:
      return "";
  }
}

/** Normalize ?libc=. Unknown → "". */
export function resolveLibc(param: string | undefined): Libc {
  switch ((param ?? "").trim().toLowerCase()) {
    case "musl":
      return "musl";
    case "gnu":
    case "glibc":
      return "gnu";
    case "static":
      return "static";
    default:
      return "";
  }
}

/** Linux deb/rpm tiebreak from User-Agent when ?prefer= is unset. */
export function resolveLinuxPackagePrefer(userAgent: string): string {
  const ua = userAgent.toLowerCase();
  if (ua.includes("ubuntu") || ua.includes("debian")) return "deb";
  if (ua.includes("fedora") || ua.includes("rhel") || ua.includes("centos")) return "rpm";
  return "";
}

function linuxExtensionsForUA(userAgent: string): string[] {
  if (resolveLinuxPackagePrefer(userAgent) === "rpm") {
    return [".appimage", ".rpm", ".deb", ".tar.gz", ".tar.xz", ".zip", ".jar"];
  }
  return platformExtensions.linux;
}

function libcPenalty(name: string, want: Libc): number {
  const musl = hasBoundedKeyword(name, "musl");
  const gnu = hasBoundedKeyword(name, "gnu") || hasBoundedKeyword(name, "glibc");
  const staticTag = hasBoundedKeyword(name, "static");
  const tagged = musl || gnu;

  switch (want) {
    case "musl":
      if (musl || staticTag) return 0;
      if (gnu) return 2;
      return 1;
    case "gnu":
      if (gnu || staticTag) return 0;
      if (musl) return 2;
      return 1;
    case "static":
      return staticTag ? 0 : 1;
    default:
      return tagged ? 1 : 0;
  }
}

function extRankFor(name: string, exts: string[], prefer: string): number | null {
  const idx = exts.findIndex((ext) => name.endsWith(ext));
  if (idx === -1) return null;
  const key = exts[idx]!.replace(/^\./, "");
  if (prefer && prefer === key) return -1;
  return idx;
}

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

/**
 * Insert separators before Uppercase+digit after a lowercase letter
 * ("winX64" → "win-x64"), then lowercase. Digit look-ahead avoids splitting
 * AppImage → app-image. Mirrors picker.canonicalizeName.
 */
export function canonicalizeName(name: string): string {
  if (!name) return "";
  let out = "";
  for (let i = 0; i < name.length; i++) {
    const ch = name[i]!;
    const prev = i > 0 ? name[i - 1]! : "";
    const next = i + 1 < name.length ? name[i + 1]! : "";
    if (
      i > 0 &&
      ch >= "A" &&
      ch <= "Z" &&
      prev >= "a" &&
      prev <= "z" &&
      next >= "0" &&
      next <= "9"
    ) {
      out += "-";
    }
    out += ch.toLowerCase();
  }
  return out;
}

const ambiguousArchiveExts = [".tar.gz", ".tar.xz", ".tgz", ".txz", ".zip"];

function isAmbiguousArchive(name: string): boolean {
  return ambiguousArchiveExts.some((ext) => name.endsWith(ext));
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
  // Bare versioned archives (htop-3.5.2.tar.xz, v1.0.0.zip) are source dists.
  // A .zip with a platform/arch keyword is a real binary — keep it.
  if (isAmbiguousArchive(lower) && !mentionsAnyPlatform(lower) && !mentionsAnyArch(lower)) {
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

function mentionsPlatform(name: string, platform: Platform): boolean {
  return platformKeywords[platform].some((kw) => hasBoundedKeyword(name, kw));
}

function mentionsOtherArch(name: string, want: Arch): boolean {
  if (!want) return false;
  for (const arch of Object.keys(archKeywords) as Exclude<Arch, "">[]) {
    if (arch === want) continue;
    if (mentionsArch(name, arch)) return true;
  }
  return false;
}

function isNonNative(name: string): boolean {
  return nonNativeKeywords.some((kw) => hasBoundedKeyword(name, kw));
}

/** 0 = 64-bit, 1 = unspecified, 2 = 32-bit. */
function archBitWidth(name: string): number {
  if (mentionsArch(name, "amd64") || mentionsArch(name, "arm64")) return 0;
  if (mentionsArch(name, "386") || mentionsArch(name, "arm")) return 2;
  return 1;
}

function variantPenalty(name: string): number {
  let penalty = 0;
  for (const kw of variantKeywords) {
    if (hasBoundedKeyword(name, kw)) penalty++;
  }
  return penalty;
}

function candidateBetter(
  ext: number,
  family: number,
  bits: number,
  libc: number,
  variant: number,
  bestExt: number,
  bestFamily: number,
  bestBits: number,
  bestLibc: number,
  bestVariant: number,
): boolean {
  if (ext !== bestExt) return ext < bestExt;
  if (family !== bestFamily) return family < bestFamily;
  if (bits !== bestBits) return bits < bestBits;
  if (libc !== bestLibc) return libc < bestLibc;
  return variant < bestVariant;
}

/** Closer CPU family wins when an exact arch asset is missing. */
function archFamilyPenalty(name: string, want: Arch): number {
  if (!want) return 0;
  if (mentionsArch(name, want)) return 0;
  const hasAMD64 = mentionsArch(name, "amd64");
  const hasARM64 = mentionsArch(name, "arm64");
  const hasX86 = mentionsArch(name, "386");
  const hasARM = mentionsArch(name, "arm");
  switch (want) {
    case "amd64":
      if (hasX86) return 2;
      if (hasARM64) return 3;
      if (hasARM) return 4;
      break;
    case "386":
      if (hasAMD64) return 1;
      if (hasARM64) return 3;
      if (hasARM) return 4;
      break;
    case "arm64":
      if (hasARM) return 2;
      if (hasAMD64) return 3;
      if (hasX86) return 4;
      break;
    case "arm":
      if (hasARM64) return 1;
      if (hasAMD64) return 3;
      if (hasX86) return 4;
      break;
    default: {
      const _exhaustive: never = want;
      return _exhaustive;
    }
  }
  return 5;
}

/**
 * Selects the best release asset for platform/arch — same ranking as
 * picker.PickAssetForArchOpts. No keyword-only fallback when extension is missing.
 */
export function pickBestAsset(
  assets: Asset[],
  platform: Platform,
  arch: Arch,
  opts: PickOpts = {},
): Asset | null {
  if (assets.length === 0) return null;

  let exts = platformExtensions[platform];
  if (platform === "linux" && !opts.prefer && opts.userAgent) {
    exts = linuxExtensionsForUA(opts.userAgent);
  }
  const prefer = resolvePrefer(opts.prefer);
  const libcWant = resolveLibc(opts.libc);
  type Scored = {
    asset: Asset;
    extRank: number;
    archHit: boolean;
    platformHit: boolean;
    family: number;
    bitWidth: number;
    libc: number;
    variant: number;
  };
  const candidates: Scored[] = [];

  for (const asset of assets) {
    const name = canonicalizeName(asset.name);
    if (isSource(name)) continue;
    if (isNonNative(name)) continue;
    if (mentionsOtherPlatform(name, platform)) continue;

    let extRank = extRankFor(name, exts, prefer);
    // Native release binaries commonly omit an extension (e.g. herdr-macos-aarch64).
    // Only accept explicitly platform-tagged names so source files stay out.
    if (
      extRank === null &&
      mentionsPlatform(name, platform) &&
      (!arch || mentionsArch(name, arch)) &&
      !name.includes(".")
    ) {
      extRank = exts.length;
    }
    if (extRank === null) continue;

    candidates.push({
      asset,
      extRank,
      archHit: arch !== "" && mentionsArch(name, arch),
      platformHit: mentionsPlatform(name, platform),
      family: archFamilyPenalty(name, arch),
      bitWidth: archBitWidth(name),
      libc: libcPenalty(name, libcWant),
      variant: variantPenalty(name),
    });
  }

  if (candidates.length === 0) return null;

  let pool = candidates;
  if (arch) {
    const archMatches = candidates.filter((c) => c.archHit);
    if (archMatches.length > 0) {
      pool = archMatches;
    } else {
      const compatible = candidates.filter(
        (c) => !mentionsOtherArch(canonicalizeName(c.asset.name), arch),
      );
      if (compatible.length > 0) pool = compatible;
    }
  }

  const platformMatches = pool.filter((c) => c.platformHit);
  if (platformMatches.length > 0) pool = platformMatches;

  let best = pool[0]!;
  for (const c of pool.slice(1)) {
    if (
      candidateBetter(
        c.extRank,
        c.family,
        c.bitWidth,
        c.libc,
        c.variant,
        best.extRank,
        best.family,
        best.bitWidth,
        best.libc,
        best.variant,
      )
    ) {
      best = c;
    }
  }
  return best.asset;
}
