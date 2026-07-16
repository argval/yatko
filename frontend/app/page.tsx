"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

const EXAMPLES = ["cli/cli", "neovim/neovim", "astral-sh/uv", "BurntSushi/ripgrep"];

export default function Home() {
  const [input, setInput] = useState("");
  const router = useRouter();

  function parseInput(value: string) {
    return value.match(/(?:github\.com\/)?([a-zA-Z0-9._-]+)\/([a-zA-Z0-9._-]+)\/?$/);
  }

  function navigate(slug: string) {
    const match = slug.match(/([a-zA-Z0-9._-]+)\/([a-zA-Z0-9._-]+)/);
    if (!match) return;
    router.push(`/p/${match[1]}/${match[2]}`);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const match = parseInput(input);
    if (match) navigate(`${match[1]}/${match[2]}`);
  }

  const parsedHint = parseInput(input);
  const hintSlug = parsedHint ? `${parsedHint[1]}/${parsedHint[2]}` : null;

  return (
    <main className="flex-1 flex flex-col items-center justify-center px-4 py-20 min-h-[100dvh]">
      <div className="w-full max-w-xl space-y-14 text-center">
        {/* Hero */}
        <div className="space-y-5">
          <h1 className="text-5xl sm:text-7xl font-semibold tracking-tighter leading-[1.05]">
            Yatko
          </h1>
          <p className="text-base sm:text-lg text-muted leading-relaxed max-w-md mx-auto tracking-normal">
            Clean download links for any public GitHub repo so that you don't have to called a <a target="_blank" rel="noopener noreferrer" href="https://www.reddit.com/r/github/s/7YaS7nTVup" className="font-medium text-fg-brand hover:underline">"Smelly Nerd"</a> anymore
          </p>
        </div>

        {/* Search */}
        <div className="space-y-4">
          <form onSubmit={handleSubmit} className="space-y-2">
            <div className="flex gap-2">
              <input
                type="text"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="owner/repo or github.com/owner/repo"
                className="flex-1 px-4 py-3 rounded-xl border border-border bg-surface text-base placeholder:text-muted/60 focus:outline-none focus:ring-2 focus:ring-foreground/15 transition-[box-shadow,border-color] duration-200"
              />
              <button
                type="submit"
                className="px-6 py-3 rounded-xl bg-foreground text-background font-medium text-base hover:opacity-90 active:scale-[0.98] transition-[opacity,transform] duration-150"
              >
                Go
              </button>
            </div>
            {hintSlug && (
              <p className="text-xs text-muted text-left pl-1">
                Press Go to view downloads for <span className="font-mono">{hintSlug}</span>
              </p>
            )}
          </form>

          {/* Example repos */}
          <div className="space-y-2.5 text-left">
            <p className="text-xs text-muted font-medium">Try one</p>
            <div className="flex flex-wrap gap-2">
              {EXAMPLES.map((slug) => (
                <button
                  key={slug}
                  type="button"
                  onClick={() => navigate(slug)}
                  className="px-3 py-1.5 rounded-lg text-sm bg-foreground/[0.04] hover:bg-foreground/[0.08] active:scale-[0.98] transition-[background-color,transform] duration-150 font-mono"
                >
                  {slug}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* How it works */}
        <div className="space-y-6 text-left">
          <h2 className="text-lg font-semibold tracking-tight text-center">How it works</h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="rounded-xl border border-border bg-surface/60 p-5 space-y-2">
              <p className="text-sm font-medium tracking-tight">Direct download</p>
              <p className="text-xs text-muted/80 font-mono break-all">yatko.app/dl/owner/repo</p>
              <p className="text-xs text-muted leading-relaxed">
                Detects the user&apos;s platform and redirects straight to the right binary.
              </p>
            </div>
            <div className="rounded-xl border border-border bg-surface/60 p-5 space-y-2">
              <p className="text-sm font-medium tracking-tight">Landing page</p>
              <p className="text-xs text-muted/80 font-mono break-all">yatko.app/owner/repo</p>
              <p className="text-xs text-muted leading-relaxed">
                Same shape as your GitHub URL — swap the domain and it just works.
              </p>
            </div>
            <div className="rounded-xl border border-border bg-surface/60 p-5 space-y-2">
              <p className="text-sm font-medium tracking-tight">Version badge</p>
              <p className="text-xs text-muted/80 font-mono break-all">yatko.app/badge/owner/repo</p>
              <p className="text-xs text-muted leading-relaxed">
                Dynamic SVG badge showing the current release version for your README.
              </p>
            </div>
            <div className="rounded-xl border border-border bg-surface/60 p-5 space-y-2">
              <p className="text-sm font-medium tracking-tight">Link API</p>
              <p className="text-xs text-muted/80 font-mono break-all">yatko.app/api/link/owner/repo</p>
              <p className="text-xs text-muted leading-relaxed">
                Returns JSON with the resolved download URL - for CI pipelines and scripts.
              </p>
            </div>
          </div>
        </div>
      </div>
    </main>
  );
}
