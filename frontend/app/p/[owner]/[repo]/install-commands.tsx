"use client";

import { useCopy } from "./use-copy";

export function InstallCommands({ commands }: { commands: string[] }) {
  return (
    <div className="border border-border rounded-xl bg-surface/60 p-6 sm:p-8">
      <h2 className="text-lg font-semibold tracking-tight mb-4">Quick Install</h2>
      <div className="space-y-2">
        {commands.map((cmd) => (
          <CopyBlock key={cmd} command={cmd} />
        ))}
      </div>
    </div>
  );
}

function CopyBlock({ command }: { command: string }) {
  const [copied, copy] = useCopy();

  return (
    <div className="flex items-center gap-2 rounded-lg bg-foreground/5 px-4 py-3 font-mono text-sm group">
      <span className="text-foreground/40 select-none">$</span>
      <code className="flex-1 truncate">{command}</code>
      <button
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
