"use client";

import { Suspense, use, useEffect, useState } from "react";
import { type Asset, type Platform } from "./platform-utils";
import { usePlatform } from "./use-platform";
import {
  assetFromLinkPick,
  downloadLinkPath,
  fetchLinkPick,
  pickFromReleaseTable,
  type LinkPick,
} from "./link-decision";
import { DownloadButton } from "./download-button";
import { AssetChecksum } from "./asset-checksum";

export function DownloadSection({
  owner,
  repo,
  assets,
  picks,
  tagName,
  publishedDate,
  checksumsPromise,
}: {
  owner: string;
  repo: string;
  assets: Asset[];
  picks?: Record<string, LinkPick>;
  tagName: string;
  publishedDate: string;
  checksumsPromise: Promise<Record<string, string>>;
}) {
  const detected = usePlatform();
  const platform = detected?.platform ?? null;
  const arch = detected?.arch ?? null;
  // Fallback path when /api/release predates the picks table (rolling deploy).
  const [fetchedPick, setFetchedPick] = useState<LinkPick | null | undefined>(undefined);

  const tablePick =
    platform != null ? pickFromReleaseTable(picks, platform, arch ?? "") : undefined;
  const needsFetch = tablePick === undefined && !!platform && assets.length > 0;

  useEffect(() => {
    if (!needsFetch || !platform) {
      setFetchedPick(undefined);
      return;
    }
    const ac = new AbortController();
    let cancelled = false;
    setFetchedPick(undefined);
    fetchLinkPick(downloadLinkPath(owner, repo, tagName, platform, arch ?? ""), ac.signal)
      .then((got) => {
        if (!cancelled) setFetchedPick(got);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        setFetchedPick(null);
      });
    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [needsFetch, owner, repo, tagName, platform, arch]);

  const pick = tablePick !== undefined ? tablePick : fetchedPick;
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
