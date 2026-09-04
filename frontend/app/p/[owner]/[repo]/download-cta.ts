import { downloadRedirectPath } from "./link-decision";
import { platformLabels, type Arch, type Asset, type Platform } from "./platform-utils";

export function downloadCta({
  platform,
  arch,
  tagName,
  primaryAsset,
  hasAssets,
  owner,
  repo,
}: {
  platform: Platform;
  arch: Arch;
  tagName: string;
  /** undefined = pick still in flight; null = Go abstained. */
  primaryAsset: Asset | null | undefined;
  hasAssets: boolean;
  owner: string;
  repo: string;
}): { href: string; label: string; external: boolean } {
  if (!hasAssets) {
    return {
      href: `https://github.com/${owner}/${repo}/releases/latest`,
      label: "View Release on GitHub",
      external: true,
    };
  }
  if (primaryAsset === null) {
    return {
      href: "#downloads",
      label: "See all downloads",
      external: false,
    };
  }
  return {
    href: downloadRedirectPath(owner, repo, tagName, platform, arch),
    label: `Download for ${platformLabels[platform]}`,
    external: false,
  };
}
