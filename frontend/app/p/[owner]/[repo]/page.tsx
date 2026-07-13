import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ReleasePageBody, type ReleaseData } from "./release-page";

const BACKEND_URL = process.env.BACKEND_URL || "http://localhost:8080";

type Props = {
  params: Promise<{ owner: string; repo: string }>;
};

export async function getRelease(owner: string, repo: string, version?: string): Promise<ReleaseData> {
  const path = version
    ? `/api/release/${owner}/${repo}/${version}`
    : `/api/release/${owner}/${repo}`;
  let res: Response;
  try {
    res = await fetch(`${BACKEND_URL}${path}`, { next: { revalidate: 300 } });
  } catch {
    throw new Error("Couldn't reach the download service. Try again in a moment.");
  }
  if (res.status === 404) notFound();
  if (res.status === 403) throw new Error("This repository is private or you don't have access.");
  if (res.status === 429) throw new Error("GitHub API rate limit exceeded. Try again in a minute.");
  if (!res.ok) throw new Error("Couldn't reach the download service. Try again in a moment.");
  return res.json();
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { owner, repo } = await params;
  return {
    title: `${repo} - Download | Yoink`,
    description: `Download the latest release of ${owner}/${repo}`,
  };
}

export default async function ReleasePage({ params }: Props) {
  const { owner, repo } = await params;
  const release = await getRelease(owner, repo);
  return <ReleasePageBody owner={owner} repo={repo} release={release} />;
}
