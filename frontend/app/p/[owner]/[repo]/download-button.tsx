"use client";

import { useMemo, useState, useEffect } from "react";
import {
  detectPlatform,
  detectArch,
  isSource,
  mentionsOtherPlatform,
  platformExtensions,
  platformKeywords,
  formatSize,
  type Platform,
  type Arch,
  type Asset,
} from "./platform-utils";

const platformLabels: Record<Platform, string> = {
  windows: "Windows",
  macos: "macOS",
  linux: "Linux",
};

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

function pickAssets(assets: Asset[], platform: Platform, arch: Arch): Asset[] {
  const exts = platformExtensions[platform];
  const keywords = platformKeywords[platform];
  const results: { asset: Asset; extRank: number; archRank: number }[] = [];
  for (const asset of assets) {
    const name = asset.name.toLowerCase();
    if (isSource(name)) continue;
    if (mentionsOtherPlatform(name, platform)) continue;

    let extRank = exts.findIndex((ext) => name.endsWith(ext));
    if (extRank === -1) {
      // No recognized extension (e.g. bare goreleaser binaries) - fall back to
      // a platform keyword match, ranked below any extension match.
      if (!keywords.some((kw) => name.includes(kw))) continue;
      extRank = exts.length;
    }
    results.push({ asset, extRank, archRank: archScore(name, arch) });
  }
  results.sort((a, b) => a.extRank - b.extRank || a.archRank - b.archRank);
  return results.map((r) => r.asset);
}

export function DownloadButton({
  owner,
  repo,
  assets,
  onPrimaryAsset,
}: {
  owner: string;
  repo: string;
  assets: Asset[];
  onPrimaryAsset?: (asset: Asset | null) => void;
}) {
  const [platform, setPlatform] = useState<Platform>("windows");
  const [arch, setArch] = useState<Arch>("");

  useEffect(() => {
    setPlatform(detectPlatform());
    setArch(detectArch());
  }, []);

  const primaryAssets = useMemo(
    () => pickAssets(assets, platform, arch),
    [assets, platform, arch]
  );
  const primaryAsset = primaryAssets[0] ?? null;

  useEffect(() => {
    onPrimaryAsset?.(primaryAsset);
  }, [primaryAsset, onPrimaryAsset]);

  const hasAssets = assets.length > 0;
  const primaryHref =
    primaryAsset?.browser_download_url ??
    `https://github.com/${owner}/${repo}/releases/latest`;

  return (
    <div className="flex flex-col items-center gap-2">
      <a
        href={primaryHref}
        className="inline-flex items-center gap-2 px-8 py-4 rounded-xl bg-foreground text-background font-semibold text-lg tracking-tight hover:opacity-90 active:scale-[0.98] transition-[opacity,transform] duration-150"
      >
        <DownloadIcon />
        {hasAssets ? `Download for ${platformLabels[platform]}` : "View Release on GitHub"}
      </a>

      {primaryAsset ? (
        <p className="text-xs text-muted font-mono">
          {primaryAsset.name} &middot; {formatSize(primaryAsset.size)}
        </p>
      ) : hasAssets ? (
        <p className="text-xs text-muted">
          No binary found for {platformLabels[platform]} - see all downloads below
        </p>
      ) : (
        <p className="text-xs text-muted">
          No downloads available for this release
        </p>
      )}
    </div>
  );
}

function DownloadIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path
        d="M10 3v10m0 0l-4-4m4 4l4-4M3 17h14"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
