"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ReleaseLoading } from "./p/[owner]/[repo]/release-loading";
import { HomeSearchForm } from "./home-search";
import { HomeExamples } from "./home-examples";
import { HomeHowItWorks } from "./home-how-it-works";

export default function Home() {
  const [navigating, setNavigating] = useState(false);
  const router = useRouter();

  function navigate(owner: string, repo: string) {
    // Paint loading UI on this page immediately — route loading.tsx only
    // appears after the soft navigation flight starts, which can lag a beat.
    setNavigating(true);
    router.push(`/p/${owner}/${repo}`);
  }

  if (navigating) {
    return <ReleaseLoading />;
  }

  return (
    <main className="flex-1 flex flex-col items-center justify-center px-4 py-20 min-h-[100dvh]">
      <div className="w-full max-w-xl space-y-14 text-center">
        <div className="space-y-5">
          <h1 className="text-5xl sm:text-7xl font-semibold tracking-tighter leading-[1.05]">Yatko</h1>
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

        <div className="space-y-4">
          <HomeSearchForm onNavigate={navigate} onNavigating={() => setNavigating(true)} />
          <HomeExamples onNavigating={() => setNavigating(true)} />
        </div>

        <HomeHowItWorks />
      </div>
    </main>
  );
}
