import Link from "next/link";
import { StatusCard, statusCardPrimaryAction, statusCardSecondaryAction } from "./status-card";

export default function NotFound() {
  return (
    <StatusCard
      emoji="📦"
      title="No releases found"
      description="This repo doesn't exist or hasn't published any releases yet."
    >
      <Link href="/" className={statusCardPrimaryAction}>
        Back to search
      </Link>
      <a href="https://github.com" className={statusCardSecondaryAction}>
        Open GitHub
      </a>
    </StatusCard>
  );
}
