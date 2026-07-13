"use client";

import { useCopy } from "./use-copy";

type ShareLink = {
  label: string;
  url: string;
  description: string;
};

export function ShareLinks({ owner, repo }: { owner: string; repo: string }) {
  const base = "https://yoink.dev";
  const links: ShareLink[] = [
    {
      label: "Smart download",
      url: `${base}/dl/${owner}/${repo}`,
      description: "Redirects to the right binary for the user's platform",
    },
    {
      label: "Landing page",
      url: `${base}/p/${owner}/${repo}`,
      description: "Shareable download page with release notes and all assets",
    },
    {
      label: "Version badge",
      url: `${base}/badge/${owner}/${repo}`,
      description: "Embed in your README: ![version](https://yoink.dev/badge/owner/repo)",
    },
    {
      label: "Direct link API",
      url: `${base}/api/link/${owner}/${repo}`,
      description: "Returns JSON with the resolved download URL - for CI/scripts",
    },
  ];

  return (
    <div className="border border-border rounded-xl bg-surface/60 p-6 sm:p-8">
      <h2 className="text-lg font-semibold tracking-tight mb-4">Share</h2>
      <ul className="space-y-4">
        {links.map((link) => (
          <ShareRow key={link.label} {...link} />
        ))}
      </ul>
    </div>
  );
}

function ShareRow({ label, url, description }: ShareLink) {
  const [copied, copy] = useCopy();

  return (
    <li className="space-y-1.5">
      <div className="flex items-center justify-between gap-4">
        <span className="text-sm font-medium">{label}</span>
        <button
          type="button"
          onClick={() => copy(url)}
          className="shrink-0 text-xs text-muted hover:text-foreground transition-colors"
        >
          {copied ? "Copied!" : "Copy"}
        </button>
      </div>
      <div className="flex items-center gap-2 rounded-lg bg-foreground/5 px-3 py-2">
        <code className="flex-1 text-xs font-mono truncate text-muted">{url}</code>
      </div>
      <p className="text-xs text-muted">{description}</p>
    </li>
  );
}
