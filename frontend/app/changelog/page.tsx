import { readFile } from "node:fs/promises";
import { join } from "node:path";
import type { Metadata } from "next";
import ReactMarkdown from "react-markdown";
import { BackToYatko } from "@/components/back-to-yatko";
import { SITE_NAME } from "@/lib/site";

const title = "Changelog";
const description = "New features and fixes for Yatko’s GitHub release download pages.";
const socialTitle = `${title} · ${SITE_NAME}`;

export const metadata: Metadata = {
  title,
  description,
  alternates: { canonical: "/changelog" },
  openGraph: {
    title: socialTitle,
    description,
    type: "website",
    siteName: SITE_NAME,
    url: "/changelog",
  },
  twitter: { card: "summary_large_image", title: socialTitle, description },
};

export default async function ChangelogPage() {
  const markdown = await readFile(join(process.cwd(), "CHANGELOG.md"), "utf8");

  return (
    <>
      <BackToYatko />
      <main className="flex-1 px-4 py-16 sm:py-20">
        <article className="mx-auto flex max-w-xl flex-col gap-2">
          <ReactMarkdown
            components={{
              h1: ({ children }) => (
                <h1 className="mbe-4 text-3xl font-semibold tracking-tight text-foreground">{children}</h1>
              ),
              h2: ({ children }) => (
                <h2 className="text-lg font-medium text-foreground">{children}</h2>
              ),
              p: ({ children }) => <p className="text-sm text-muted">{children}</p>,
              ul: ({ children }) => (
                <ul className="list-disc space-y-1 ps-5 text-sm leading-relaxed text-foreground/90">
                  {children}
                </ul>
              ),
              hr: () => <hr className="my-4 border-border" />,
            }}
          >
            {markdown}
          </ReactMarkdown>
        </article>
      </main>
    </>
  );
}
