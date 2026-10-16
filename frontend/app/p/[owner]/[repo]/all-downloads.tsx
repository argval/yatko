"use client";

import { useState } from "react";
import { assetMatchesTarget, assetTargetLabel, platformLabels, formatSize, type Asset } from "./platform-utils";
import { usePlatform } from "./use-platform";
import { CollapsibleCard } from "./collapsible-card";
import { PlatformFilterToggle } from "./platform-filter-toggle";

const compactCount = new Intl.NumberFormat("en", {
  notation: "compact",
  maximumFractionDigits: 1,
});

export function AllDownloads({ assets }: { assets: Asset[] }) {
  const detected = usePlatform();
  const [filterEnabled, setFilterEnabled] = useState(false);

  const visible =
    filterEnabled && detected?.platform
      ? assets.filter((a) => assetMatchesTarget(a, detected))
      : assets;

  return (
    <CollapsibleCard title="All Downloads">
      <PlatformFilterToggle disabled={!detected?.platform} checked={filterEnabled} onChange={setFilterEnabled} />

      {visible.length === 0 && (
        <p className="text-sm text-foreground/40 py-2">
          {filterEnabled && detected?.platform
            ? `No assets found for ${platformLabels[detected.platform]}. Try unchecking the filter.`
            : "No downloads available for this release."}
        </p>
      )}

      <ul className="space-y-1">
        {visible.map((asset) => {
          const platformLabel = assetTargetLabel(asset);
          return (
            <li key={asset.name}>
              <a
                href={asset.browser_download_url}
                target="_blank" rel="noopener noreferrer"
                className="flex min-h-11 items-center justify-between gap-4 px-3 py-2 rounded-lg hover:bg-foreground/5 active:bg-foreground/10 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground"
              >
                <span className="grid flex-1 gap-1 min-w-0">
                  <span className="text-sm font-mono truncate">{asset.name}</span>
                  {platformLabel && (
                    <span className="w-fit max-w-full text-xs font-medium px-1.5 py-0.5 rounded-md bg-foreground/10 text-foreground/60">
                      {platformLabel}
                    </span>
                  )}
                </span>
                <span className="flex items-center gap-3 shrink-0">
                  {asset.download_count > 0 && (
                    <span className="text-xs text-foreground/30 hidden sm:inline">
                      {compactCount.format(asset.download_count)}↓
                    </span>
                  )}
                  <span className="text-xs text-foreground/40">{formatSize(asset.size)}</span>
                </span>
              </a>
            </li>
          );
        })}
      </ul>
    </CollapsibleCard>
  );
}
