"use client";

import { useState, useCallback } from "react";
import { DownloadButton } from "./download-button";
import { AssetChecksum } from "./asset-checksum";
import type { Asset } from "./platform-utils";

export function DownloadSection({
  owner,
  repo,
  assets,
  tagName,
  publishedDate,
  checksums,
}: {
  owner: string;
  repo: string;
  assets: Asset[];
  tagName: string;
  publishedDate: string;
  checksums: Record<string, string>;
}) {
  const [selectedAsset, setSelectedAsset] = useState<Asset | null>(null);

  const handlePrimaryAsset = useCallback((asset: Asset | null) => {
    setSelectedAsset(asset);
  }, []);

  return (
    <div className="flex flex-col items-center gap-2">
      <DownloadButton
        owner={owner}
        repo={repo}
        assets={assets}
        onPrimaryAsset={handlePrimaryAsset}
      />
      <p className="text-sm text-muted">
        {tagName} &middot; {publishedDate}
      </p>
      {selectedAsset && (
        <AssetChecksum hash={checksums[selectedAsset.name] ?? null} />
      )}
    </div>
  );
}
