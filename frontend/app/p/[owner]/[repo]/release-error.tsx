"use client";

import Link from "next/link";
import { StatusCard, statusCardPrimaryAction, statusCardSecondaryAction } from "./status-card";

export function ReleaseError({ message }: { message: string }) {
  return (
    <StatusCard emoji="⚠️" title="Something went wrong" description={message}>
      <button type="button" onClick={() => window.location.reload()} className={statusCardPrimaryAction}>
        Try again
      </button>
      <Link href="/" className={statusCardSecondaryAction}>
        Back to search
      </Link>
    </StatusCard>
  );
}
