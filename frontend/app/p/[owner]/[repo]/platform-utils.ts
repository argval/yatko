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

const platformKeywords: Record<Platform, string[]> = {
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

export function mentionsOtherPlatform(name: string, current: Platform): boolean {
  for (const [p, keywords] of Object.entries(platformKeywords) as [Platform, string[]][]) {
    if (p === current) continue;
    if (keywords.some((kw) => name.includes(kw))) return true;
  }
  return false;
}

export function assetPlatformLabel(name: string): string | null {
  const lower = name.toLowerCase();
  if (platformKeywords.windows.some((kw) => lower.includes(kw)) || lower.endsWith(".exe") || lower.endsWith(".msi")) {
    return "Windows";
  }
  if (platformKeywords.macos.some((kw) => lower.includes(kw)) || lower.endsWith(".dmg") || lower.endsWith(".pkg")) {
    return "macOS";
  }
  if (platformKeywords.linux.some((kw) => lower.includes(kw)) || lower.endsWith(".deb") || lower.endsWith(".rpm")) {
    return "Linux";
  }
  return null;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
