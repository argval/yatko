"use client";

import { createContext, useContext, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { ReleaseLoading } from "./release-loading";

type ReleaseNav = {
  navigate: (href: string) => void;
  prefetch: (href: string) => void;
};

const ReleaseNavContext = createContext<ReleaseNav | null>(null);

export function ReleaseNavShell({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [navigating, setNavigating] = useState(false);

  function navigate(href: string) {
    setNavigating(true);
    router.push(href);
  }

  function prefetch(href: string) {
    router.prefetch(href);
  }

  if (navigating) {
    return <ReleaseLoading />;
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
