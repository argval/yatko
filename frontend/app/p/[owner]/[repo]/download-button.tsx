"use client";

import { platformLabels, formatSize, type Arch, type Platform, type Asset } from "./platform-utils";
import { CopyButton } from "./copy-button";
import { downloadCta } from "./download-cta";

export function DownloadButton({
  owner,
  repo,
  platform,
  arch,
  primaryAsset,
  hasAssets,
  tagName,
}: {
  owner: string;
  repo: string;
  platform: Platform;
  arch: Arch;
  primaryAsset: Asset | null | undefined;
  hasAssets: boolean;
  tagName: string;
}) {
  const { href, label, external } = downloadCta({
    platform,
    arch,
    tagName,
    primaryAsset,
    hasAssets,
    owner,
    repo,
  });

  return (
    <div className="flex flex-col items-center gap-2">
      <div className="flex items-center gap-2">
        <a
          href={href}
          {...(external && { target: "_blank", rel: "noopener noreferrer" })}
          className="inline-flex items-center gap-2 px-8 py-4 rounded-xl bg-foreground text-background font-semibold text-lg tracking-tight hover:opacity-90 active:scale-[0.98] transition-[opacity,transform] duration-150"
        >
          <DownloadIcon />
          {label}
        </a>
        <CopyButton
          text={`https://yatko.app/${owner}/${repo}`}
          label="Copy link to this page"
          size={20}
          className="shrink-0 inline-flex size-12 items-center justify-center rounded-xl border border-border text-foreground/50 hover:text-foreground hover:bg-foreground/5 transition-colors"
        />
      </div>

      {primaryAsset ? (
        <p className="text-xs text-muted font-mono">
          {primaryAsset.name} &middot; {formatSize(primaryAsset.size)}
        </p>
      ) : primaryAsset === undefined && hasAssets ? (
        <>
          <p className="h-4 w-48 rounded bg-foreground/[0.06] animate-pulse" aria-hidden />
          <p className="sr-only" role="status" aria-live="polite">
            Looking up file name…
          </p>
        </>
      ) : hasAssets ? (
        <p className="text-xs text-muted">
          No binary found for {platformLabels[platform]} – see all downloads below
        </p>
      ) : (
        <p className="text-xs text-muted">
          No downloads available for this release
        </p>
      )}
      <a
        href={`/code/${owner}/${repo}?tag=${encodeURIComponent(tagName)}`}
        className="text-sm text-muted underline underline-offset-4 hover:text-foreground transition-colors"
      >
        Download source code
      </a>
    </div>
  );
}

function DownloadIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path
        d="M10 3v10m0 0l-4-4m4 4l4-4M3 17h14"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
