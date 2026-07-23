"use client";

import { platformLabels, formatSize, type Platform, type Asset } from "./platform-utils";
import { CopyButton } from "./copy-button";

export function DownloadButton({
  owner,
  repo,
  platform,
  primaryAsset,
  hasAssets,
}: {
  owner: string;
  repo: string;
  platform: Platform;
  primaryAsset: Asset | null;
  hasAssets: boolean;
}) {
  const primaryHref =
    primaryAsset?.browser_download_url ??
    `https://github.com/${owner}/${repo}/releases/latest`;

  return (
    <div className="flex flex-col items-center gap-2">
      <div className="flex items-center gap-2">
        <a
          href={primaryHref}
          {...(!primaryAsset && { target: "_blank", rel: "noopener noreferrer" })}
          className="inline-flex items-center gap-2 px-8 py-4 rounded-xl bg-foreground text-background font-semibold text-lg tracking-tight hover:opacity-90 active:scale-[0.98] transition-[opacity,transform] duration-150"
        >
          <DownloadIcon />
          {hasAssets ? `Download for ${platformLabels[platform]}` : "View Release on GitHub"}
        </a>
        <CopyButton
          text={`https://yatko.app/p/${owner}/${repo}`}
          label="Copy link to this page"
          size={20}
          className="shrink-0 p-4 rounded-xl border border-border text-foreground/50 hover:text-foreground hover:bg-foreground/5 transition-colors"
        />
      </div>

      {primaryAsset ? (
        <p className="text-xs text-muted font-mono">
          {primaryAsset.name} &middot; {formatSize(primaryAsset.size)}
        </p>
      ) : hasAssets ? (
        <p className="text-xs text-muted">
          No binary found for {platformLabels[platform]} - see all downloads below
        </p>
      ) : (
        <p className="text-xs text-muted">
          No downloads available for this release
        </p>
      )}
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
