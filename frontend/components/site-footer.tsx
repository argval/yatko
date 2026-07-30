import Link from "next/link";

export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-border/60 px-4 py-6">
      <nav className="mx-auto flex max-w-xl items-center justify-center gap-3 text-xs text-muted">
        <Link href="/privacy" className="hover:text-foreground transition-colors">
          Privacy
        </Link>
        <a
          href="https://github.com/argval/yatko"
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
