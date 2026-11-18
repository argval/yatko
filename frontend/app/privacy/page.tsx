import type { Metadata } from "next";
import { BackToYatko } from "@/components/back-to-yatko";
import { GITHUB_REPO_URL, TWITTER_URL } from "@/lib/site";

const title = "Privacy";
const description =
  "Privacy policy for Yatko (yatko.app) — what we collect, cache, and do not store when you use clean GitHub release download links.";
const contactLink =
  "inline-flex min-h-11 items-center rounded-lg border border-border bg-surface px-3 font-medium text-foreground hover:bg-foreground/5 active:bg-foreground/10 focus-visible:outline-2 focus-visible:outline-offset-2";

export const metadata: Metadata = {
  title,
  description,
  alternates: { canonical: "/privacy" },
  openGraph: {
    title: "Privacy · Yatko",
    description,
    type: "website",
    siteName: "Yatko",
    url: "/privacy",
  },
  twitter: {
    card: "summary_large_image",
    title: "Privacy · Yatko",
    description,
  },
};

export default function PrivacyPage() {
  return (
    <>
      <BackToYatko />
      <main className="flex-1 px-4 py-16 sm:py-20">
        <article className="mx-auto max-w-xl space-y-8 text-sm leading-relaxed text-foreground/90">
          <div className="space-y-3">
            <h1 className="text-3xl font-semibold tracking-tight text-foreground">Privacy</h1>
            <p className="text-muted">Last updated <time dateTime="2026-11-18">November 18, 2026</time></p>
          </div>

          <section aria-labelledby="contact" className="flex flex-col gap-3 rounded-xl border border-border bg-surface/60 p-4">
            <h2 id="contact" className="text-base font-medium text-foreground">Contact and reporting</h2>
            <p>
              For download problems, include the Yatko link and your operating system.
              Use X for privacy questions or requests.
            </p>
            <div className="flex flex-wrap gap-2">
              <a href={`${GITHUB_REPO_URL}/issues/new`} target="_blank" rel="noopener noreferrer" className={contactLink}>
                Report a bug on GitHub
              </a>
              <a href={TWITTER_URL} target="_blank" rel="noopener noreferrer" className={contactLink}>
                Contact @argvalue on X
              </a>
            </div>
            <p className="text-muted">GitHub issues are public. Leave out personal or sensitive details.</p>
          </section>

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
          <h2 className="text-base font-medium text-foreground">Choosing your download</h2>
          <p>
            To pick the right file for your computer, Yatko sends a release&rsquo;s public file
            names, the repository name, and your device type (such as macOS on Apple Silicon) to
            TypeSafe&rsquo;s decision model. It does not send your IP address or browser
            details. TypeSafe processes that data under its own privacy terms.
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
          <h2 className="text-base font-medium text-foreground">License</h2>
          <p>
            Yatko’s source code is available under the{" "}
            <a
              href={`${GITHUB_REPO_URL}/blob/main/LICENSE`}
              target="_blank"
              rel="noopener noreferrer"
              className="font-medium text-foreground underline underline-offset-2"
            >
              MIT License
            </a>
            .
          </p>
        </section>
      </article>
    </main>
    </>
  );
}
