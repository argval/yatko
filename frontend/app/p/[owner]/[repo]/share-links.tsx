"use client";

import { CollapsibleCard } from "./collapsible-card";
import { CopyButton } from "./copy-button";

type ShareItem = {
  label: string;
  text: string;
  description: string;
};

export function ShareLinks({ owner, repo }: { owner: string; repo: string }) {
  const base = "https://yatko.app";
  // Landing page uses the github.com → yatko.app path (no /p/), so shared
  // links match the product URL and resolve to the same release page.
  const items: ShareItem[] = [
    {
      label: "Smart download",
      text: `${base}/dl/${owner}/${repo}`,
      description: "Redirects to the right binary for the user's platform",
    },
    {
      label: "Landing page",
      text: `${base}/${owner}/${repo}`,
      description: "Shareable download page with release notes and all assets",
    },
    {
      label: "Direct link API",
      text: `${base}/api/link/${owner}/${repo}`,
      description: "Returns JSON with the resolved download URL - for CI/scripts",
    },
    {
      label: "README snippet",
      text: `[Download](${base}/dl/${owner}/${repo})`,
      description: "Markdown link for your project README",
    },
  ];

  return (
    <CollapsibleCard title="Share">
      <ul className="space-y-4">
        {items.map((item) => (
          <ShareRow key={item.label} {...item} />
        ))}
      </ul>
    </CollapsibleCard>
  );
}

function ShareRow({ label, text, description }: ShareItem) {
  return (
    <li className="space-y-1.5">
      <div className="flex items-center justify-between gap-4">
        <span className="text-sm font-medium">{label}</span>
        <CopyButton text={text} label={`Copy ${label}`} />
      </div>
      <div className="flex items-center gap-2 rounded-lg bg-foreground/5 px-3 py-2">
        <code className="flex-1 text-xs font-mono truncate text-muted">{text}</code>
      </div>
      <p className="text-xs text-muted">{description}</p>
    </li>
  );
}
