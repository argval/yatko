"use client";

import { useState, useEffect } from "react";
import { detectPlatform, assetPlatformLabel, formatSize, type Platform, type Asset } from "./platform-utils";
import { CollapsibleCard } from "./collapsible-card";

const platformMap: Record<Platform, string> = {
  windows: "Windows",
  macos: "macOS",
  linux: "Linux",
};

function formatDownloadCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return `${n}`;
}

export function AllDownloads({ assets }: { assets: Asset[] }) {
  const [platform, setPlatform] = useState<Platform>("windows");
  const [filterEnabled, setFilterEnabled] = useState(false);

  useEffect(() => {
    setPlatform(detectPlatform());
  }, []);

  const visible = filterEnabled
    ? assets.filter((a) => {
        const label = assetPlatformLabel(a.name);
        return label === platformMap[platform];
      })
    : assets;

  return (
    <CollapsibleCard title="All Downloads">
      <label className="flex items-center gap-2 text-xs text-muted cursor-pointer select-none hover:text-foreground transition-colors mb-4">
        <input
          type="checkbox"
          checked={filterEnabled}
          onChange={(e) => setFilterEnabled(e.target.checked)}
          className="rounded border-foreground/20 accent-foreground"
        />
        My platform only
      </label>

      {visible.length === 0 && (
        <p className="text-sm text-foreground/40 py-2">
          {filterEnabled
            ? `No assets found for ${platformMap[platform]}. Try unchecking the filter.`
            : "No downloads available for this release."}
        </p>
      )}

      <ul className="space-y-1">
        {visible.map((asset) => {
          const platformLabel = assetPlatformLabel(asset.name);
          return (
            <li key={asset.name}>
              <a
                href={asset.browser_download_url}
                className="flex items-center justify-between gap-4 px-3 py-2 rounded-lg hover:bg-foreground/5 transition-colors"
              >
                <span className="flex items-center gap-2 min-w-0">
                  <span className="text-sm font-mono truncate">{asset.name}</span>
                  {platformLabel && (
                    <span className="shrink-0 text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-foreground/10 text-foreground/60">
                      {platformLabel}
                    </span>
                  )}
                </span>
                <span className="flex items-center gap-3 shrink-0">
                  {asset.download_count > 0 && (
                    <span className="text-xs text-foreground/30 hidden sm:inline">
                      {formatDownloadCount(asset.download_count)}↓
                    </span>
                  )}
                  <span className="text-xs text-foreground/40">
                    {formatSize(asset.size)}
                  </span>
                </span>
              </a>
            </li>
          );
        })}
      </ul>
    </CollapsibleCard>
  );
}
