// Filename classifier for release-page labels. Ranking lives in backend/picker;
// the download button points at /dl and reads the release picks table. Catalog:
// frontend/lib/picker-catalog.json (copy of shared/picker/catalog.json).

import catalogJson from "../../../../lib/picker-catalog.json";

export type Platform = "windows" | "macos" | "linux" | "android" | "ios";
export type Arch = "amd64" | "arm64" | "arm" | "386" | "";
export type Libc = "" | "musl" | "gnu" | "static";
export type ArtifactKind = "installer" | "package" | "archive" | "executable" | "";

export type ArtifactMetadata = {
  targets: { platform: string; arch?: string; libc?: string }[];
  role: "application" | "installer" | "source" | "debug" | "unknown" | "metadata";
  evidence: { source: string; field: string; value: string }[];
  status: "unknown" | "inspected" | "unsupported" | "failed";
  conflicts?: string[];
};

export type Asset = {
  id?: number;
  digest?: string;
  updated_at?: string;
  artifact?: ArtifactMetadata;
  name: string;
  browser_download_url: string;
  size: number;
  download_count: number;
};

type FormatEntry = { kind: string; platform?: string };
type Catalog = {
  platforms: Record<Platform, { aliases: string[]; extensions: string[] }>;
  architectures: Record<string, string[]>;
  unsupportedPlatforms: string[];
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

const platformKeywords: Record<Platform, string[]> = {
  windows: catalog.platforms.windows.aliases,
  macos: catalog.platforms.macos.aliases,
  linux: catalog.platforms.linux.aliases,
  android: catalog.platforms.android.aliases,
  ios: catalog.platforms.ios.aliases,
};

const archKeywords = catalog.architectures;

const displayOrder: Platform[] = ["windows", "macos", "android", "ios", "linux"];

export type ArtifactFacts = {
  original: string;
  canonical: string;
  platforms: string[];
  arches: string[];
  libc: Libc;
  kind: ArtifactKind;
  extension: string;
  variants: string[];
  source: boolean;
  nonNative: boolean;
  evidence: string[];
  formatPlatform: Platform | "";
};

function isLower(ch: string | undefined): boolean {
  return !!ch && ch >= "a" && ch <= "z";
}

type AliasHit = { start: number; end: number; key: string; kw: string };

function boundedKeywordSpans(name: string, kw: string): Array<[number, number]> {
  if (!kw) return [];
  const kwStartsWithLetter = isLower(kw[0]);
  const kwEndsWithLetter = isLower(kw[kw.length - 1]);
  const spans: Array<[number, number]> = [];
  let start = 0;
  for (;;) {
    const idx = name.indexOf(kw, start);
    if (idx === -1) return spans;
    const beforeOK = !kwStartsWithLetter || idx === 0 || !isLower(name[idx - 1]);
    const afterIdx = idx + kw.length;
    const afterOK = !kwEndsWithLetter || afterIdx === name.length || !isLower(name[afterIdx]!);
    if (beforeOK && afterOK) spans.push([idx, afterIdx]);
    start = idx + 1;
  }
}

function hasBoundedKeyword(name: string, kw: string): boolean {
  return boundedKeywordSpans(name, kw).length > 0;
}

function selectLongestHits(hits: AliasHit[]): AliasHit[] {
  const sorted = hits.slice().sort((a, b) => {
    const li = a.end - a.start;
    const lj = b.end - b.start;
    if (li !== lj) return lj - li;
    if (a.start !== b.start) return a.start - b.start;
    if (a.kw < b.kw) return -1;
    if (a.kw > b.kw) return 1;
    return 0;
  });
  const kept: AliasHit[] = [];
  for (const h of sorted) {
    if (kept.some((k) => h.start < k.end && k.start < h.end)) continue;
    kept.push(h);
  }
  return kept;
}

function archAliasHits(name: string): AliasHit[] {
  const hits: AliasHit[] = [];
  for (const [arch, kws] of Object.entries(archKeywords)) {
    for (const kw of kws) {
      for (const [start, end] of boundedKeywordSpans(name, kw)) {
        hits.push({ start, end, key: arch, kw });
      }
    }
  }
  return selectLongestHits(hits);
}

function canonicalizeName(name: string): string {
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

function sourceArchive(canonical: string): boolean {
  return catalog.source.some((token) => hasBoundedKeyword(canonical, token));
}

function isNonNative(name: string): boolean {
  return catalog.nonNative.some((kw) => hasBoundedKeyword(name, kw));
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
    source: false,
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

  for (const platform of catalog.unsupportedPlatforms) {
    if (hasBoundedKeyword(canonical, platform)) {
      facts.platforms.push(platform);
      facts.evidence = appendUnique(facts.evidence, platform);
    }
  }

  const macArm = facts.platforms.includes("macos") || /\.(dmg|pkg)$/.test(canonical);
  const hits = archAliasHits(canonical);
  for (const hit of hits) {
    // Electron uses win32 for the OS namespace of x64 and ARM64 builds too.
    if (hit.kw === "win32" && hits.length > 1) continue;
    // On Macs, bare "arm" means Apple Silicon and bare "x86" means Intel 64-bit.
    const arch = hit.kw === "arm" ? (macArm ? "arm64" : "arm") : hit.kw === "x86" && macArm ? "amd64" : hit.key;
    if (!facts.arches.includes(arch)) {
      facts.arches.push(arch);
    }
    facts.evidence = appendUnique(facts.evidence, hit.kw);
  }

  // Apple is a CPU clue only in a Mac name without an explicit architecture.
  if (macArm && !facts.arches.length && hasBoundedKeyword(canonical, "apple")) {
    facts.arches.push("arm64");
    facts.evidence = appendUnique(facts.evidence, "apple");
  }
  if (macArm && hasBoundedKeyword(canonical, "universal")) {
    facts.arches = ["amd64", "arm64"];
    facts.evidence = appendUnique(facts.evidence, "universal");
  }

  facts.source = sourceArchive(canonical);

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

function androidWithLinuxHost(facts: ArtifactFacts): boolean {
  if (!facts.platforms.includes("android") || !facts.platforms.includes("linux")) return false;
  return facts.platforms.every((p) => p === "android" || p === "linux");
}

/**
 * Display OS for "All downloads" labels. linux+android names target android;
 * exclusive extensions (exe/dmg/apk) fill in when the name has no OS token.
 */
export function primaryPlatform(facts: ArtifactFacts): Platform | "" {
  const platforms = androidWithLinuxHost(facts)
    ? facts.platforms.filter((p) => p !== "linux")
    : facts.platforms;
  if (platforms.some((p) => !Object.hasOwn(platformKeywords, p))) return "";
  if (platforms.length === 1) return platforms[0] as Platform;
  if (platforms.length === 0) return facts.formatPlatform;
  if (facts.formatPlatform && platforms.includes(facts.formatPlatform)) {
    return facts.formatPlatform;
  }
  for (const p of displayOrder) {
    if (platforms.includes(p)) return p;
  }
  return facts.formatPlatform;
}
