"use client";

import { useState } from "react";
import { platformLabels as basePlatformLabels, type Platform } from "./platform-utils";
import { usePlatform } from "./use-platform";
import { CollapsibleCard } from "./collapsible-card";
import { CopyButton } from "./copy-button";
import { PlatformFilterToggle } from "./platform-filter-toggle";

export type InstallPlatform = "macos" | "windows" | "linux" | "universal";

export type InstallCommand = {
  command: string;
  platform: InstallPlatform;
};

const platformLabels: Record<InstallPlatform, string> = {
  ...basePlatformLabels,
  universal: "Universal",
};

export function InstallCommands({
  commands,
  initialPlatform,
}: {
  commands: InstallCommand[];
  initialPlatform?: Platform;
}) {
  const [platform] = usePlatform(initialPlatform);
  const [filterEnabled, setFilterEnabled] = useState(false);

  const visible = filterEnabled
    ? commands.filter((c) => c.platform === "universal" || c.platform === platform)
    : commands;

  return (
    <CollapsibleCard title="CLI Installation">
      <PlatformFilterToggle checked={filterEnabled} onChange={setFilterEnabled} />

      {visible.length === 0 && (
        <p className="text-sm text-foreground/40 py-2">
          No install commands found for {platformLabels[platform]}. Try unchecking the filter.
        </p>
      )}

      <div className="space-y-2">
        {visible.map(({ command, platform: cmdPlatform }) => (
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
