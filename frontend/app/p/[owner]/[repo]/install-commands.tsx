"use client";

import { useState } from "react";
import {
  type InstallCommand,
  type InstallPlatform,
} from "./extract-install-commands";
import { platformLabels as basePlatformLabels, type Platform } from "./platform-utils";
import { usePlatform } from "./use-platform";
import { CollapsibleCard } from "./collapsible-card";
import { CopyButton } from "./copy-button";
import { PlatformFilterToggle } from "./platform-filter-toggle";

export type { InstallCommand, InstallPlatform } from "./extract-install-commands";

const platformLabels: Record<InstallPlatform | Platform, string> = {
  ...basePlatformLabels,
  universal: "Universal",
};

export function InstallCommands({ commands }: { commands: InstallCommand[] }) {
  // Same client-side usePlatform() as DownloadSection — default filter on so
  // CLI install mirrors the download button's "your platform" behavior.
  const detected = usePlatform();
  const [filterEnabled, setFilterEnabled] = useState(true);

  if (!detected) {
    return (
      <CollapsibleCard title="CLI Installation">
        <p className="text-xs text-foreground/45 mb-2">
          Extracted from this repo&apos;s README — review before running.
        </p>
        <div
          className="h-10 rounded-lg bg-foreground/[0.06] animate-pulse"
          aria-hidden
        />
        <p className="sr-only" role="status" aria-live="polite">
          Detecting platform…
        </p>
      </CollapsibleCard>
    );
  }

  const visible = filterEnabled
    ? commands.filter(
        (c) => c.platform === "universal" || c.platform === detected.platform,
      )
    : commands;

  // Prefer this OS, then universal, then everything else (when filter is off).
  const ordered = [...visible].sort((a, b) => {
    const rank = (p: InstallPlatform) => {
      if (p === detected.platform) return 0;
      if (p === "universal") return 1;
      return 2;
    };
    return rank(a.platform) - rank(b.platform);
  });

  return (
    <CollapsibleCard title="CLI Installation">
      <p className="text-xs text-foreground/45 mb-2">
        Extracted from this repo&apos;s README — review before running.
      </p>
      <PlatformFilterToggle checked={filterEnabled} onChange={setFilterEnabled} />

      {ordered.length === 0 && (
        <p className="text-sm text-foreground/40 py-2">
          No install commands found for {platformLabels[detected.platform]}. Try unchecking the
          filter.
        </p>
      )}

      <div className="space-y-2">
        {ordered.map(({ command, platform: cmdPlatform }) => (
          <CopyBlock key={command} command={command} platform={cmdPlatform} />
        ))}
      </div>
    </CollapsibleCard>
  );
}

function CopyBlock({ command, platform }: { command: string; platform: InstallPlatform }) {
  return (
    <div className="flex items-center gap-2 rounded-lg bg-foreground/5 px-4 py-3 font-mono text-sm group">
      <span className="text-foreground/40 select-none shrink-0">$</span>
      <div className="flex-1 min-w-0 overflow-x-auto">
        <code className="whitespace-pre">{command}</code>
      </div>
      <span className="shrink-0 text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-foreground/10 text-foreground/60 font-sans">
        {platformLabels[platform]}
      </span>
      <CopyButton text={command} />
    </div>
  );
}
