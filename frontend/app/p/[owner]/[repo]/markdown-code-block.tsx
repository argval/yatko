"use client";

import { CopyButton } from "./copy-button";

export function MarkdownCodeBlock({
  code,
  language,
}: {
  code: string;
  language?: string;
}) {
  const label = language?.trim() || "text";

  return (
    <div className="group relative my-3 rounded-lg bg-foreground/5 overflow-hidden">
      <div className="flex items-center justify-between gap-2 px-3 py-1.5 border-b border-foreground/5">
        <span className="text-[10px] font-medium uppercase tracking-wider text-muted font-sans">
          {label}
        </span>
        <CopyButton text={code} size={14} label="Copy code" />
      </div>
      <pre className="m-0 p-4 overflow-x-auto bg-transparent rounded-none text-foreground">
        <code className="text-xs bg-transparent p-0 rounded-none break-words whitespace-pre text-inherit">
          {code}
        </code>
      </pre>
    </div>
  );
}
