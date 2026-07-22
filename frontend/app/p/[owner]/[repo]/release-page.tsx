import { Suspense } from "react";
import Image from "next/image";
import Link from "next/link";
import { InstallCommands } from "./install-commands";
import { DownloadSection } from "./download-section";
import { VersionSelector } from "./version-selector";
import { PrereleaseToggle } from "./prerelease-toggle";
import { BelowFoldSections } from "./below-fold";
import { CollapsibleCard } from "./collapsible-card";
import { ReleaseNavShell } from "./release-nav-shell";
import { RepoMarkdown, RepoDescription } from "./markdown";
import { extractInstallCommands } from "./extract-install-commands";
import { chromeTextButton } from "@/components/chrome";
import type { Arch, Asset, Platform } from "./platform-utils";

export type ReleaseData = {
  owner: string;
  repo: string;
  description: string;
  avatar_url: string;
  tag_name: string;
  name: string;
  body: string;
  published_at: string;
  html_url: string;
  prerelease: boolean;
  assets: Asset[];
  /** Embedded by /api/release so the page skips a second round-trip. */
  releases?: ReleaseSummary[];
};

export type ReleaseSummary = {
  tag_name: string;
  name: string;
  published_at: string;
  prerelease: boolean;
};

export function ReleasePageBody({
  owner,
  repo,
  release,
  initialPlatform,
  initialArch,
  readmePromise,
  releasesPromise,
  checksumsPromise,
}: {
  owner: string;
  repo: string;
  release: ReleaseData;
  initialPlatform: Platform;
  initialArch: Arch;
  readmePromise: Promise<string>;
  releasesPromise: Promise<ReleaseSummary[]>;
  checksumsPromise: Promise<Record<string, string>>;
}) {
  const publishedDate = new Date(release.published_at).toLocaleDateString("en-US", {
    year: "numeric",
    month: "long",
    day: "numeric",
  });

  return (
    <ReleaseNavShell>
      <Link
        href="/"
        className={`fixed top-4 left-4 z-50 ${chromeTextButton}`}
        aria-label="Back to Yatko homepage"
      >
        <BackIcon />
        Yatko
      </Link>
      <main className="flex-1 flex flex-col items-center px-4 py-12 sm:py-20">
        <div className="w-full max-w-2xl space-y-8">
          {/* Header */}
          <div className="text-center space-y-4">
            <Image
              src={`https://github.com/${owner}.png?size=96`}
              alt={`${owner} avatar`}
              width={64}
              height={64}
              unoptimized
              className="rounded-2xl mx-auto"
            />
            <h1 className="text-4xl sm:text-5xl font-semibold tracking-tighter leading-none">{repo}</h1>
            <p className="text-muted text-base sm:text-lg">
              <a
                href={`https://github.com/${owner}/${repo}`}
                target="_blank"
                rel="noopener noreferrer"
                className="hover:text-foreground transition-colors"
              >
                {owner}/{repo}
              </a>
            </p>
            {release.prerelease && (
              <span className="inline-block text-xs font-medium px-2.5 py-1 rounded-full bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20">
                Pre-release
              </span>
            )}
          </div>

          {/* Download section */}
          <div className="flex flex-col items-center gap-3">
            {release.description && <RepoDescription>{release.description}</RepoDescription>}
            <DownloadSection
              owner={owner}
              repo={repo}
              assets={release.assets}
              tagName={release.tag_name}
              publishedDate={publishedDate}
              initialPlatform={initialPlatform}
              initialArch={initialArch}
              checksumsPromise={checksumsPromise}
            />
            <Suspense
              fallback={
                <div className="flex flex-col items-center gap-2 w-full max-w-xs">
                  <div className="h-9 w-36 rounded-lg bg-foreground/[0.06] animate-pulse" />
                  <div className="h-4 w-40 rounded bg-foreground/[0.04] animate-pulse" />
                </div>
              }
            >
              <ReleaseVersionControls
                owner={owner}
                repo={repo}
                currentTag={release.tag_name}
                prerelease={release.prerelease}
                releasesPromise={releasesPromise}
              />
            </Suspense>
          </div>

          {/* Install commands + About — stream in with the README */}
          <Suspense
            fallback={
              <div className="space-y-3">
                <div className="h-14 rounded-xl border border-border bg-surface/40 animate-pulse" />
              </div>
            }
          >
            <ReadmeSections
              owner={owner}
              repo={repo}
              readmePromise={readmePromise}
              initialPlatform={initialPlatform}
            />
          </Suspense>

          {/* Release notes */}
          {release.body && (
            <CollapsibleCard title="Release Notes">
              <RepoMarkdown owner={owner} repo={repo} refName={release.tag_name}>
                {release.body}
              </RepoMarkdown>
            </CollapsibleCard>
          )}

          {/* Below-fold client islands — code-split + mount when near viewport */}
          <BelowFoldSections
            owner={owner}
            repo={repo}
            assets={release.assets}
            initialPlatform={initialPlatform}
          />
        </div>
      </main>
    </ReleaseNavShell>
  );
}

async function ReleaseVersionControls({
  owner,
  repo,
  currentTag,
  prerelease,
  releasesPromise,
}: {
  owner: string;
  repo: string;
  currentTag: string;
  prerelease: boolean;
  releasesPromise: Promise<ReleaseSummary[]>;
}) {
  const releases = await releasesPromise;
  return (
    <>
      <VersionSelector owner={owner} repo={repo} currentTag={currentTag} showPrereleases={prerelease} releases={releases} />
      <PrereleaseToggle owner={owner} repo={repo} isCurrentPrerelease={prerelease} releases={releases} />
    </>
  );
}

async function ReadmeSections({
  owner,
  repo,
  readmePromise,
  initialPlatform,
}: {
  owner: string;
  repo: string;
  readmePromise: Promise<string>;
  initialPlatform: Platform;
}) {
  const readme = await readmePromise;
  if (!readme) return null;
  const installCommands = extractInstallCommands(readme);
  return (
    <>
      {installCommands.length > 0 && (
        <InstallCommands commands={installCommands} initialPlatform={initialPlatform} />
      )}
      <CollapsibleCard title="About" defaultOpen={false}>
        <RepoMarkdown owner={owner} repo={repo}>
          {readme}
        </RepoMarkdown>
      </CollapsibleCard>
    </>
  );
}

function BackIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <path
        d="M10 3 5 8l5 5"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
