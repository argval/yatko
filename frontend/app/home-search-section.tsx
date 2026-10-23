"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { HomeSearchForm } from "./home-search";
import { HomeExamples } from "./home-examples";
import { ReleaseLoading } from "./p/[owner]/[repo]/release-loading";
import { prefetchLatestPick } from "./p/[owner]/[repo]/link-decision";
import { usePlatform } from "./p/[owner]/[repo]/use-platform";

export function HomeSearchSection() {
  const router = useRouter();
  const [navigating, setNavigating] = useState(false);
  // Detect the platform early so the click can start the download pick.
  const detected = usePlatform();

  // Runs the download pick in parallel with the release page render.
  function startNavigating(owner: string, repo: string) {
    setNavigating(true);
    if (detected?.platform) prefetchLatestPick(owner, repo, detected.platform, detected.arch, detected.context);
  }

  function navigate(owner: string, repo: string) {
    startNavigating(owner, repo);
    router.push(`/${owner}/${repo}`);
  }

  if (navigating) return <ReleaseLoading />;

  return (
    <>
      <HomeSearchForm onNavigate={navigate} onStartNavigating={startNavigating} />
      <HomeExamples onNavigate={startNavigating} />
    </>
  );
}
