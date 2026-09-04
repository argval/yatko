// Asset Picker browser adapter — classify + rank, mirroring backend/picker.
// Shared golden fixtures: shared/picker/fixtures.json. Alias table:
// frontend/lib/picker-catalog.json (copy of shared/picker/catalog.json).

import catalogJson from "../../../../lib/picker-catalog.json";

export type Platform = "windows" | "macos" | "linux" | "android" | "ios";
export type Arch = "amd64" | "arm64" | "arm" | "386" | "";
export type Libc = "" | "musl" | "gnu" | "static";
export type ArtifactKind = "installer" | "package" | "archive" | "executable" | "";
export type Confidence = "high" | "medium" | "low";

export type Asset = {
  name: string;
  browser_download_url: string;
  size: number;
  download_count: number;
};

type FormatEntry = { kind: string; platform?: string };
type Catalog = {
  platforms: Record<Platform, { aliases: string[]; extensions: string[] }>;
  architectures: Record<Exclude<Arch, "">, string[]>;
  libc: Record<"musl" | "gnu" | "static", string[]>;
  variants: string[];
  nonNative: string[];
  source: string[];
  ambiguousArchives: string[];
  formats: Record<string, FormatEntry>;
  preferAliases: Record<string, string>;
  linuxRpmExtensions: string[];
};

const catalog = catalogJson as Catalog;

const formatKeysLongestFirst = Object.keys(catalog.formats).sort(
  (a, b) => b.length - a.length,
);

export const platformKeywords: Record<Platform, string[]> = {
  windows: catalog.platforms.windows.aliases,
  macos: catalog.platforms.macos.aliases,
  linux: catalog.platforms.linux.aliases,
  android: catalog.platforms.android.aliases,
  ios: catalog.platforms.ios.aliases,
};

export const platformExtensions: Record<Platform, string[]> = {
  windows: catalog.platforms.windows.extensions,
  macos: catalog.platforms.macos.extensions,
  linux: catalog.platforms.linux.extensions,
  android: catalog.platforms.android.extensions,
  ios: catalog.platforms.ios.extensions,
};

const archKeywords = catalog.architectures;

export type PickOpts = {
  prefer?: string;
  libc?: Libc;
  /** Optional Linux deb/rpm tiebreak when prefer is unset. */
  userAgent?: string;
};

export type ArtifactFacts = {
  original: string;
  canonical: string;
  platforms: Platform[];
  arches: Arch[];
  libc: Libc;
  kind: ArtifactKind;
  extension: string;
  variants: string[];
  source: boolean;
  nonNative: boolean;
  evidence: string[];
  formatPlatform: Platform | "";
};

export type RankedAsset = {
  asset: Asset;
  reasons: string[];
};

export type AssetDecision = {
  asset: Asset | null;
  confidence: Confidence;
  reasons: string[];
  alternatives: RankedAsset[];
  facts: ArtifactFacts | null;
  shouldAutoSelect: boolean;
};

