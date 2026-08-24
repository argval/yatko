"use client";

import { useEffect, useTransition } from "react";
import Link from "next/link";
import { bustReleasePageCache } from "./bust-release-cache";
import { StatusCard, statusCardPrimaryAction, statusCardSecondaryAction } from "./status-card";

export function ReleaseError({
  message,
  owner,
  repo,
  version,
}: {
  message: string;
  owner: string;
  repo: string;
  version?: string;
}) {
  const [pending, startTransition] = useTransition();

  // Transient error HTML can still land in the Full Route Cache on first
  // generation. Bust immediately so Reload / the next visitor is not stuck
  // on a stale failure for up to `revalidate` seconds.
  useEffect(() => {
    void bustReleasePageCache(owner, repo, version);
  }, [owner, repo, version]);

  function retry() {
    startTransition(() => {
      void bustReleasePageCache(owner, repo, version).finally(() => {
        window.location.reload();
      });
    });
  }

  return (
    <StatusCard emoji="⚠️" title="Something went wrong" description={message}>
      <button
        type="button"
        onClick={retry}
        disabled={pending}
        className={statusCardPrimaryAction}
      >
        Try again
      </button>
      <Link href="/" className={statusCardSecondaryAction}>
        Back to search
      </Link>
    </StatusCard>
  );
}
