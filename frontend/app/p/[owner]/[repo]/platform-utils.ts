// Platform detection and display helpers. Asset ranking lives in Go
// (backend/picker). The download button points at /dl as soon as platform
// is known; filename comes from the /api/release picks table. Keep UA
// detection aligned with Go DetectPlatform / DetectArch / ResolveArch.

export type { Arch, Asset, Platform } from "./pick-asset";
import { classify, primaryPlatform, type Arch, type Asset, type Libc, type Platform } from "./pick-asset";

export const platformLabels: Record<Platform, string> = {
  windows: "Windows",
  macos: "macOS",
  linux: "Linux",
  android: "Android",
  ios: "iOS",
};

/** Unknown operating systems stay unknown until the visitor selects a target. */
export function detectPlatformFromUA(userAgent: string): Platform | "" {
  const ua = userAgent.toLowerCase();
  // Android before Linux; iPhone/iPad/iPod before macOS — same order as Go.
  if (ua.includes("windows") || ua.includes("win64") || ua.includes("win32")) return "windows";
  if (ua.includes("android")) return "android";
  if (ua.includes("iphone") || ua.includes("ipad") || ua.includes("ipod")) return "ios";
  if (ua === "macos" || ua.includes("macintosh") || ua.includes("mac os") || ua.includes("darwin")) return "macos";
  if (ua.includes("linux") || ua.includes("ubuntu") || ua.includes("fedora") || ua.includes("debian")) {
    return "linux";
  }
  return "";
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

export function detectPlatform(): Platform | "" {
  if (typeof navigator === "undefined") return "";
  const hints = browserHints();
  return detectPlatformFromUA(hints?.platform ?? "") || detectPlatformFromUA(navigator.userAgent);
}

export type BrowserHints = {
  platform?: string;
  architecture?: string;
  bitness?: string;
  getHighEntropyValues?: (hints: string[]) => Promise<BrowserHints>;
};

export function browserHints(): BrowserHints | undefined {
  if (typeof navigator === "undefined") return undefined;
  return (navigator as Navigator & { userAgentData?: BrowserHints }).userAgentData;
}

/** Client hints identify the hardware even when the UA reports an emulated CPU. */
export function archFromHints(hints: BrowserHints | undefined): Arch {
  const arch = hints?.architecture?.toLowerCase();
  if (arch === "arm" || arch === "arm64" || arch === "aarch64") {
    if (arch !== "arm" || hints?.bitness === "64") return "arm64";
    return hints?.bitness === "32" ? "arm" : "";
  }
  if (arch === "x86" || arch === "x86_64" || arch === "amd64") {
    if (arch !== "x86" || hints?.bitness === "64") return "amd64";
    return hints?.bitness === "32" ? "386" : "";
  }
  return "";
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
  const fromHints = archFromHints(browserHints());
  if (fromHints) return fromHints;
  const fromUA = detectArchFromUA(navigator.userAgent);
  if (fromUA) return fromUA;
  // Macintosh UAs omit arch; WebGL distinguishes Apple Silicon from Intel.
  if (detectPlatformFromUA(navigator.userAgent) === "macos") {
    const fromGPU = detectArchFromWebGL();
    if (fromGPU) return fromGPU;
  }
  return "";
}

export function assetPlatformLabel(name: string): string | null {
  const platform = primaryPlatform(classify(name));
  return platform ? platformLabels[platform] : null;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Mirrors Go's emulation table: Windows x64 runs x86; Windows ARM64 runs x64
 *  and x86; Apple Silicon runs Intel builds via Rosetta. */
const EMULATION: Partial<Record<Platform, Partial<Record<Arch, string[]>>>> = {
  windows: { amd64: ["386"], arm64: ["amd64", "386"] },
  macos: { arm64: ["amd64"] },
};

function runsEmulated(platform: Platform | "", visitor: Arch, asset: string): boolean {
  return !!platform && !!EMULATION[platform]?.[visitor]?.includes(asset);
}

/** Server metadata is authoritative; filename facts support older API responses. */
export function assetMatchesTarget(asset: Asset, target: { platform: Platform | ""; arch: Arch; libc?: Libc }): boolean {
  if (!target.platform) return false;
  if (asset.artifact) {
    return asset.artifact.status !== "failed" &&
      !["source", "debug", "metadata"].includes(asset.artifact.role) &&
      !asset.artifact.conflicts?.length &&
      asset.artifact.targets.some((entry) => entry.platform === target.platform &&
        (!target.arch || entry.arch === target.arch || entry.arch === "universal" ||
          (!!entry.arch && runsEmulated(target.platform, target.arch, entry.arch))) &&
        (!target.libc || !entry.libc || entry.libc === target.libc || entry.libc === "static"));
  }
  const facts = classify(asset.name);
  const sidecar = /\.(sha256|sha512|md5|sig|asc|minisig|blockmap|ya?ml|json)$/.test(facts.canonical);
  const debug = /(?:^|[^a-z])(?:symbols|pdbs?|dsyms?|dbsym|debuginfo|debugsymbols|debug-symbols)(?:$|[^a-z])/.test(facts.canonical);
  return !facts.source && !facts.nonNative && !sidecar && !debug && primaryPlatform(facts) === target.platform &&
    (!target.arch || facts.arches.includes(target.arch) || facts.arches.some((arch) => runsEmulated(target.platform, target.arch, arch))) &&
    (!target.libc || !facts.libc || facts.libc === target.libc || facts.libc === "static");
}

export function assetTargetLabel(asset: Asset): string {
  if (!asset.artifact) return assetPlatformLabel(asset.name) ?? "Target unknown";
  const { targets, role, conflicts, status } = asset.artifact;
  if (conflicts?.length) return "Conflicting target evidence";
  if (role === "source") return "Source code";
  if (role === "debug") return "Debug files";
  if (role === "metadata") return "Release metadata";
  if (!targets.length) return status === "failed" ? "Target unknown · inspection failed" : "Target unknown";
  return targets.map(({ platform, arch }) => {
    const label = Object.hasOwn(platformLabels, platform) ? platformLabels[platform as Platform] : platform || "OS unknown";
    return [label, arch || "CPU unknown"].join(" · ");
  }).join(" / ");
}

/** Commands with explicit targets must match both the selected OS and CPU. */
export function installCommandMatchesTarget(command: { platform: string; platforms?: string[]; arches?: string[] }, target: { platform: Platform | ""; arch: Arch }): boolean {
  const platforms = command.platforms ?? [command.platform];
  if (!target.platform || (!platforms.includes("universal") && !platforms.includes(target.platform))) return false;
  return !command.arches?.length || (!!target.arch && command.arches.includes(target.arch));
}
