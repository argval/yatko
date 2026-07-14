import { StatusCard, statusCardPrimaryAction, statusCardSecondaryAction } from "./status-card";

export function ReleaseError({ message }: { message: string }) {
  return (
    <StatusCard emoji="⚠️" title="Something went wrong" description={message}>
      <a href="" className={statusCardPrimaryAction}>
        Try again
      </a>
      <a href="/" className={statusCardSecondaryAction}>
        Back to search
      </a>
    </StatusCard>
  );
}
