"use client";

import { useState, useEffect } from "react";
import { useCopy } from "./use-copy";
import type { Asset } from "./platform-utils";

const CHECKSUM_RE = /checksum|sha256sums|sha512sums|md5sums/i;

function findChecksumAsset(assets: Asset[]): Asset | null {
  return (
    assets.find(
      (a) =>
        CHECKSUM_RE.test(a.name) ||
        a.name.endsWith(".sha256") ||
        a.name.endsWith(".sha512") ||
        a.name.endsWith(".md5")
    ) ?? null
  );
}

function parseChecksumFile(content: string, filename: string): string | null {
  for (const line of content.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const parts = trimmed.split(/\s+/);
    if (parts.length >= 2) {
      const lineFile = parts[parts.length - 1].replace(/^\*/, "");
      if (lineFile === filename) return parts[0];
    }
  }
  return null;
}

export function AssetChecksum({
  assets,
  primaryAsset,
}: {
  assets: Asset[];
  primaryAsset: Asset;
}) {
  const checksumAsset = findChecksumAsset(assets);
  const [hash, setHash] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [copied, copy] = useCopy();

  useEffect(() => {
    if (!checksumAsset) return;
    setLoading(true);
    setHash(null);
    fetch(checksumAsset.browser_download_url)
      .then((r) => r.text())
      .then((text) => {
        setHash(parseChecksumFile(text, primaryAsset.name));
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [checksumAsset?.browser_download_url, primaryAsset.name]);

  if (!checksumAsset) return null;
  if (loading) return <p className="text-xs text-foreground/30">Verifying checksum…</p>;
  if (!hash) return null;

  return (
    <div className="flex items-center gap-2 text-xs text-foreground/40 font-mono max-w-xs sm:max-w-sm">
      <span className="truncate" title={hash}>
        SHA256: {hash.slice(0, 16)}…
      </span>
      <button
        type="button"
        onClick={() => copy(hash)}
        className="shrink-0 hover:text-foreground/70 transition-colors"
        title="Copy full checksum"
      >
        {copied ? "✓" : "Copy"}
      </button>
    </div>
  );
}
