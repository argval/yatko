import Link from "next/link";
import { StatusCard, statusCardPrimaryAction, statusCardSecondaryAction } from "./status-card";

export function NotFoundCard({
  owner,
  repo,
  repoExists,
}: {
  owner?: string;
  repo?: string;
  repoExists?: boolean;
}) {
  const githubUrl = owner && repo ? `https://github.com/${owner}/${repo}` : "https://github.com";
  const title = repoExists ? "No releases yet" : "No releases found";
  const description = repoExists
    ? "This repo exists but hasn't published any releases yet."
    : "This repo doesn't exist or hasn't published any releases yet.";
  return (
    <StatusCard emoji="📦" title={title} description={description}>
      {repoExists && owner && repo ? (
        <a href={`/code/${owner}/${repo}`} className={statusCardPrimaryAction}>
          Download source code
        </a>
      ) : (
        <Link href="/" className={statusCardPrimaryAction}>
          Back to search
        </Link>
      )}
      <a href={githubUrl} target="_blank" rel="noopener noreferrer" className={statusCardSecondaryAction}>
        Open GitHub
      </a>
      {repoExists && (
        <Link href="/" className={statusCardSecondaryAction}>
          Back to search
        </Link>
      )}
    </StatusCard>
  );
}

export default function NotFound() {
  return <NotFoundCard />;
}
