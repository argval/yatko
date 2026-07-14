import { StatusCard, statusCardPrimaryAction } from "../status-card";

export default function NotFound() {
  return (
    <StatusCard
      emoji="📦"
      title="Release not found"
      description="This version tag doesn't exist or hasn't been published."
    >
      <a href="/" className={statusCardPrimaryAction}>
        Back to search
      </a>
    </StatusCard>
  );
}
