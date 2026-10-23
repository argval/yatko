import { downloadRedirectPath } from "./link-decision";
import { platformLabels, type Arch, type Asset, type Platform } from "./platform-utils";
import type { VisitorContext } from "./visitor-context";

export function downloadCta({
  platform,
  arch,
  tagName,
  primaryAsset,
  hasAssets,
  owner,
  repo,
  visitorContext,
}: {
  platform: Platform;
  arch: Arch;
  tagName: string;
  /** undefined = pick still in flight; null = Go abstained. */
  primaryAsset: Asset | null | undefined;
  hasAssets: boolean;
  owner: string;
  repo: string;
  visitorContext?: VisitorContext;
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
  // While the pick is in flight, /dl still works if clicked, but the label
  // must not promise a platform build that may not exist.
  return {
    href: downloadRedirectPath(owner, repo, tagName, platform, arch, primaryAsset?.name, visitorContext),
    label: primaryAsset ? `Download for ${platformLabels[platform]}` : "Finding your download…",
    external: false,
  };
}