/** Normalize ?prefer= to an extension key (no leading dot). Unknown → "". */
export function resolvePrefer(param: string | undefined): string {
  let p = (param ?? "").trim().toLowerCase();
  if (p.startsWith(".")) p = p.slice(1);
  if (catalog.preferAliases[p]) p = catalog.preferAliases[p];
  return catalog.formats[p] ? p : "";
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
    return catalog.linuxRpmExtensions;
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

function isAmbiguousArchive(name: string): boolean {
  return catalog.ambiguousArchives.some((ext) => name.endsWith(ext));
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
  return archKeywords[arch].some((kw) => hasBoundedKeyword(name, kw));
}

function mentionsPlatform(name: string, platform: Platform): boolean {
  return platformKeywords[platform].some((kw) => hasBoundedKeyword(name, kw));
}

function mentionsAnyPlatform(name: string): boolean {
  return (Object.keys(platformKeywords) as Platform[]).some((p) => mentionsPlatform(name, p));
}

function mentionsAnyArch(name: string): boolean {
  return (Object.keys(archKeywords) as Exclude<Arch, "">[]).some((a) => mentionsArch(name, a));
}

export function isSource(name: string): boolean {
  const lower = name.toLowerCase();
  if (catalog.source.some((tok) => lower.includes(tok))) return true;
  if (isAmbiguousArchive(lower) && !mentionsAnyPlatform(lower) && !mentionsAnyArch(lower)) {
    return true;
  }
  return false;
}

function isNonNative(name: string): boolean {
  return catalog.nonNative.some((kw) => hasBoundedKeyword(name, kw));
}

function archBitWidth(name: string): number {
  if (mentionsArch(name, "amd64") || mentionsArch(name, "arm64")) return 0;
  if (mentionsArch(name, "386") || mentionsArch(name, "arm")) return 2;
  return 1;
}

function variantPenalty(name: string): number {
  let penalty = 0;
  for (const kw of catalog.variants) {
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

function candidateEqual(
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
  return (
    ext === bestExt &&
    family === bestFamily &&
    bits === bestBits &&
    libc === bestLibc &&
    variant === bestVariant
  );
}

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

function matchFormat(canonical: string): { key: string; entry: FormatEntry } | null {
  for (const key of formatKeysLongestFirst) {
    if (canonical.endsWith(`.${key}`)) {
      return { key, entry: catalog.formats[key]! };
    }
  }
  return null;
}

function appendUnique(dst: string[], v: string): string[] {
  return dst.includes(v) ? dst : [...dst, v];
}

/** Structured facts from a filename. Unknown tokens stay absent. */
export function classify(name: string): ArtifactFacts {
  const canonical = canonicalizeName(name);
  const facts: ArtifactFacts = {
    original: name,
    canonical,
    platforms: [],
    arches: [],
    libc: "",
    kind: "",
    extension: "",
    variants: [],
    source: isSource(canonical),
    nonNative: isNonNative(canonical),
    evidence: [],
    formatPlatform: "",
  };

  for (const p of Object.keys(platformKeywords) as Platform[]) {
    for (const kw of platformKeywords[p]) {
      if (hasBoundedKeyword(canonical, kw)) {
        facts.platforms.push(p);
        facts.evidence = appendUnique(facts.evidence, kw);
        break;
      }
    }
  }

  for (const a of Object.keys(archKeywords) as Exclude<Arch, "">[]) {
    for (const kw of archKeywords[a]) {
      if (hasBoundedKeyword(canonical, kw)) {
        facts.arches.push(a);
        facts.evidence = appendUnique(facts.evidence, kw);
        break;
      }
    }
  }

  const fmt = matchFormat(canonical);
  if (fmt) {
    facts.extension = fmt.key;
    facts.kind = (fmt.entry.kind as ArtifactKind) || "";
    if (fmt.entry.platform) facts.formatPlatform = fmt.entry.platform as Platform;
    facts.evidence = appendUnique(facts.evidence, `.${fmt.key}`);
  }

  for (const [libc, kws] of Object.entries(catalog.libc) as ["musl" | "gnu" | "static", string[]][]) {
    for (const kw of kws) {
      if (hasBoundedKeyword(canonical, kw)) {
        if (libc === "musl") facts.libc = "musl";
        else if (facts.libc === "") facts.libc = libc;
        facts.evidence = appendUnique(facts.evidence, kw);
        break;
      }
    }
  }

  for (const kw of catalog.variants) {
    if (hasBoundedKeyword(canonical, kw)) {
      facts.variants.push(kw);
      facts.evidence = appendUnique(facts.evidence, kw);
    }
  }

  return facts;
}

function hasPlatform(facts: ArtifactFacts, p: Platform): boolean {
  return facts.platforms.includes(p);
}

function hasArch(facts: ArtifactFacts, a: Arch): boolean {
  return a !== "" && facts.arches.includes(a);
}

function hasOtherPlatform(facts: ArtifactFacts, p: Platform): boolean {
  return facts.platforms.some((got) => got !== p);
}

function hasOtherArch(facts: ArtifactFacts, want: Arch): boolean {
  if (!want) return false;
  return facts.arches.some((got) => got !== want);
}

function formatIsGeneric(ext: string): boolean {
  const entry = catalog.formats[ext];
  if (!entry) return true;
  return !entry.platform;
}

type Scored = {
  asset: Asset;
  facts: ArtifactFacts;
  extRank: number;
  archHit: boolean;
  platformHit: boolean;
  family: number;
  bitWidth: number;
  libc: number;
  variant: number;
};

function reasonsFor(c: Scored, platform: Platform, arch: Arch, tied: boolean): string[] {
  const reasons: string[] = [];
  if (c.platformHit) reasons.push(`platform token matches ${platform}`);
  if (c.archHit) reasons.push(`arch token matches ${arch}`);
  if (c.facts.formatPlatform === platform && c.facts.extension) {
    reasons.push(`extension .${c.facts.extension} implies ${platform}`);
  } else if (c.facts.extension) {
    reasons.push(`extension .${c.facts.extension}`);
  }
  if (c.family > 0 && arch) reasons.push(`cpu family fallback for ${arch}`);
  if (c.variant > 0) reasons.push("secondary build variant");
  if (tied) reasons.push("tied with another candidate");
  if (reasons.length === 0) reasons.push(`extension matched ${platform} defaults`);
  return reasons;
}

function confidenceFor(
  best: Scored,
  platform: Platform,
  arch: Arch,
  tied: boolean,
  candidateCount: number,
): Confidence {
  const formatExclusive = best.facts.formatPlatform === platform && best.facts.extension !== "";
  const generic = formatIsGeneric(best.facts.extension);
  const platformEvidence = best.platformHit || formatExclusive;
  const archEvidence = !arch || best.archHit;

  if (!platformEvidence && generic && !best.archHit) return "low";
  if ((best.platformHit || formatExclusive) && archEvidence && !tied) return "high";
  if (formatExclusive && !tied) return "high";
  if (tied && candidateCount > 1) return "medium";
  if (best.platformHit || formatExclusive || best.archHit) return "medium";
  return "low";
}

function emptyDecision(reasons: string[]): AssetDecision {
  return {
    asset: null,
    confidence: "low",
    reasons,
    alternatives: [],
    facts: null,
    shouldAutoSelect: false,
  };
}

/**
 * Classify, rank, and decide — same rules as picker.DecideAsset.
 * shouldAutoSelect is false when confidence is low (abstain).
 */
export function decideBestAsset(
  assets: Asset[],
  platform: Platform,
  arch: Arch,
  opts: PickOpts = {},
): AssetDecision {
  if (assets.length === 0) return emptyDecision(["no release assets"]);

  let exts = platformExtensions[platform];
  if (platform === "linux" && !opts.prefer && opts.userAgent) {
    exts = linuxExtensionsForUA(opts.userAgent);
  }
  const prefer = resolvePrefer(opts.prefer);
  const libcWant = resolveLibc(opts.libc);
  const candidates: Scored[] = [];

  for (const asset of assets) {
    const facts = classify(asset.name);
    if (facts.source || facts.nonNative) continue;
    if (hasOtherPlatform(facts, platform)) continue;

    let extRank = extRankFor(facts.canonical, exts, prefer);
    if (
      extRank === null &&
      hasPlatform(facts, platform) &&
      (!arch || hasArch(facts, arch)) &&
      !facts.canonical.includes(".")
    ) {
      extRank = exts.length;
    }
    if (extRank === null) continue;

    candidates.push({
      asset,
      facts,
      extRank,
      archHit: arch !== "" && hasArch(facts, arch),
      platformHit: hasPlatform(facts, platform),
      family: archFamilyPenalty(facts.canonical, arch),
      bitWidth: archBitWidth(facts.canonical),
      libc: libcPenalty(facts.canonical, libcWant),
      variant: variantPenalty(facts.canonical),
    });
  }

  if (candidates.length === 0) return emptyDecision(["no matching installable asset"]);

  let pool = candidates;
  if (arch) {
    const archMatches = candidates.filter((c) => c.archHit);
    if (archMatches.length > 0) {
      pool = archMatches;
    } else {
      const compatible = candidates.filter((c) => !hasOtherArch(c.facts, arch));
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

  let tied = false;
  const alternatives: RankedAsset[] = [];
  for (const c of pool) {
    if (c.asset.name === best.asset.name) continue;
    if (
      candidateEqual(
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
      tied = true;
    }
    if (alternatives.length < 3) {
      alternatives.push({ asset: c.asset, reasons: reasonsFor(c, platform, arch, false) });
    }
  }

  const confidence = confidenceFor(best, platform, arch, tied, pool.length);
  return {
    asset: best.asset,
    confidence,
    reasons: reasonsFor(best, platform, arch, tied),
    alternatives,
    facts: best.facts,
    shouldAutoSelect: confidence !== "low",
  };
}

/**
 * Auto-selected asset only — same as picker.PickAssetForArchOpts.
 * Low-confidence guesses return null so the CTA can abstain.
 */
export function pickBestAsset(
  assets: Asset[],
  platform: Platform,
  arch: Arch,
  opts: PickOpts = {},
): Asset | null {
  const d = decideBestAsset(assets, platform, arch, opts);
  return d.shouldAutoSelect ? d.asset : null;
}
