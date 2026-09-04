"use client";

import { useEffect, useState } from "react";
import type { Arch, Asset, Platform } from "./pick-asset";
import {
  assetFromLinkPick,
  downloadLinkPath,
  fetchLinkPick,
  type LinkPick,
} from "./link-decision";

/**
 * Authoritative download pick from the Go `/api/link` handler.
 * `undefined` while the request is in flight; `null` when the picker abstains
 * or the request fails. Does not rank on the client.
 */
export function useAuthoritativeAsset({
  owner,
  repo,
  tagName,
  assets,
  platform,
  arch,
}: {
  owner: string;
  repo: string;
  tagName: string;
  assets: Asset[];
  platform: Platform | null;
  arch: Arch | null;
}): Asset | null | undefined {
  const [pick, setPick] = useState<LinkPick | null | undefined>(undefined);

  useEffect(() => {
    if (!platform) {
      setPick(undefined);
      return;
    }
    const ac = new AbortController();
    let cancelled = false;
    setPick(undefined);
    fetchLinkPick(downloadLinkPath(owner, repo, tagName, platform, arch ?? ""), ac.signal)
      .then((got) => {
        if (!cancelled) setPick(got);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        setPick(null);
      });
    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [owner, repo, tagName, platform, arch]);

  return assetFromLinkPick(assets, pick);
}
