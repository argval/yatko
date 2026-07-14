"use client";

import { StatusCard, statusCardPrimaryAction, statusCardSecondaryAction } from "./status-card";

export function ErrorState({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <StatusCard
      emoji="⚠️"
      title="Something went wrong"
      description={error.message || "Couldn't load release info. The backend may be unavailable."}
    >
      <button onClick={reset} className={statusCardPrimaryAction}>
        Try again
      </button>
      <a href="/" className={statusCardSecondaryAction}>
        Back to search
      </a>
    </StatusCard>
  );
}
