import Link from "next/link";
import { GITHUB_REPO_URL, SITE_NAME } from "@/lib/site";

export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-border/60 px-4 py-6">
      <nav
        aria-label={`${SITE_NAME} site`}
        className="mx-auto flex max-w-xl items-center justify-center gap-3 text-xs text-muted"
      >
        <Link href="/privacy" className="hover:text-foreground transition-colors">
          Privacy
        </Link>
        <a
          href={GITHUB_REPO_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="hover:text-foreground transition-colors"
        >
          Source
        </a>
      </nav>
    </footer>
  );
}
