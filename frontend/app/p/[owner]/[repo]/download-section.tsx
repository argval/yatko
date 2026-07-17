"use client";

import { Suspense, use } from "react";
import {
  usePlatform,
  isSource,
  mentionsOtherPlatform,
  hasBoundedKeyword,
  platformExtensions,
  platformKeywords,
  type Platform,
  type Arch,
  type Asset,
} from "./platform-utils";
import { DownloadButton } from "./download-button";
import { AssetChecksum } from "./asset-checksum";

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
      if (!keywords.some((kw) => hasBoundedKeyword(name, kw))) continue;
      extRank = exts.length;
    }
    results.push({ asset, extRank, archRank: archScore(name, arch) });
  }
  results.sort((a, b) => a.extRank - b.extRank || a.archRank - b.archRank);
  return results.map((r) => r.asset);
}

export function DownloadSection({
  owner,
  repo,
  assets,
  tagName,
  publishedDate,
  checksumsPromise,
}: {
  owner: string;
  repo: string;
  assets: Asset[];
  tagName: string;
  publishedDate: string;
  checksumsPromise: Promise<Record<string, string>>;
}) {
  const [platform, arch] = usePlatform();
  const primaryAsset = pickAssets(assets, platform, arch)[0] ?? null;

  return (
    <div className="flex flex-col items-center gap-2">
      <DownloadButton
        owner={owner}
        repo={repo}
        platform={platform}
        primaryAsset={primaryAsset}
        hasAssets={assets.length > 0}
      />
      <p className="text-sm text-muted">
        {tagName} &middot; {publishedDate}
      </p>
      {primaryAsset && (
        <Suspense fallback={null}>
          <AssetChecksumSlot checksumsPromise={checksumsPromise} assetName={primaryAsset.name} />
        </Suspense>
      )}
    </div>
  );
}

// Isolates the use() suspend to just this row, so the button above never
// waits on checksums - only whichever asset ends up selected needs the map.
function AssetChecksumSlot({
  checksumsPromise,
  assetName,
}: {
  checksumsPromise: Promise<Record<string, string>>;
  assetName: string;
}) {
  const checksums = use(checksumsPromise);
  return <AssetChecksum hash={checksums[assetName] ?? null} />;
}
