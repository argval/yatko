"use client";

import {
  useEffect,
  useEffectEvent,
  useId,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from "react";
import { useRouter } from "next/navigation";

const EXAMPLES = ["cli/cli", "neovim/neovim", "astral-sh/uv", "BurntSushi/ripgrep"];
const SEARCH_DEBOUNCE_MS = 250;
const SEARCH_MIN_LEN = 2;

type SearchItem = {
  owner: string;
  repo: string;
  description: string;
  stars: number;
  avatar_url: string;
};

function parseInput(value: string) {
  return value.match(/(?:github\.com\/)?([a-zA-Z0-9._-]+)\/([a-zA-Z0-9._-]+)\/?$/);
}

function formatStars(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, "")}m`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1).replace(/\.0$/, "")}k`;
  return String(n);
}

export default function Home() {
  const [input, setInput] = useState("");
  const [suggestions, setSuggestions] = useState<SearchItem[]>([]);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const router = useRouter();
  const listId = useId();
  const blurTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  function navigate(owner: string, repo: string) {
    router.push(`/p/${owner}/${repo}`);
  }

  function go() {
    if (activeIndex >= 0 && activeIndex < suggestions.length) {
      const item = suggestions[activeIndex];
      navigate(item.owner, item.repo);
      return;
    }
    const match = parseInput(input);
    if (match) navigate(match[1], match[2]);
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    go();
  }

  const runSearch = useEffectEvent(async (query: string) => {
    abortRef.current?.abort();
    const trimmed = query.trim();
    if (trimmed.length < SEARCH_MIN_LEN) {
      setSuggestions([]);
      setLoading(false);
      setActiveIndex(-1);
      return;
    }

    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    try {
      const res = await fetch(`/api/search?q=${encodeURIComponent(trimmed)}`, {
        signal: ctrl.signal,
      });
      if (!res.ok) {
        setSuggestions([]);
        setActiveIndex(-1);
        return;
      }
      const data = (await res.json()) as { items?: SearchItem[] };
      setSuggestions(Array.isArray(data.items) ? data.items : []);
      setActiveIndex(-1);
      setOpen(true);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      setSuggestions([]);
      setActiveIndex(-1);
    } finally {
      if (abortRef.current === ctrl) setLoading(false);
    }
  });

  useEffect(() => {
    const t = setTimeout(() => {
      void runSearch(input);
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [input]);

  useEffect(() => {
    return () => {
      abortRef.current?.abort();
      if (blurTimer.current) clearTimeout(blurTimer.current);
    };
  }, []);

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown") {
      if (!open || suggestions.length === 0) return;
      e.preventDefault();
      setActiveIndex((i) => (i + 1) % suggestions.length);
      return;
    }
    if (e.key === "ArrowUp") {
      if (!open || suggestions.length === 0) return;
      e.preventDefault();
      setActiveIndex((i) => (i <= 0 ? suggestions.length - 1 : i - 1));
      return;
    }
    if (e.key === "Escape") {
      setOpen(false);
      setActiveIndex(-1);
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault();
      go();
    }
  }

  const parsedHint = parseInput(input);
  const hintSlug = parsedHint ? `${parsedHint[1]}/${parsedHint[2]}` : null;
  const showList = open && (suggestions.length > 0 || loading);

  return (
    <main className="flex-1 flex flex-col items-center justify-center px-4 py-20 min-h-[100dvh]">
      <div className="w-full max-w-xl space-y-14 text-center">
        {/* Hero */}
        <div className="space-y-5">
          <h1 className="text-5xl sm:text-7xl font-semibold tracking-tighter leading-[1.05]">
            Yatko
          </h1>
          <p className="text-base sm:text-lg text-muted leading-relaxed max-w-md mx-auto tracking-normal">
            Clean download links for any public GitHub repo so that you don&apos;t have to called a{" "}
            <a
              target="_blank"
              rel="noopener noreferrer"
              href="https://www.reddit.com/r/github/s/7YaS7nTVup"
              className="font-medium text-fg-brand hover:underline"
            >
              &quot;Smelly Nerd&quot;
            </a>{" "}
            anymore
          </p>
        </div>

        {/* Search */}
        <div className="space-y-4">
          <form onSubmit={handleSubmit} className="space-y-2">
            <div className="relative">
              <div className="flex gap-2">
                <input
                  type="text"
                  value={input}
                  onChange={(e) => {
                    setInput(e.target.value);
                    setOpen(true);
                  }}
                  onKeyDown={handleKeyDown}
                  onFocus={() => {
                    if (blurTimer.current) clearTimeout(blurTimer.current);
                    if (suggestions.length > 0) setOpen(true);
                  }}
                  onBlur={() => {
                    // Delay so a mousedown on a suggestion can fire first.
                    blurTimer.current = setTimeout(() => setOpen(false), 120);
                  }}
                  enterKeyHint="go"
                  role="combobox"
                  aria-expanded={showList}
                  aria-controls={listId}
                  aria-autocomplete="list"
                  aria-activedescendant={
                    activeIndex >= 0 ? `${listId}-opt-${activeIndex}` : undefined
                  }
                  placeholder="Search repos or paste owner/repo"
                  autoComplete="off"
                  spellCheck={false}
                  className="flex-1 px-4 py-3 rounded-xl border border-border bg-surface text-base placeholder:text-muted/60 focus:outline-none focus:ring-2 focus:ring-foreground/15 transition-[box-shadow,border-color] duration-200"
                />
                <button
                  type="submit"
                  className="px-6 py-3 rounded-xl bg-foreground text-background font-medium text-base hover:opacity-90 active:scale-[0.98] transition-[opacity,transform] duration-150"
                >
                  Go
                </button>
              </div>

              {showList && (
                <ul
                  id={listId}
                  role="listbox"
                  className="absolute z-20 left-0 right-14 mt-2 max-h-80 overflow-auto rounded-xl border border-border bg-surface text-left shadow-[0_12px_40px_-16px_rgb(0_0_0/0.35)]"
                >
                  {loading && suggestions.length === 0 && (
                    <li className="px-4 py-3 text-sm text-muted">Searching…</li>
                  )}
                  {suggestions.map((item, i) => {
                    const slug = `${item.owner}/${item.repo}`;
                    const active = i === activeIndex;
                    return (
                      <li key={slug} role="option" aria-selected={active} id={`${listId}-opt-${i}`}>
                        <button
                          type="button"
                          className={`w-full flex items-start gap-3 px-4 py-3 text-left transition-colors duration-100 ${
                            active ? "bg-foreground/[0.08]" : "hover:bg-foreground/[0.04]"
                          }`}
                          onMouseEnter={() => setActiveIndex(i)}
                          onMouseDown={(e) => e.preventDefault()}
                          onClick={() => navigate(item.owner, item.repo)}
                        >
                          <img
                            src={item.avatar_url || `https://github.com/${item.owner}.png?size=64`}
                            alt=""
                            width={32}
                            height={32}
                            className="mt-0.5 size-8 rounded-lg bg-foreground/[0.06] shrink-0"
                          />
                          <span className="min-w-0 flex-1 space-y-0.5">
                            <span className="block font-mono text-sm tracking-normal truncate">
                              {slug}
                            </span>
                            {item.description && (
                              <span className="block text-xs text-muted leading-snug line-clamp-2">
                                {item.description}
                              </span>
                            )}
                          </span>
                          <span className="shrink-0 text-xs text-muted tabular-nums pt-0.5">
                            ★ {formatStars(item.stars)}
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
            {hintSlug && !showList && (
              <p className="text-xs text-muted text-left pl-1">
                Press Enter or Go to view downloads for{" "}
                <span className="font-mono">{hintSlug}</span>
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
                  onClick={() => navigate(...(slug.split("/") as [string, string]))}
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
