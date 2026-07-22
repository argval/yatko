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

export function parseInput(value: string) {
  return value.match(/(?:github\.com\/)?([a-zA-Z0-9._-]+)\/([a-zA-Z0-9._-]+)\/?$/);
}

function normalizeQuery(q: string) {
  return q.trim().toLowerCase();
}

function matchesQuery(item: SearchItem, q: string) {
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
  const router = useRouter();
  const listId = useId();
  const blurTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const cacheRef = useRef<Map<string, SearchItem[]>>(new Map());
  const suggestionsRef = useRef<SearchItem[]>([]);

  function cacheSet(query: string, items: SearchItem[]) {
    const cache = cacheRef.current;
    if (cache.has(query)) cache.delete(query);
    cache.set(query, items);
    while (cache.size > CLIENT_CACHE_MAX) {
      const oldest = cache.keys().next().value;
      if (oldest === undefined) break;
      cache.delete(oldest);
    }
  }

  // Prefetch the highlighted suggestion (and typed owner/repo) so Enter/Go
  // often hits a warm RSC payload.
  useEffect(() => {
    if (activeIndex >= 0 && activeIndex < suggestions.length) {
      const item = suggestions[activeIndex];
      router.prefetch(`/p/${item.owner}/${item.repo}`);
      return;
    }
    const match = parseInput(input);
    if (match) router.prefetch(`/p/${match[1]}/${match[2]}`);
  }, [activeIndex, suggestions, input, router]);

  function go() {
    if (activeIndex >= 0 && activeIndex < suggestions.length) {
      const item = suggestions[activeIndex];
      onNavigate(item.owner, item.repo);
      return;
    }
    const match = parseInput(input);
    if (match) onNavigate(match[1], match[2]);
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
    const trimmed = normalizeQuery(query);
    if (trimmed.length < SEARCH_MIN_LEN) {
      applySuggestions([], false);
      setLoading(false);
      setOpen(false);
      return;
    }

    const cached = cacheRef.current.get(trimmed);
    if (cached) {
      applySuggestions(cached);
      setLoading(false);
      return;
    }

    // Instant feedback: narrow whatever we already have while the request runs.
    const optimistic = optimisticFilter(suggestionsRef.current, trimmed);
    if (optimistic.length > 0) {
      applySuggestions(optimistic);
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
      cacheSet(trimmed, items);
      applySuggestions(items);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      // Keep whatever is on screen.
    } finally {
      if (abortRef.current === ctrl) setLoading(false);
    }
  });

  useEffect(() => {
    const q = normalizeQuery(input);
    // Synchronous cache hit — no debounce wait.
    if (q.length >= SEARCH_MIN_LEN) {
      const cached = cacheRef.current.get(q);
      if (cached) {
        applySuggestions(cached);
        setLoading(false);
        return;
      }
      const optimistic = optimisticFilter(suggestionsRef.current, q);
      if (optimistic.length > 0) applySuggestions(optimistic);
    } else if (q.length < SEARCH_MIN_LEN) {
      applySuggestions([], false);
      setOpen(false);
      setLoading(false);
    }

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

  return {
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
  };
}
