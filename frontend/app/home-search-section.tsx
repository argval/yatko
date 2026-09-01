"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { HomeSearchForm } from "./home-search";
import { HomeExamples } from "./home-examples";
import { ReleaseLoading } from "./p/[owner]/[repo]/release-loading";

export function HomeSearchSection() {
  const router = useRouter();
  const [navigating, setNavigating] = useState(false);

  function navigate(owner: string, repo: string) {
    setNavigating(true);
    router.push(`/${owner}/${repo}`);
  }

  if (navigating) return <ReleaseLoading />;

  return (
    <>
      <HomeSearchForm onNavigate={navigate} onStartNavigating={() => setNavigating(true)} />
      <HomeExamples onNavigate={() => setNavigating(true)} />
    </>
  );
}
