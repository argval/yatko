export type Platform = "windows" | "macos" | "linux";
export type Arch = "amd64" | "arm64" | "";

export type Asset = {
  name: string;
  browser_download_url: string;
  size: number;
  download_count: number;
};

export function detectPlatform(): Platform {
  if (typeof navigator === "undefined") return "windows";
  const ua = navigator.userAgent.toLowerCase();
  if (ua.includes("macintosh") || ua.includes("mac os") || ua.includes("darwin")) return "macos";
  if (ua.includes("linux")) return "linux";
  return "windows";
}

export function detectArch(): Arch {
  if (typeof navigator === "undefined") return "";
  const ua = navigator.userAgent.toLowerCase();
  if (ua.includes("arm64") || ua.includes("aarch64")) return "arm64";
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
