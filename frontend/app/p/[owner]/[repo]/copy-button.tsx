"use client";

import { useCopy } from "./use-copy";
import { CheckIcon, CopyIcon } from "./icons";

export function CopyButton({
  text,
  label = "Copy to clipboard",
  size = 16,
  className = "shrink-0 p-1 rounded text-foreground/30 hover:text-foreground/60 transition-colors",
}: {
  text: string;
  label?: string;
  size?: number;
  className?: string;
}) {
  const [copied, copy] = useCopy();
  return (
    <button
      type="button"
      onClick={() => copy(text)}
      className={className}
      aria-label={copied ? "Copied" : label}
    >
      {copied ? <CheckIcon size={size} /> : <CopyIcon size={size} />}
    </button>
  );
}
