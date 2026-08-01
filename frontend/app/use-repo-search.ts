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

export type SearchItem = {
  owner: string;
  repo: string;
  description: string;
  stars: number;
  avatar_url: string;
};

// Keep debounce short — GitHub Search latency dominates; we optimistic-filter
// cached prefix results while the network request is in flight.
const SEARCH_DEBOUNCE_MS = 100;
const SEARCH_MIN_LEN = 2;
const CLIENT_CACHE_MAX = 40;

const GITHUB_REPO_URL_RE =
  /^(?:https?:\/\/)?(?:www\.)?github\.com\/([a-zA-Z0-9._-]+)\/([a-zA-Z0-9._-]+)/i;
const GITHUB_OWNER_URL_RE =
  /^(?:https?:\/\/)?(?:www\.)?github\.com\/([a-zA-Z0-9._-]+)\/?$/i;
const OWNER_REPO_RE = /^([a-zA-Z0-9._-]+)\/([a-zA-Z0-9._-]+)\/?$/;

/** Extract owner/repo from a slug or pasted GitHub URL. */
export function parseInput(value: string): { owner: string; repo: string } | null {
  const trimmed = value.trim();
  if (!trimmed) return null;

  const urlMatch = trimmed.match(GITHUB_REPO_URL_RE);
  if (urlMatch) {
    const owner = urlMatch[1];
    const repo = urlMatch[2].replace(/\.git$/i, "");
    if (owner && repo) return { owner, repo };
    return null;
  }

  // Bare github.com/owner (no repo) is not a slug — leave it for normalizeSearchQuery.
  if (GITHUB_OWNER_URL_RE.test(trimmed)) return null;

  const slugMatch = trimmed.match(OWNER_REPO_RE);
  if (slugMatch) return { owner: slugMatch[1], repo: slugMatch[2] };
  return null;
}

/** Stable autocomplete query: trim, lowercase, collapse GitHub URLs. */
export function normalizeSearchQuery(q: string) {
  const trimmed = q.trim();
  const ownerOnly = trimmed.match(GITHUB_OWNER_URL_RE);
  // Owner URL → bare login (same as typing the owner; backend dual-searches).
  if (ownerOnly?.[1]) return ownerOnly[1].toLowerCase();
  const parsed = parseInput(q);
  if (parsed) return `${parsed.owner}/${parsed.repo}`.toLowerCase();
  const lower = trimmed.toLowerCase().replace(/\/$/, "");
  // Legacy sentinel from older clients → bare login.
  if (lower.startsWith("owner:")) {
    const owner = lower.slice("owner:".length);
    if (owner) return owner;
  }
  return lower;
}

/** Mirrors backend search.FilterItems: slug vs bare. */
export function matchesQuery(item: SearchItem, q: string): boolean {
  const slash = q.indexOf("/");
  if (slash >= 0) {
    const owner = q.slice(0, slash);
    const repo = q.slice(slash + 1);
    if (!owner || !repo) return false;
    if (item.owner.toLowerCase() !== owner) return false;
    return item.repo.toLowerCase().startsWith(repo);
  }
  const slug = `${item.owner}/${item.repo}`.toLowerCase();
  return slug.includes(q) || item.repo.toLowerCase().includes(q) || item.owner.toLowerCase().includes(q);
}

/** Filter a previous result set to items that still match the longer query. */
function optimisticFilter(items: SearchItem[], q: string) {
  if (!q) return [];
  return items.filter((item) => matchesQuery(item, q));
}

