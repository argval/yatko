"use client";

import { useRouter } from "next/navigation";
import { HomeSearchForm } from "./home-search";

export function HomeSearchSection() {
  const router = useRouter();
  function navigate(owner: string, repo: string) {
    router.push(`/p/${owner}/${repo}`);
  }
  return <HomeSearchForm onNavigate={navigate} />;
}
