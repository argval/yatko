"use client";

import { useState, useEffect } from "react";

export type ReleaseSummary = {
  tag_name: string;
  name: string;
  published_at: string;
  prerelease: boolean;
};

// Module-level cache deduplicates fetches across components on the same page.
const cache = new Map<string, ReleaseSummary[]>();

function isReleaseList(data: unknown): data is ReleaseSummary[] {
  return Array.isArray(data);
}

export function useReleases(owner: string, repo: string) {
  const key = `${owner}/${repo}`;
  const cached = cache.get(key);
  const [releases, setReleases] = useState<ReleaseSummary[]>(
    isReleaseList(cached) ? cached : []
  );
  const [loading, setLoading] = useState(!isReleaseList(cached));

  useEffect(() => {
    const existing = cache.get(key);
    if (isReleaseList(existing)) {
      setReleases(existing);
      setLoading(false);
      return;
    }
    // Drop a poisoned cache entry (e.g. an error payload from an older client).
    cache.delete(key);

    const backendUrl = process.env.NEXT_PUBLIC_BACKEND_URL || "http://localhost:8080";
    let cancelled = false;

    fetch(`${backendUrl}/api/releases/${owner}/${repo}`)
      .then(async (r) => {
        const data: unknown = await r.json();
        if (!r.ok || !isReleaseList(data)) {
          throw new Error("Invalid releases response");
        }
        return data;
      })
      .then((data) => {
        if (cancelled) return;
        cache.set(key, data);
        setReleases(data);
        setLoading(false);
      })
      .catch(() => {
        if (cancelled) return;
        setReleases([]);
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [key, owner, repo]);

  return { releases, loading };
}
