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
import { TargetSelector } from "./target-selector";
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
  // Jev decisions need this visitor's context; rule-only responses may include a table.
  const [fetchedPick, setFetchedPick] = useState<{ path: string; pick: LinkPick | null } | null>(null);

  const tablePick =
    platform ? pickFromReleaseTable(picks, platform, arch ?? "") : undefined;
  const fetchPath = tablePick === undefined && platform && !detected?.pending && assets.length > 0
    ? downloadLinkPath(owner, repo, tagName, platform, arch ?? "", detected?.context) : null;

  useEffect(() => {
    if (!fetchPath) return;
    const ac = new AbortController();
    let cancelled = false;
    fetchLinkPick(fetchPath, ac.signal)
      .then((got) => {
        if (!cancelled) setFetchedPick({ path: fetchPath, pick: got });
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        setFetchedPick({ path: fetchPath, pick: null });
      });
    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [fetchPath]);

  const pick = tablePick !== undefined ? tablePick :
    fetchedPick?.path === fetchPath ? fetchedPick?.pick : undefined;
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
  // Open the device controls only when they can help: no OS was detected, or
  // the CPU is unknown and the picker could not choose between CPU builds.
  const needsChoice = !visitorPlatform ||
    (!visitorArch && (pick?.status === "ambiguous" || pick?.status === "unknown"));

  return (
    <div className="flex flex-col items-center gap-2">
      {visitorPlatform ? <DownloadButton
        owner={owner}
        repo={repo}
        platform={visitorPlatform}
        arch={visitorArch ?? ""}
        primaryAsset={primaryAsset}
        hasAssets={assets.length > 0}
        tagName={tagName}
        decision={pick}
        visitorContext={detected.context}
      /> : <>
        <a href="#downloads" className="inline-flex min-h-11 items-center rounded-xl bg-foreground px-6 py-3 font-semibold text-background focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground active:opacity-90">Browse downloads</a>
        <p className="text-sm text-muted">Select an operating system to find a download.</p>
      </>}
      <TargetSelector target={detected} needsChoice={needsChoice} />
      <p className="text-sm text-muted">
        {tagName} &middot; {publishedDate}
      </p>
      {primaryAsset && visitorPlatform && (
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
