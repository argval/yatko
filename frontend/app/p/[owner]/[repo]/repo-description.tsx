import type { ReactNode } from "react";

const URL_RE = /https?:\/\/[^\s<>"'`]+/g;

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
