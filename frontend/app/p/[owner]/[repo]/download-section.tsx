"use client";

import { Suspense, use, useEffect, useState } from "react";
import { type Asset, type Platform } from "./platform-utils";
import { usePlatform } from "./use-platform";
import {
  assetFromLinkPick,
  downloadLinkPath,
  fetchLinkPick,
  type LinkPick,
} from "./link-decision";
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
  const platform = detected?.platform ?? null;
  const arch = detected?.arch ?? null;
  const [pick, setPick] = useState<LinkPick | null | undefined>(undefined);

  useEffect(() => {
    if (!platform || assets.length === 0) {
      setPick(undefined);
      return;
    }
    const ac = new AbortController();
    let cancelled = false;
    setPick(undefined);
    fetchLinkPick(downloadLinkPath(owner, repo, tagName, platform, arch ?? ""), ac.signal)
      .then((got) => {
        if (!cancelled) setPick(got);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        setPick(null);
      });
    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [owner, repo, tagName, platform, arch, assets.length]);

  const primaryAsset = assetFromLinkPick(assets, pick);

  if (!detected) {
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
          Detecting platform…
        </p>
        <div
          className="h-4 w-48 rounded bg-foreground/[0.06] animate-pulse"
          aria-hidden
        />
      </div>
    );
  }

  const { platform: visitorPlatform, arch: visitorArch } = detected;

  return (
    <div className="flex flex-col items-center gap-2">
      <DownloadButton
        owner={owner}
        repo={repo}
        platform={visitorPlatform}
        arch={visitorArch ?? ""}
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
            platform={visitorPlatform}
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
