"use client";

import Image from "next/image";
import Link from "next/link";
import { useRepoSearch } from "./use-repo-search";

const compactStars = new Intl.NumberFormat("en", {
  notation: "compact",
  maximumFractionDigits: 1,
});

export function HomeSearchForm({
  onNavigate,
  onNavigating,
}: {
  onNavigate: (owner: string, repo: string) => void;
  onNavigating: () => void;
}) {
  const {
    input,
    setInput,
    suggestions,
    loading,
    activeIndex,
    setActiveIndex,
    listId,
    blurTimer,
    setOpen,
    handleSubmit,
    handleKeyDown,
    hintSlug,
    showList,
  } = useRepoSearch(onNavigate);

  return (
    <form onSubmit={handleSubmit} className="space-y-2">
      <div className="relative">
        <div className="flex gap-2">
          <div className="relative flex-1">
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
                blurTimer.current = setTimeout(() => setOpen(false), 120);
              }}
              enterKeyHint="go"
              role="combobox"
              aria-expanded={showList}
              aria-controls={listId}
              aria-autocomplete="list"
              aria-activedescendant={activeIndex >= 0 ? `${listId}-opt-${activeIndex}` : undefined}
              placeholder="Search repos or paste owner/repo"
              autoComplete="off"
              spellCheck={false}
              className="w-full px-4 py-3 pr-10 rounded-xl border border-border bg-surface text-base placeholder:text-muted/60 focus:outline-none focus:ring-2 focus:ring-foreground/15 transition-[box-shadow,border-color] duration-200"
            />
            {loading && (
              <span
                aria-hidden
                className="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 rounded-full border-2 border-muted/30 border-t-muted animate-spin"
              />
            )}
          </div>
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
                  <Link
                    href={`/p/${item.owner}/${item.repo}`}
                    prefetch
                    className={`w-full flex items-start gap-3 px-4 py-3 text-left transition-colors duration-100 ${
                      active ? "bg-foreground/[0.08]" : "hover:bg-foreground/[0.04]"
                    }`}
                    onMouseEnter={() => setActiveIndex(i)}
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={onNavigating}
                  >
                    <Image
                      src={item.avatar_url || `https://github.com/${item.owner}.png?size=64`}
                      alt=""
                      width={32}
                      height={32}
                      unoptimized
                      className="mt-0.5 size-8 rounded-lg bg-foreground/[0.06] shrink-0"
                    />
                    <span className="min-w-0 flex-1 space-y-0.5">
                      <span className="block font-mono text-sm tracking-normal truncate">{slug}</span>
                      {item.description && (
                        <span className="block text-xs text-muted leading-snug line-clamp-2">
                          {item.description}
                        </span>
                      )}
                    </span>
                    <span className="shrink-0 text-xs text-muted tabular-nums pt-0.5">
                      ★ {compactStars.format(item.stars)}
                    </span>
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </div>
      {hintSlug && !showList && (
        <p className="text-xs text-muted text-left pl-1">
          Press Enter or Go to view downloads for <span className="font-mono">{hintSlug}</span>
        </p>
      )}
    </form>
  );
}
