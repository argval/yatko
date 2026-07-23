"use client";

import { createContext, useContext, type ReactNode } from "react";
import { useRouter } from "next/navigation";

type ReleaseNav = {
  navigate: (href: string) => void;
  prefetch: (href: string) => void;
};

const ReleaseNavContext = createContext<ReleaseNav | null>(null);

export function ReleaseNavShell({ children }: { children: ReactNode }) {
  const router = useRouter();

  function navigate(href: string) {
    router.push(href);
  }

  function prefetch(href: string) {
    router.prefetch(href);
  }

  return (
    <ReleaseNavContext.Provider value={{ navigate, prefetch }}>{children}</ReleaseNavContext.Provider>
  );
}

export function useReleaseNav(): ReleaseNav {
  const ctx = useContext(ReleaseNavContext);
  if (!ctx) {
    throw new Error("useReleaseNav must be used within ReleaseNavShell");
  }
  return ctx;
}
