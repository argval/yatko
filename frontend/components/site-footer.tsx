import Link from "next/link";

export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-border/60 px-4 py-6">
      <div className="mx-auto flex max-w-xl flex-col items-center gap-2 text-center text-xs text-muted sm:flex-row sm:justify-between sm:text-left">
        <p>Yatko is not affiliated with GitHub, Inc.</p>
        <nav className="flex items-center gap-3">
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
      </div>
    </footer>
  );
}
