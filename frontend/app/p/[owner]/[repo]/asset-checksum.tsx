"use client";

import { useCopy } from "./use-copy";

export function AssetChecksum({ hash }: { hash: string | null }) {
  const [copied, copy] = useCopy();

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
