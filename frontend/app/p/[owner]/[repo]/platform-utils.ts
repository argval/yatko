// Platform detection, asset types and display helpers. The backend decides
// downloads (Jev, rule picker fallback) and attaches `artifact` facts to every
// asset; the browser only filters and labels with them. Keep UA detection
// aligned with Go DetectPlatform / DetectArch / ResolveArch.

export type Platform = "windows" | "macos" | "linux" | "android" | "ios";
export type Arch = "amd64" | "arm64" | "arm" | "386" | "";
export type Libc = "" | "musl" | "gnu" | "static";
export type ArtifactMetadata = {
  targets: { platform: string; arch?: string; libc?: string }[];
  role: "application" | "installer" | "source" | "debug" | "unknown" | "metadata";
  evidence: { source: string; field: string; value: string }[];
  status: "unknown" | "inspected" | "unsupported" | "failed";
  conflicts?: string[];
  targets_partial?: boolean;
  launcher_arch?: string;
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
  if (
    ua.includes("windows") ||
    ua.includes("win64") ||
    ua.includes("win32") ||
    ua.includes("mingw") ||
    ua.includes("msys") ||
    ua.includes("cygwin")
  ) {
    return "windows";
  }
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
  if (ua.includes("armv7") || ua.includes("armv6") || ua.includes("armhf") || /(^|[^a-z])arm([^a-z]|$)/.test(ua)) return "arm";
  if (ua.includes("x86_64") || ua.includes("amd64") || ua.includes("win64") || ua.includes("x64")) {
    return "amd64";
  }
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
  platformVersion?: string;
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

/** Server `artifact` facts decide compatibility; an asset without them matches nothing. */
export function assetMatchesTarget(asset: Asset, target: { platform: Platform | ""; arch: Arch; libc?: Libc }): boolean {
  const artifact = asset.artifact;
  if (!target.platform || !artifact) return false;
  if (/\.appimage$/i.test(asset.name) && (!artifact.launcher_arch ||
    (target.arch && artifact.launcher_arch !== target.arch && !runsEmulated(target.platform, target.arch, artifact.launcher_arch)))) return false;
  return artifact.status !== "failed" &&
    !["source", "debug", "metadata"].includes(artifact.role) &&
    !artifact.conflicts?.length &&
    artifact.targets.some((entry) => entry.platform === target.platform &&
      (artifact.targets_partial || !target.arch || entry.arch === target.arch || entry.arch === "universal" ||
        (!!entry.arch && runsEmulated(target.platform, target.arch, entry.arch))) &&
      (!target.libc || !entry.libc || entry.libc === target.libc || entry.libc === "static"));
}

export function assetTargetLabel(asset: Asset): string {
  if (!asset.artifact) return "Target unknown";
  const { targets, role, conflicts, status, targets_partial } = asset.artifact;
  if (conflicts?.length) return "Conflicting target evidence";
  if (role === "source") return "Source code";
  if (role === "debug") return "Debug files";
  if (role === "metadata") return "Release metadata";
  if (!targets.length) return status === "failed" ? "Target unknown · inspection failed" : "Target unknown";
  return targets.map(({ platform, arch }) => {
    const label = Object.hasOwn(platformLabels, platform) ? platformLabels[platform as Platform] : platform || "OS unknown";
    return [label, arch || "CPU unknown"].join(" · ");
  }).join(" / ") + (targets_partial ? " · additional targets unverified" : "");
}

/** Commands with explicit targets must match both the selected OS and CPU. */
export function installCommandMatchesTarget(command: { platform: string; platforms?: string[]; arches?: string[] }, target: { platform: Platform | ""; arch: Arch }): boolean {
  const platforms = command.platforms ?? [command.platform];
  if (!target.platform || (!platforms.includes("universal") && !platforms.includes(target.platform))) return false;
  return !command.arches?.length || (!!target.arch && command.arches.includes(target.arch));
}