export function useRepoSearch(onNavigate: (owner: string, repo: string) => void) {
  const [input, setInput] = useState("");
  const [suggestions, setSuggestions] = useState<SearchItem[]>([]);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const [noResults, setNoResults] = useState(false);
  const router = useRouter();
  const listId = useId();
  const blurTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const cacheRef = useRef<Map<string, SearchItem[]>>(new Map());
  const suggestionsRef = useRef<SearchItem[]>([]);

  function cacheSet(query: string, items: SearchItem[]) {
    // Don't persist empty hits — they make the dropdown flash "nothing" and
    // skip the next network attempt via the sync cache path.
    if (items.length === 0) return;
    const cache = cacheRef.current;
    if (cache.has(query)) cache.delete(query);
    cache.set(query, items);
    while (cache.size > CLIENT_CACHE_MAX) {
      const oldest = cache.keys().next().value;
      if (oldest === undefined) break;
      cache.delete(oldest);
    }
  }

  // Prefetch only the row the user is about to open (highlighted, or an
  // explicit owner/repo paste). Prefetching the whole suggestion list was
  // blocking keystroke paint (~3s) because each release page is a heavy RSC.
  const activeSlug =
    activeIndex >= 0 && activeIndex < suggestions.length
      ? `${suggestions[activeIndex].owner}/${suggestions[activeIndex].repo}`
      : null;
  const typedMatch = parseInput(input);
  const typedSlug = typedMatch ? `${typedMatch.owner}/${typedMatch.repo}` : null;
  const prefetchSlug = activeSlug ?? typedSlug;

  useEffect(() => {
    if (!prefetchSlug) return;
    router.prefetch(`/${prefetchSlug}`);
  }, [prefetchSlug, router]);

  function go() {
    if (activeIndex >= 0 && activeIndex < suggestions.length) {
      const item = suggestions[activeIndex];
      onNavigate(item.owner, item.repo);
      return;
    }
    const match = parseInput(input);
    if (match) onNavigate(match.owner, match.repo);
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    go();
  }

  function applySuggestions(items: SearchItem[], show = true) {
    suggestionsRef.current = items;
    setSuggestions(items);
    setActiveIndex(-1);
    if (show) setOpen(true);
  }

  const runSearch = useEffectEvent(async (query: string) => {
    abortRef.current?.abort();
    const trimmed = normalizeSearchQuery(query);
    if (trimmed.length < SEARCH_MIN_LEN) {
      applySuggestions([], false);
      setNoResults(false);
      setLoading(false);
      setOpen(false);
      return;
    }

    const cached = cacheRef.current.get(trimmed);
    if (cached && cached.length > 0) {
      applySuggestions(cached);
      setNoResults(false);
      setLoading(false);
      // Still refresh in the background? Skip — warm cache is fine for autocomplete.
      return;
    }

    // Instant feedback: narrow whatever we already have while the request runs.
    const optimistic = optimisticFilter(suggestionsRef.current, trimmed);
    if (optimistic.length > 0) {
      applySuggestions(optimistic);
      setNoResults(false);
    }

    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    try {
      const res = await fetch(`/api/search?q=${encodeURIComponent(trimmed)}`, {
        signal: ctrl.signal,
      });
      if (!res.ok) {
        // Keep optimistic/previous suggestions on transient failures.
        return;
      }
      const data = (await res.json()) as { items?: SearchItem[] };
      const items = Array.isArray(data.items) ? data.items : [];
      // Refuse to commit if a newer search superseded this request.
      if (abortRef.current !== ctrl || ctrl.signal.aborted) return;
      cacheSet(trimmed, items);
      applySuggestions(items);
      setNoResults(items.length === 0);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      // Keep whatever is on screen.
    } finally {
      if (abortRef.current === ctrl) setLoading(false);
    }
  });

  useEffect(() => {
    const q = normalizeSearchQuery(input);
    // Synchronous cache hit — no debounce wait.
    if (q.length >= SEARCH_MIN_LEN) {
      const cached = cacheRef.current.get(q);
      if (cached && cached.length > 0) {
        applySuggestions(cached);
        setNoResults(false);
        setLoading(false);
        // Fall through to debounce so a keystroke still can refresh later paths;
        // warm non-empty cache is shown immediately above.
      } else {
        const optimistic = optimisticFilter(suggestionsRef.current, q);
        if (optimistic.length > 0) {
          applySuggestions(optimistic);
          setNoResults(false);
        }
      }
    } else {
      abortRef.current?.abort();
      abortRef.current = null;
      applySuggestions([], false);
      setNoResults(false);
      setOpen(false);
      setLoading(false);
    }

    const t = setTimeout(() => {
      void runSearch(input);
    }, SEARCH_DEBOUNCE_MS);
    return () => {
      // Only cancel the pending debounce — do not abort an in-flight fetch
      // here. Aborting on every keystroke cleanup raced with completions and
      // made the list flash empty. runSearch aborts the prior request itself.
      clearTimeout(t);
    };
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
  const hintSlug = parsedHint ? `${parsedHint.owner}/${parsedHint.repo}` : null;
  const queryReady = normalizeSearchQuery(input).length >= SEARCH_MIN_LEN;
  const showList = open && queryReady && (suggestions.length > 0 || loading || noResults);

  return {
    input,
    setInput,
    suggestions,
    loading,
    noResults,
    activeIndex,
    setActiveIndex,
    listId,
    blurTimer,
    setOpen,
    handleSubmit,
    handleKeyDown,
    hintSlug,
    showList,
  };
}
