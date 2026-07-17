"use client";

import { useState } from "react";
import { useCopy } from "./use-copy";
import { CollapsibleCard } from "./collapsible-card";
import { usePlatform } from "./platform-utils";

export type InstallPlatform = "macos" | "windows" | "linux" | "universal";

export type InstallCommand = {
  command: string;
  platform: InstallPlatform;
};

const platformLabels: Record<InstallPlatform, string> = {
  macos: "macOS",
  windows: "Windows",
  linux: "Linux",
  universal: "Universal",
};

export function InstallCommands({ commands }: { commands: InstallCommand[] }) {
  const [platform] = usePlatform();
  const [filterEnabled, setFilterEnabled] = useState(false);

  const visible = filterEnabled
    ? commands.filter((c) => c.platform === "universal" || c.platform === platform)
    : commands;

  return (
    <CollapsibleCard title="CLI Installation">
      <label className="flex items-center gap-2 text-xs text-muted cursor-pointer select-none hover:text-foreground transition-colors mb-4">
        <input
          type="checkbox"
          checked={filterEnabled}
          onChange={(e) => setFilterEnabled(e.target.checked)}
          className="rounded border-foreground/20 accent-foreground"
        />
        My platform only
      </label>

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
  const [copied, copy] = useCopy();

  return (
    <div className="flex items-center gap-2 rounded-lg bg-foreground/5 px-4 py-3 font-mono text-sm group">
      <span className="text-foreground/40 select-none shrink-0">$</span>
      <div className="flex-1 min-w-0 overflow-x-auto">
        <code className="whitespace-pre">{command}</code>
      </div>
      <span className="shrink-0 text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-foreground/10 text-foreground/60 font-sans">
        {platformLabels[platform]}
      </span>
      <button
        type="button"
        onClick={() => copy(command)}
        className="shrink-0 p-1 rounded text-foreground/30 hover:text-foreground/60 transition-colors"
        aria-label="Copy to clipboard"
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
      </button>
    </div>
  );
}

function CopyIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}
