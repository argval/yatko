"use client";

import { useState, type ReactNode, type ToggleEvent } from "react";

export function CollapsibleCard({
  title,
  defaultOpen = true,
  mountChildren = "always",
  children,
}: {
  title: string;
  defaultOpen?: boolean;
  /** "when-opened" skips rendering children until the section is opened once. */
  mountChildren?: "always" | "when-opened";
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const [openedOnce, setOpenedOnce] = useState(defaultOpen);

  function handleToggle(e: ToggleEvent<HTMLDetailsElement>) {
    const next = e.currentTarget.open;
    setOpen(next);
    if (next) setOpenedOnce(true);
  }

  const showChildren = mountChildren === "when-opened" ? openedOnce : true;

  return (
    <details
      open={open}
      onToggle={handleToggle}
      className="border border-border rounded-xl bg-surface/60 group"
    >
      <summary className="px-6 sm:px-8 py-5 cursor-pointer font-semibold tracking-tight text-lg flex items-center justify-between select-none">
        {title}
        <svg
          width="16"
          height="16"
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="transition-transform group-open:rotate-180"
          aria-hidden
        >
          <path d="M4 6l4 4 4-4" />
        </svg>
      </summary>
      <div className="px-6 sm:px-8 pb-6 sm:pb-8">{showChildren ? children : null}</div>
    </details>
  );
}
