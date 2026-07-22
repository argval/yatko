"use client";

import { CopyButton } from "./copy-button";

export function AssetChecksum({ hash }: { hash: string | null }) {
  if (!hash) return null;

  return (
    <div className="flex items-center gap-2 text-xs text-foreground/40 font-mono max-w-xs sm:max-w-sm">
      <span className="truncate" title={hash}>
        SHA256: {hash.slice(0, 16)}…
      </span>
      <CopyButton
        text={hash}
        label="Copy full checksum"
        className="shrink-0 text-foreground/40 hover:text-foreground/70 transition-colors"
      />
    </div>
  );
}
