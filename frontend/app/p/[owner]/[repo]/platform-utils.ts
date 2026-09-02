// Platform detection and display helpers. Asset ranking lives in pick-asset.ts
// (mirrors backend/picker). Keep detection aligned with Go DetectPlatform /
// DetectArch / ResolveArch where the server and browser share a User-Agent.

export type { Arch, Asset, Platform } from "./pick-asset";
export {
  canonicalizeName,
  hasBoundedKeyword,
  isSource,
  mentionsOtherPlatform,
  pickBestAsset,
  platformExtensions,
  platformKeywords,
} from "./pick-asset";

import type { Arch, Platform } from "./pick-asset";
import { canonicalizeName, hasBoundedKeyword, platformKeywords } from "./pick-asset";

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

/**
 * Safari (and many Chrome builds) still ship "Intel Mac OS X" in the UA on
 * Apple Silicon, with no arm64 token. Probe the GPU renderer — Apple GPUs mean
 * arm64; Intel/AMD/NVIDIA mean amd64. Returns "" when WebGL is unavailable.
 * Cached: detectArch is used as a useSyncExternalStore getSnapshot and must
 * stay cheap and Object.is-stable.
 */
let cachedWebGLArch: Arch | undefined;

export function detectArchFromWebGL(): Arch {
  if (cachedWebGLArch !== undefined) return cachedWebGLArch;
  if (typeof document === "undefined") {
    cachedWebGLArch = "";
    return cachedWebGLArch;
  }
  try {
    const canvas = document.createElement("canvas");
    const gl =
      canvas.getContext("webgl") ||
      canvas.getContext("experimental-webgl");
    if (!gl || typeof (gl as WebGLRenderingContext).getExtension !== "function") {
      cachedWebGLArch = "";
      return cachedWebGLArch;
    }
    const ctx = gl as WebGLRenderingContext;
    const ext = ctx.getExtension("WEBGL_debug_renderer_info");
    if (!ext) {
      cachedWebGLArch = "";
      return cachedWebGLArch;
    }
    const renderer = String(ctx.getParameter(ext.UNMASKED_RENDERER_WEBGL) ?? "").toLowerCase();
    if (!renderer) {
      cachedWebGLArch = "";
      return cachedWebGLArch;
    }
    // "Apple Software Renderer" can appear in VMs — treat as unknown.
    if (/\bapple\s+software\b/.test(renderer)) {
      cachedWebGLArch = "";
      return cachedWebGLArch;
    }
    if (/\bapple\b/.test(renderer)) {
      cachedWebGLArch = "arm64";
      return cachedWebGLArch;
    }
    if (/\b(intel|amd|nvidia|radeon|geforce|iris|quadro)\b/.test(renderer)) {
      cachedWebGLArch = "amd64";
      return cachedWebGLArch;
    }
  } catch {
    // WebGL can throw in locked-down / headless environments.
  }
  cachedWebGLArch = "";
  return cachedWebGLArch;
}

/** Test-only: clear the WebGL arch memo. */
export function resetDetectArchFromWebGLCache(): void {
  cachedWebGLArch = undefined;
}

export function detectArch(): Arch {
  if (typeof navigator === "undefined") return "";
  const fromUA = detectArchFromUA(navigator.userAgent);
  if (fromUA) return fromUA;
  const uad = (
    navigator as Navigator & { userAgentData?: { architecture?: string } }
  ).userAgentData;
  if (uad?.architecture) {
    const arch = uad.architecture.toLowerCase();
    if (arch.includes("arm")) return "arm64";
    if (arch.includes("x86") || arch.includes("amd64")) return "amd64";
  }
  // Macintosh UAs omit arch; WebGL distinguishes Apple Silicon from Intel.
  if (detectPlatformFromUA(navigator.userAgent) === "macos") {
    const fromGPU = detectArchFromWebGL();
    if (fromGPU) return fromGPU;
  }
  return "";
}

export function assetPlatformLabel(name: string): string | null {
  const lower = canonicalizeName(name);
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
