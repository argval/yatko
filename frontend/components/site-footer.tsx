import Link from "next/link";
import { GITHUB_REPO_URL, SITE_NAME } from "@/lib/site";

const footerLink =
  "inline-flex min-h-11 items-center px-2 transition-colors hover:text-foreground active:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2";

export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-border/60 px-4 py-2">
      <nav
        aria-label={`${SITE_NAME} site`}
        className="mx-auto flex max-w-xl flex-wrap items-center justify-center gap-1 text-xs text-muted"
      >
        <Link href="/changelog" className={footerLink}>
          Changelog
        </Link>
        <Link href="/privacy" className={footerLink}>
          Privacy
        </Link>
        <a
          href={GITHUB_REPO_URL}
          target="_blank"
          rel="noopener noreferrer"
          className={footerLink}
        >
          Source
        </a>
      </nav>
    </footer>
  );
}
