"use client";

import { Suspense, use } from "react";
import { usePlatform, pickBestAsset, type Asset } from "./platform-utils";
import { DownloadButton } from "./download-button";
import { AssetChecksum } from "./asset-checksum";

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
  const primaryAsset = pickBestAsset(assets, platform, arch);

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
