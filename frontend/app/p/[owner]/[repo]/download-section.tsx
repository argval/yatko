"use client";

import { Suspense, use } from "react";
import { pickBestAsset, type Asset } from "./platform-utils";
import { useIsClient, usePlatform } from "./use-platform";
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
  const isClient = useIsClient();
  const [platform, arch] = usePlatform();

  if (!isClient) {
    return (
      <div className="flex flex-col items-center gap-2" aria-hidden>
        <div className="h-14 w-56 rounded-xl bg-foreground/[0.08] animate-pulse" />
        <p className="text-sm text-muted">
          {tagName} &middot; {publishedDate}
        </p>
        <div className="h-4 w-48 rounded bg-foreground/[0.06] animate-pulse" />
      </div>
    );
  }

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
        <Suspense
          fallback={
            <div className="h-4 w-48 rounded bg-foreground/[0.06] animate-pulse" aria-hidden />
          }
        >
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
