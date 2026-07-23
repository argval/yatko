"use client";

import dynamic from "next/dynamic";
import { useEffect, useRef, useState } from "react";
import type { Asset, Platform } from "./platform-utils";

const sectionFallback = (
  <div className="h-14 rounded-xl border border-border bg-surface/40 animate-pulse" aria-hidden />
);

const AllDownloads = dynamic(
  () => import("./all-downloads").then((m) => ({ default: m.AllDownloads })),
  { loading: () => sectionFallback },
);

const ShareLinks = dynamic(
  () => import("./share-links").then((m) => ({ default: m.ShareLinks })),
  { loading: () => sectionFallback },
);

/** Defers importing/hydrating below-fold client islands until near the viewport. */
export function BelowFoldSections({
  owner,
  repo,
  assets,
  initialPlatform,
}: {
  owner: string;
  repo: string;
  assets: Asset[];
  initialPlatform: Platform;
}) {
  const [ready, setReady] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) {
          setReady(true);
          io.disconnect();
        }
      },
      { rootMargin: "240px 0px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  return (
    <div ref={ref} id="downloads" className="space-y-8 scroll-mt-8">
      {ready ? (
        <>
          <AllDownloads assets={assets} initialPlatform={initialPlatform} />
          <ShareLinks owner={owner} repo={repo} />
        </>
      ) : (
        <>
          {sectionFallback}
          {sectionFallback}
        </>
      )}
    </div>
  );
}
