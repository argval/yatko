import { platformLabels, type Platform, type Asset } from "./platform-utils";

export function downloadCta({
  platform,
  primaryAsset,
  hasAssets,
  owner,
  repo,
}: {
  platform: Platform;
  primaryAsset: Asset | null;
  hasAssets: boolean;
  owner: string;
  repo: string;
}): { href: string; label: string; external: boolean } {
  if (primaryAsset) {
    return {
      href: primaryAsset.browser_download_url,
      label: `Download for ${platformLabels[platform]}`,
      external: false,
    };
  }
  if (hasAssets) {
    return {
      href: "#downloads",
      label: "See all downloads",
      external: false,
    };
  }
  return {
    href: `https://github.com/${owner}/${repo}/releases/latest`,
    label: "View Release on GitHub",
    external: true,
  };
}
