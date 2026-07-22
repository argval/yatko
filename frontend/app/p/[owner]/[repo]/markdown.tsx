import type { ReactNode } from "react";
import Markdown, { defaultUrlTransform } from "react-markdown";
import type { Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import type { Schema } from "hast-util-sanitize";
import type { PluggableList } from "unified";
import { MarkdownCodeBlock } from "./markdown-code-block";

/** GitHub-flavored HTML that shows up in READMEs and release notes. */
const sanitizeSchema: Schema = {
  ...defaultSchema,
  attributes: {
    ...defaultSchema.attributes,
    details: [...(defaultSchema.attributes?.details ?? []), "open"],
    img: [
      ...(defaultSchema.attributes?.img ?? []),
      "src",
      "alt",
      "title",
      "width",
      "height",
      "align",
    ],
    a: [...(defaultSchema.attributes?.a ?? []), "title"],
    td: [...(defaultSchema.attributes?.td ?? []), "align", "colSpan", "rowSpan"],
    th: [...(defaultSchema.attributes?.th ?? []), "align", "colSpan", "rowSpan"],
    svg: ["xmlns", "viewBox", "width", "height", "fill", "class", "style", "aria-hidden", "role"],
    path: ["d", "fill", "stroke", "strokeWidth", "stroke-width"],
    circle: ["cx", "cy", "r", "fill", "stroke"],
    rect: ["x", "y", "width", "height", "rx", "ry", "fill", "stroke"],
    g: ["fill", "stroke", "transform"],
  },
  tagNames: [...(defaultSchema.tagNames ?? []), "svg", "path", "circle", "rect", "g"],
};

const remarkPlugins: PluggableList = [remarkGfm];
const rehypePlugins: PluggableList = [rehypeRaw, [rehypeSanitize, sanitizeSchema]];

/** Typography plugin + a few GFM extras in globals.css */
const proseBase =
  "prose prose-sm dark:prose-invert max-w-none prose-a:text-blue-500 prose-img:rounded-lg";

function isExternalHref(href: string | undefined): boolean {
  if (!href) return false;
  return href.startsWith("http://") || href.startsWith("https://") || href.startsWith("//");
}

function languageFromClassName(className: string | undefined): string | undefined {
  const match = /(?:^|\s)language-([\w-]+)/.exec(className ?? "");
  return match?.[1];
}

const markdownComponents: Components = {
  a({ href, children, target: _target, rel: _rel, ...props }) {
    const external = isExternalHref(href);
    // Drop any target/rel from props, then set ours last so raw HTML cannot strip noopener.
    return (
      <a
        href={href}
        {...props}
        {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
      >
        {children}
      </a>
    );
  },
  img({ src, alt, ...props }) {
    if (!src) return null;
    // Plain <img>: release/README assets come from many hosts; next/image needs an allowlist.
    return <img src={src} alt={alt ?? ""} loading="lazy" decoding="async" {...props} />;
  },
  // Unwrap <pre> so MarkdownCodeBlock owns the chrome (language label + copy).
  pre({ children }) {
    return <>{children}</>;
  },
  code({ className, children, ...props }) {
    const text = String(children).replace(/\n$/, "");
    const language = languageFromClassName(className);
    const isBlock = Boolean(language) || text.includes("\n");
    if (isBlock) {
      return <MarkdownCodeBlock code={text} language={language} />;
    }
    return (
      <code className={className} {...props}>
        {children}
      </code>
    );
  },
};

/**
 * Resolve repo-relative markdown URLs against raw.githubusercontent.com.
 * Anchors and non-path schemes are left alone (sanitize still vets protocols).
 */
export function resolveRepoContentUrl(
  url: string,
  owner: string,
  repo: string,
  ref = "HEAD",
): string {
  const safe = defaultUrlTransform(url);
  if (!safe) return safe;
  if (safe.startsWith("http://") || safe.startsWith("https://")) return safe;
  if (safe.startsWith("#") || safe.includes(":")) return safe;
  try {
    return new URL(safe, `https://raw.githubusercontent.com/${owner}/${repo}/${ref}/`).toString();
  } catch {
    return safe;
  }
}

/** Strip trailing punctuation that often sticks to autolinked URLs. */
function splitUrlAndTrailing(raw: string): { url: string; trailing: string } {
  let url = raw;
  let trailing = "";
  while (/[.,;:!?)\]}'"”’]$/.test(url)) {
    trailing = url.slice(-1) + trailing;
    url = url.slice(0, -1);
  }
  return { url, trailing };
}

const URL_RE = /https?:\/\/[^\s<>"'`]+/g;

/** Plain-text repo description with http(s) URLs turned into links. */
export function RepoDescription({ children }: { children: string }) {
  const nodes: ReactNode[] = [];
  let last = 0;
  for (const match of children.matchAll(URL_RE)) {
    const index = match.index ?? 0;
    if (index > last) nodes.push(children.slice(last, index));
    const { url, trailing } = splitUrlAndTrailing(match[0]);
    if (url) {
      nodes.push(
        <a
          key={`url-${index}`}
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          className="text-blue-500 hover:underline"
        >
          {url}
        </a>,
      );
    }
    if (trailing) nodes.push(trailing);
    last = index + match[0].length;
  }
  if (last < children.length) nodes.push(children.slice(last));

  return (
    <p className="text-center text-muted leading-relaxed max-w-md [&_a]:text-blue-500 [&_a:hover]:underline">
      {nodes}
    </p>
  );
}

export function RepoMarkdown({
  children,
  owner,
  repo,
  refName = "HEAD",
  className,
}: {
  children: string;
  owner: string;
  repo: string;
  /** Git ref used to resolve relative image/link paths (tag, branch, or HEAD). */
  refName?: string;
  className?: string;
}) {
  return (
    <div className={className ? `${proseBase} ${className}` : proseBase}>
      <Markdown
        remarkPlugins={remarkPlugins}
        rehypePlugins={rehypePlugins}
        urlTransform={(url) => resolveRepoContentUrl(url, owner, repo, refName)}
        components={markdownComponents}
      >
        {children}
      </Markdown>
    </div>
  );
}
