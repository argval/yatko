"use client";

import { CopyButton } from "./copy-button";
import { useCopy } from "./use-copy";
import { verifyChecksumCommand } from "./verify-checksum";
import type { Platform } from "./pick-asset";

export function AssetChecksum({
  hash,
  filename,
  platform,
}: {
  hash: string | null;
  filename: string;
  platform: Platform;
}) {
  const [verifyCopied, copyVerify] = useCopy();

  if (!hash) return null;

  const verifyCmd = verifyChecksumCommand(hash, filename, platform);

  return (
    <div className="flex flex-col items-center gap-1 max-w-xs sm:max-w-sm">
      <div className="flex items-center gap-2 text-xs text-foreground/40 font-mono w-full justify-center">
        <span className="truncate" title={hash}>
          SHA256: {hash.slice(0, 16)}…
        </span>
        <CopyButton
          text={hash}
          label="Copy full checksum"
          className="shrink-0 inline-flex size-6 items-center justify-center text-foreground/40 hover:text-foreground/70 transition-colors"
        />
      </div>
      <button
        type="button"
        onClick={() => copyVerify(verifyCmd)}
        className="text-xs text-foreground/40 hover:text-foreground/70 transition-colors"
        aria-label={verifyCopied ? "Copied verify command" : "Copy verify command"}
      >
        {verifyCopied ? "Copied verify command" : "Copy verify command"}
      </button>
      {verifyCopied && (
        <span className="sr-only" role="status" aria-live="polite">
          Copied
        </span>
      )}
    </div>
  );
}
