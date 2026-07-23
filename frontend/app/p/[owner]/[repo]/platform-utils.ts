// Platform detection and display helpers. Asset ranking lives in pick-asset.ts
// (mirrors backend/picker). Keep detection aligned with Go DetectPlatform /
// DetectArch / ResolveArch where the server and browser share a User-Agent.

export type { Arch, Asset, Platform } from "./pick-asset";
export {
  hasBoundedKeyword,
  isSource,
  mentionsOtherPlatform,
  pickBestAsset,
  platformExtensions,
  platformKeywords,
} from "./pick-asset";

import type { Arch, Platform } from "./pick-asset";
import { hasBoundedKeyword, platformKeywords } from "./pick-asset";

export const platformLabels: Record<Platform, string> = {
  windows: "Windows",
  macos: "macOS",
  linux: "Linux",
  android: "Android",
  ios: "iOS",
};

/** Mirrors picker.DetectPlatform, but maps unknown → windows for UI defaults. */
export function detectPlatformFromUA(userAgent: string): Platform {
  const ua = userAgent.toLowerCase();
  // Android before Linux; iPhone/iPad/iPod before macOS — same order as Go.
  if (ua.includes("windows") || ua.includes("win64") || ua.includes("win32")) return "windows";
  if (ua.includes("android")) return "android";
  if (ua.includes("iphone") || ua.includes("ipad") || ua.includes("ipod")) return "ios";
  if (ua.includes("macintosh") || ua.includes("mac os") || ua.includes("darwin")) return "macos";
  if (ua.includes("linux") || ua.includes("ubuntu") || ua.includes("fedora") || ua.includes("debian")) {
    return "linux";
  }
  return "windows";
}

/** Mirrors picker.DetectArch. */
export function detectArchFromUA(userAgent: string): Arch {
  const ua = userAgent.toLowerCase();
  if (ua.includes("arm64") || ua.includes("aarch64")) return "arm64";
  if (ua.includes("armv7") || ua.includes("armv6") || ua.includes("armhf")) return "arm";
  if (ua.includes("x86_64") || ua.includes("amd64") || ua.includes("win64")) return "amd64";
  if (ua.includes("i386") || ua.includes("i686") || ua.includes("wow64")) return "386";
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

export function assetPlatformLabel(name: string): string | null {
  const lower = name.toLowerCase();
  if (
    platformKeywords.windows.some((kw) => hasBoundedKeyword(lower, kw)) ||
    lower.endsWith(".exe") ||
    lower.endsWith(".msi")
  ) {
    return "Windows";
  }
  if (
    platformKeywords.macos.some((kw) => hasBoundedKeyword(lower, kw)) ||
    lower.endsWith(".dmg") ||
    lower.endsWith(".pkg")
  ) {
    return "macOS";
  }
  if (
    platformKeywords.android.some((kw) => hasBoundedKeyword(lower, kw)) ||
    lower.endsWith(".apk") ||
    lower.endsWith(".aab")
  ) {
    return "Android";
  }
  if (
    platformKeywords.ios.some((kw) => hasBoundedKeyword(lower, kw)) ||
    lower.endsWith(".ipa")
  ) {
    return "iOS";
  }
  if (
    platformKeywords.linux.some((kw) => hasBoundedKeyword(lower, kw)) ||
    lower.endsWith(".deb") ||
    lower.endsWith(".rpm")
  ) {
    return "Linux";
  }
  return null;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
