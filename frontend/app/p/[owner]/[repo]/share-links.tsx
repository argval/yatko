"use client";

import { CollapsibleCard } from "./collapsible-card";
import { CopyButton } from "./copy-button";

type ShareLink = {
  label: string;
  url: string;
  description: string;
};

export function ShareLinks({ owner, repo }: { owner: string; repo: string }) {
  const base = "https://yatko.app";
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
      label: "Direct link API",
      url: `${base}/api/link/${owner}/${repo}`,
      description: "Returns JSON with the resolved download URL - for CI/scripts",
    },
  ];

  return (
    <CollapsibleCard title="Share">
      <ul className="space-y-4">
        {links.map((link) => (
          <ShareRow key={link.label} {...link} />
        ))}
      </ul>
    </CollapsibleCard>
  );
}

function ShareRow({ label, url, description }: ShareLink) {
  return (
    <li className="space-y-1.5">
      <div className="flex items-center justify-between gap-4">
        <span className="text-sm font-medium">{label}</span>
        <CopyButton text={url} label={`Copy ${label}`} />
      </div>
      <div className="flex items-center gap-2 rounded-lg bg-foreground/5 px-3 py-2">
        <code className="flex-1 text-xs font-mono truncate text-muted">{url}</code>
      </div>
      <p className="text-xs text-muted">{description}</p>
    </li>
  );
}
