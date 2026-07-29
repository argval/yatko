"use client";

import dynamic from "next/dynamic";

const markdownFallback = (
  <div className="space-y-2" aria-hidden>
    <div className="h-4 w-full rounded bg-foreground/[0.06] animate-pulse" />
    <div className="h-4 w-[83%] rounded bg-foreground/[0.06] animate-pulse" />
    <div className="h-4 w-[67%] rounded bg-foreground/[0.06] animate-pulse" />
  </div>
);

/**
 * Client-only markdown so ISR/SSR generations skip react-markdown + sanitize
 * on the server (the main Active CPU cost on /p pages). Robots already
 * disallow /p/, so missing markdown in the cached HTML is fine.
 */
export const DeferredRepoMarkdown = dynamic(
  () => import("./markdown").then((m) => ({ default: m.RepoMarkdown })),
  { ssr: false, loading: () => markdownFallback },
);
