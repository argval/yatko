import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Privacy — Yatko",
  description: "What Yatko collects, caches, and does not store.",
  alternates: { canonical: "/privacy" },
};

export default function PrivacyPage() {
  return (
    <main className="flex-1 px-4 py-16 sm:py-20">
      <article className="mx-auto max-w-xl space-y-8 text-sm leading-relaxed text-foreground/90">
        <div className="space-y-3">
          <p>
            <Link href="/" className="text-muted hover:text-foreground transition-colors">
              ← Yatko
            </Link>
          </p>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">Privacy</h1>
          <p className="text-muted">Last updated July 30, 2026</p>
        </div>

        <section className="space-y-3">
          <h2 className="text-base font-medium text-foreground">What Yatko is</h2>
          <p>
            Yatko turns public GitHub release pages into clean download links. It is an
            independent project and is{" "}
            <strong className="font-medium text-foreground">not affiliated with GitHub, Inc.</strong>
          </p>
        </section>

        <section className="space-y-3">
          <h2 className="text-base font-medium text-foreground">What we fetch</h2>
          <p>
            When you visit a Yatko URL, our servers request public metadata from GitHub’s API
            (releases, assets, README content, and repo search suggestions). Downloads redirect
            you to GitHub’s own asset URLs — Yatko does not proxy or store the binary files.
          </p>
        </section>

        <section className="space-y-3">
          <h2 className="text-base font-medium text-foreground">Caching</h2>
          <p>
            Public release and search responses may be cached briefly so the same repo does not
            hit GitHub on every request. Cache entries are about public repo data, not your
            identity.
          </p>
        </section>

        <section className="space-y-3">
          <h2 className="text-base font-medium text-foreground">Accounts and personal data</h2>
          <p>
            Yatko has no user accounts and does not ask for a login. We do not sell personal data.
            Like most websites, infrastructure logs may briefly include technical details such as
            IP address, User-Agent, and the path requested — used for rate limiting, abuse
            prevention, and keeping the service reliable.
          </p>
        </section>

        <section className="space-y-3">
          <h2 className="text-base font-medium text-foreground">Analytics</h2>
          <p>
            The site uses Vercel Analytics and Speed Insights for aggregate traffic and
            performance. Those products are operated by Vercel under their own privacy terms.
          </p>
        </section>

        <section className="space-y-3">
          <h2 className="text-base font-medium text-foreground">Contact</h2>
          <p>
            Questions or requests: open an issue on{" "}
            <a
              href="https://github.com/argval/yatko"
              target="_blank"
              rel="noopener noreferrer"
              className="font-medium text-foreground underline underline-offset-2 hover:no-underline"
            >
              github.com/argval/yatko
            </a>
            . The project source is available under the MIT License.
          </p>
        </section>
      </article>
    </main>
  );
}
