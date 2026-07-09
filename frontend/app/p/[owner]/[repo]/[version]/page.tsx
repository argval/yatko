import type { Metadata } from "next";
import Image from "next/image";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import { InstallCommands } from "../install-commands";
import { DownloadSection } from "../download-section";
import { VersionSelector } from "../version-selector";
import { PrereleaseToggle } from "../prerelease-toggle";
import { AllDownloads } from "../all-downloads";
import { ShareLinks } from "../share-links";
import { getRelease } from "../page";

type Props = {
  params: Promise<{ owner: string; repo: string; version: string }>;
};

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { owner, repo, version } = await params;
  return {
    title: `${repo} ${version} — Download | Yoink`,
    description: `Download ${owner}/${repo} version ${version}`,
  };
}

export default async function VersionedReleasePage({ params }: Props) {
  const { owner, repo, version } = await params;
  const release = await getRelease(owner, repo, version);

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
            className="rounded-2xl mx-auto"
          />
          <h1 className="text-4xl sm:text-5xl font-bold tracking-tight">{repo}</h1>
          <p className="text-foreground/60 text-lg">
            <a href={`https://github.com/${owner}/${repo}`} className="hover:underline">
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
            <p className="text-center text-foreground/60 max-w-md">{release.description}</p>
          )}
          <DownloadSection
            owner={owner}
            repo={repo}
            assets={release.assets}
            tagName={release.tag_name}
            publishedDate={publishedDate}
          />
          <VersionSelector
            owner={owner}
            repo={repo}
            currentTag={release.tag_name}
            showPrereleases={release.prerelease}
          />
          <PrereleaseToggle
            owner={owner}
            repo={repo}
            isCurrentPrerelease={release.prerelease}
          />
        </div>

        {/* Quick Install */}
        {installCommands.length > 0 && (
          <InstallCommands commands={installCommands} />
        )}

        {/* About (README) */}
        {release.readme && (
          <details className="border border-foreground/10 rounded-xl group">
            <summary className="px-6 sm:px-8 py-5 cursor-pointer font-semibold text-lg flex items-center justify-between select-none">
              About
              <svg
                width="16"
                height="16"
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                className="transition-transform group-open:rotate-180"
              >
                <path d="M4 6l4 4 4-4" />
              </svg>
            </summary>
            <div className="px-6 sm:px-8 pb-6 sm:pb-8">
              <div className="prose prose-sm dark:prose-invert max-w-none [&_a]:text-blue-500 [&_a:hover]:underline [&_h1]:text-xl [&_h1]:font-bold [&_h1]:mt-6 [&_h1]:mb-3 [&_h2]:text-base [&_h2]:font-semibold [&_h2]:mt-6 [&_h2]:mb-2 [&_h3]:text-sm [&_h3]:font-semibold [&_h3]:mt-4 [&_h3]:mb-2 [&_ul]:list-disc [&_ul]:pl-5 [&_ul]:space-y-1 [&_ol]:list-decimal [&_ol]:pl-5 [&_ol]:space-y-1 [&_li]:text-sm [&_p]:text-sm [&_p]:leading-relaxed [&_code]:text-xs [&_code]:bg-foreground/5 [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:rounded [&_code]:break-words [&_pre]:bg-foreground/5 [&_pre]:p-4 [&_pre]:rounded-lg [&_pre]:overflow-x-auto [&_img]:rounded-lg [&_img]:max-w-full [&_table]:w-full [&_table]:border-collapse [&_thead]:bg-foreground/5 [&_th]:border [&_th]:border-foreground/10 [&_th]:px-4 [&_th]:py-2 [&_th]:text-left [&_th]:font-semibold [&_td]:border [&_td]:border-foreground/10 [&_td]:px-4 [&_td]:py-2">
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
            </div>
          </details>
        )}

        {/* Release notes */}
        {release.body && (
          <div className="border border-foreground/10 rounded-xl p-6 sm:p-8">
            <h2 className="text-lg font-semibold mb-4">Release Notes</h2>
            <div className="prose prose-sm dark:prose-invert max-w-none [&_a]:text-blue-500 [&_a:hover]:underline [&_h2]:text-base [&_h2]:font-semibold [&_h2]:mt-6 [&_h2]:mb-2 [&_h3]:text-sm [&_h3]:font-semibold [&_h3]:mt-4 [&_h3]:mb-2 [&_ul]:list-disc [&_ul]:pl-5 [&_ul]:space-y-1 [&_li]:text-sm [&_p]:text-sm [&_p]:leading-relaxed [&_code]:text-xs [&_code]:bg-foreground/5 [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:rounded [&_code]:break-words [&_pre]:bg-foreground/5 [&_pre]:p-4 [&_pre]:rounded-lg [&_pre]:overflow-x-auto [&_table]:w-full [&_table]:border-collapse [&_thead]:bg-foreground/5 [&_th]:border [&_th]:border-foreground/10 [&_th]:px-4 [&_th]:py-2 [&_th]:text-left [&_th]:font-semibold [&_td]:border [&_td]:border-foreground/10 [&_td]:px-4 [&_td]:py-2">
              <Markdown remarkPlugins={[remarkGfm]}>{release.body}</Markdown>
            </div>
          </div>
        )}

        {/* All downloads */}
        <AllDownloads assets={release.assets} />

        {/* Share */}
        <ShareLinks owner={owner} repo={repo} />
      </div>
    </main>
  );
}

function extractInstallCommands(readme: string): string[] {
  const commands = new Set<string>();
  const codeBlockRe = /```[^\n]*\n([\s\S]*?)```/g;
  const patterns = [
    /^\s*(?:\$|>)?\s*(pip install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(npm install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(yarn add\s+\S+)/,
    /^\s*(?:\$|>)?\s*(pnpm add\s+\S+)/,
    /^\s*(?:\$|>)?\s*(cargo install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(go install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(brew install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(gem install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(apt(?:-get)?\s+install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(uv (?:add|pip install)\s+\S+)/,
    /^\s*(?:\$|>)?\s*(winget install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(choco install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(scoop install\s+\S+)/,
    /^\s*(?:\$|>)?\s*(curl\s+.+)/,
    /^\s*(?:\$|>)?\s*(wget\s+.+)/,
  ];
  let match;
  while ((match = codeBlockRe.exec(readme)) !== null) {
    for (const line of match[1].split("\n")) {
      for (const pat of patterns) {
        const m = line.match(pat);
        if (m) commands.add(m[1].trim());
      }
    }
  }
  return [...commands];
}

function resolveReadmeUrl(url: string, owner: string, repo: string): string {
  if (!url) return url;
  if (url.startsWith("http://") || url.startsWith("https://") || url.startsWith("data:")) return url;
  const path = url.replace(/^\.\//, "");
  return `https://raw.githubusercontent.com/${owner}/${repo}/HEAD/${path}`;
}
