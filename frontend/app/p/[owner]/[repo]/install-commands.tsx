"use client";

import { useState } from "react";
import {
  type InstallCommand,
  type InstallPlatform,
} from "./extract-install-commands";
import { installCommandMatchesTarget, platformLabels as basePlatformLabels, type Platform } from "./platform-utils";
import { usePlatform, type DetectedPlatform } from "./use-platform";
import { CollapsibleCard } from "./collapsible-card";
import { CopyButton } from "./copy-button";
import { PlatformFilterToggle } from "./platform-filter-toggle";

export type { InstallCommand, InstallPlatform } from "./extract-install-commands";

const platformLabels: Record<InstallPlatform | Platform | "unknown", string> = {
  ...basePlatformLabels,
  universal: "Universal",
  unknown: "Target unknown",
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
        (command) => installCommandMatchesTarget(command, detected),
      )
    : commands;

  // Prefer this OS, then universal, then everything else (when filter is off).
  const ordered = [...visible].sort((a, b) => {
    const rank = (command: InstallCommand) => {
      const platforms = command.platforms ?? [command.platform];
      if (platforms.some((platform) => platform === detected.platform)) return 0;
      if (platforms.includes("universal")) return 1;
      return 2;
    };
    return rank(a) - rank(b);
  });

  return (
    <CollapsibleCard title="CLI Installation">
      <p className="text-xs text-foreground/45 mb-2">
        Extracted from this repo&apos;s README — review before running.
      </p>
      <PlatformFilterToggle checked={filterEnabled} onChange={setFilterEnabled} />

      {ordered.length === 0 && (
        <p className="text-sm text-foreground/40 py-2">
          {detected.platform
            ? `No install commands found for ${platformLabels[detected.platform]}. Try unchecking the filter.`
            : "Select an operating system above, or uncheck the filter to browse commands."}
        </p>
      )}

      <div className="space-y-2">
        {ordered.map((command) => (
          <div key={JSON.stringify(command)}>
            <CopyBlock command={command.command} platform={command.platform} platforms={command.platforms} arches={command.arches} />
            <InstallPrerequisites command={command} target={detected} />
          </div>
        ))}
      </div>
    </CollapsibleCard>
  );
}

/** Show stated requirements whose OS and CPU match the selected device. */
export function InstallPrerequisites({ command, target }: { command: InstallCommand; target: DetectedPlatform }) {
  const prerequisites = command.prerequisiteTargets
    ? [...new Set(command.prerequisiteTargets.filter((prerequisite) => installCommandMatchesTarget(prerequisite, target)).map((prerequisite) => prerequisite.command))]
    : command.prerequisites ?? [];
  if (!prerequisites.length) return null;
  return <p className="text-xs text-muted px-4 py-1">Prerequisites: {prerequisites.join("; ")}</p>;
}

function CopyBlock({ command, platform, platforms, arches }: { command: string; platform: InstallPlatform; platforms?: InstallPlatform[]; arches?: string[] }) {
  return (
    <div className="flex items-center gap-2 rounded-lg bg-foreground/5 px-4 py-3 font-mono text-sm group">
      <span className="text-foreground/40 select-none shrink-0">$</span>
      <div className="flex-1 min-w-0 overflow-x-auto [scrollbar-width:thin] [&::-webkit-scrollbar]:h-1 [&::-webkit-scrollbar-track]:bg-transparent [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-foreground/20 hover:[&::-webkit-scrollbar-thumb]:bg-foreground/30">
        <code className="whitespace-pre">{command}</code>
      </div>
      <span className="shrink-0 text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-foreground/10 text-foreground/60 font-sans">
        {[(platforms ?? [platform]).map((value) => platformLabels[value]).join(" / "), arches?.join(" / ")].filter(Boolean).join(" · ")}
      </span>
      <CopyButton text={command} />
    </div>
  );
}
