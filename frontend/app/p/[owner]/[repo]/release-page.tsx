import { Suspense } from "react";
import Image from "next/image";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import { InstallCommands, type InstallCommand, type InstallPlatform } from "./install-commands";
import { DownloadSection } from "./download-section";
import { VersionSelector } from "./version-selector";
import { PrereleaseToggle } from "./prerelease-toggle";
import { AllDownloads } from "./all-downloads";
import { ShareLinks } from "./share-links";
import { CollapsibleCard } from "./collapsible-card";
import type { Asset } from "./platform-utils";

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
  readme: string;
};

export type ReleaseSummary = {
  tag_name: string;
  name: string;
  published_at: string;
  prerelease: boolean;
};

const proseClass =
  "prose prose-sm dark:prose-invert max-w-none [&_a]:text-blue-500 [&_a:hover]:underline [&_h2]:text-base [&_h2]:font-semibold [&_h2]:mt-6 [&_h2]:mb-2 [&_h3]:text-sm [&_h3]:font-semibold [&_h3]:mt-4 [&_h3]:mb-2 [&_ul]:list-disc [&_ul]:pl-5 [&_ul]:space-y-1 [&_li]:text-sm [&_p]:text-sm [&_p]:leading-relaxed [&_code]:text-xs [&_code]:bg-foreground/5 [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:rounded [&_code]:break-words [&_pre]:bg-foreground/5 [&_pre]:p-4 [&_pre]:rounded-lg [&_pre]:overflow-x-auto [&_table]:w-full [&_table]:border-collapse [&_thead]:bg-foreground/5 [&_th]:border [&_th]:border-foreground/10 [&_th]:px-4 [&_th]:py-2 [&_th]:text-left [&_th]:font-semibold [&_td]:border [&_td]:border-foreground/10 [&_td]:px-4 [&_td]:py-2";

const readmeProseClass =
  proseClass +
  " [&_h1]:text-xl [&_h1]:font-bold [&_h1]:mt-6 [&_h1]:mb-3 [&_ol]:list-decimal [&_ol]:pl-5 [&_ol]:space-y-1 [&_img]:rounded-lg [&_img]:max-w-full";

export function ReleasePageBody({
  owner,
  repo,
  release,
  releasesPromise,
  checksumsPromise,
}: {
  owner: string;
  repo: string;
  release: ReleaseData;
  releasesPromise: Promise<ReleaseSummary[]>;
  checksumsPromise: Promise<Record<string, string>>;
}) {
  const publishedDate = new Date(release.published_at).toLocaleDateString("en-US", {
    year: "numeric",
    month: "long",
    day: "numeric",
  });

  const installCommands = release.readme ? extractInstallCommands(release.readme) : [];

  return (
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
            <a href={`https://github.com/${owner}/${repo}`} className="hover:text-foreground transition-colors">
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
          {release.description && (
            <div className="text-center text-muted leading-relaxed max-w-md [&_p]:m-0 [&_a]:text-blue-500 [&_a:hover]:underline">
              <Markdown remarkPlugins={[remarkGfm]}>{release.description}</Markdown>
            </div>
          )}
          <DownloadSection
            owner={owner}
            repo={repo}
            assets={release.assets}
            tagName={release.tag_name}
            publishedDate={publishedDate}
            checksumsPromise={checksumsPromise}
          />
          <Suspense fallback={null}>
            <ReleaseVersionControls
              owner={owner}
              repo={repo}
              currentTag={release.tag_name}
              prerelease={release.prerelease}
              releasesPromise={releasesPromise}
            />
          </Suspense>
        </div>

        {/* CLI Installation */}
        {installCommands.length > 0 && (
          <InstallCommands commands={installCommands} />
        )}

        {/* About (README) */}
        {release.readme && (
          <CollapsibleCard title="About" defaultOpen={false}>
            <div className={readmeProseClass}>
              <Markdown
                remarkPlugins={[remarkGfm]}
                rehypePlugins={[
                  rehypeRaw,
                  [rehypeSanitize, {
                    ...defaultSchema,
                    attributes: {
                      ...defaultSchema.attributes,
                      img: [...(defaultSchema.attributes?.img ?? []), "src", "alt", "width", "height", "align"],
                      svg: ["xmlns", "viewBox", "width", "height", "fill", "class", "style"],
                      path: ["d", "fill", "stroke", "strokeWidth"],
                    },
                    tagNames: [...(defaultSchema.tagNames ?? []), "svg", "path", "circle", "rect", "g"],
                  }],
                ]}
                urlTransform={(url) => resolveReadmeUrl(url, owner, repo)}
              >
                {release.readme}
              </Markdown>
            </div>
          </CollapsibleCard>
        )}

        {/* Release notes */}
        {release.body && (
          <CollapsibleCard title="Release Notes">
            <div className={proseClass}>
              <Markdown remarkPlugins={[remarkGfm]}>{release.body}</Markdown>
            </div>
          </CollapsibleCard>
        )}

        {/* All downloads */}
        <AllDownloads assets={release.assets} />

        {/* Share */}
        <ShareLinks owner={owner} repo={repo} />
      </div>
    </main>
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

function extractInstallCommands(readme: string): InstallCommand[] {
  const commands = new Map<string, InstallPlatform>();
  const codeBlockRe = /```[^\n]*\n([\s\S]*?)```/g;
  const patterns: { platform: InstallPlatform; re: RegExp }[] = [
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(pip install\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(npm install\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(npx\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(yarn add\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(pnpm add\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(cargo install\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(go install\s+.+)/ },
    { platform: "macos", re: /^\s*(?:\$|>)?\s*(brew install\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(gem install\s+.+)/ },
    { platform: "linux", re: /^\s*(?:\$|>)?\s*(apt(?:-get)?\s+install\s+.+)/ },
    { platform: "windows", re: /^\s*(?:\$|>)?\s*(winget install\s+.+)/ },
    { platform: "windows", re: /^\s*(?:\$|>)?\s*(choco install\s+.+)/ },
    { platform: "windows", re: /^\s*(?:\$|>)?\s*(scoop install\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(curl\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(wget\s+.+)/ },
  ];
  let match;
  while ((match = codeBlockRe.exec(readme)) !== null) {
    for (const line of match[1].split("\n")) {
      for (const { platform, re } of patterns) {
        const m = line.match(re);
        if (m) commands.set(m[1].trim(), platform);
      }
    }
  }
  return [...commands].map(([command, platform]) => ({ command, platform }));
}

function resolveReadmeUrl(url: string, owner: string, repo: string): string {
  if (!url) return url;
  if (url.startsWith("http://") || url.startsWith("https://") || url.startsWith("data:")) return url;
  const path = url.replace(/^\.\//, "");
  return `https://raw.githubusercontent.com/${owner}/${repo}/HEAD/${path}`;
}
