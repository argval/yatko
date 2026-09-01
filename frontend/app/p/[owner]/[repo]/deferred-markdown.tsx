"use client";

import dynamic from "next/dynamic";
import { clipMarkdown } from "./clip-markdown";

const markdownFallback = (
  <div className="space-y-2" aria-hidden>
    <div className="h-4 w-full rounded bg-foreground/[0.06] animate-pulse" />
    <div className="h-4 w-[83%] rounded bg-foreground/[0.06] animate-pulse" />
    <div className="h-4 w-[67%] rounded bg-foreground/[0.06] animate-pulse" />
  </div>
);

const RepoMarkdown = dynamic(
  () => import("./markdown").then((m) => ({ default: m.RepoMarkdown })),
  { ssr: false, loading: () => markdownFallback },
);

/**
 * Client-only markdown so ISR/SSR generations skip react-markdown + sanitize
 * on the server (the main Active CPU cost on /p pages). Robots already
 * disallow /p/, so missing markdown in the cached HTML is fine. The backend
 * bounds Markdown before it reaches this client component; this remains a
 * defensive cap for future callers.
 */
export function DeferredRepoMarkdown({
  children,
  owner,
  repo,
  refName,
  className,
}: {
  children: string;
  owner: string;
  repo: string;
  refName?: string;
  className?: string;
}) {
  return (
    <RepoMarkdown owner={owner} repo={repo} refName={refName} className={className}>
      {clipMarkdown(children)}
    </RepoMarkdown>
  );
}
