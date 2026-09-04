"use client";

import { Suspense, use } from "react";
import { type Asset, type Platform } from "./platform-utils";
import { usePlatform } from "./use-platform";
import { useAuthoritativeAsset } from "./use-authoritative-asset";
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
  const detected = usePlatform();
  const primaryAsset = useAuthoritativeAsset({
    owner,
    repo,
    tagName,
    assets,
    platform: detected ? detected.platform : null,
    arch: detected ? detected.arch : null,
  });

  if (!detected || primaryAsset === undefined) {
    return (
      <div className="flex flex-col items-center gap-2">
        <div
          className="h-14 w-56 rounded-xl bg-foreground/[0.08] animate-pulse"
          aria-hidden
        />
        <p className="text-sm text-muted">
          {tagName} &middot; {publishedDate}
        </p>
        <p className="sr-only" role="status" aria-live="polite">
          {detected ? "Finding a download…" : "Detecting platform…"}
        </p>
        <div
          className="h-4 w-48 rounded bg-foreground/[0.06] animate-pulse"
          aria-hidden
        />
      </div>
    );
  }

  const { platform } = detected;

  return (
    <div className="flex flex-col items-center gap-2">
      <DownloadButton
        owner={owner}
        repo={repo}
        platform={platform}
        primaryAsset={primaryAsset}
        hasAssets={assets.length > 0}
        tagName={tagName}
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
          <AssetChecksumSlot
            checksumsPromise={checksumsPromise}
            assetName={primaryAsset.name}
            platform={platform}
          />
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
  platform,
}: {
  checksumsPromise: Promise<Record<string, string>>;
  assetName: string;
  platform: Platform;
}) {
  const checksums = use(checksumsPromise);
  return <AssetChecksum hash={checksums[assetName] ?? null} filename={assetName} platform={platform} />;
}
