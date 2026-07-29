import type { Metadata } from "next";
import { ReleasePageBody } from "./release-page";
import { ReleaseError } from "./release-error";
import { NotFoundCard } from "./not-found";
import { getChecksums, getReadme, getRelease, getReleases } from "./backend";

type Props = {
  params: Promise<{ owner: string; repo: string }>;
};

/** Cache the release page HTML/RSC payload for an hour. Platform is detected
 *  client-side so we avoid headers()-forced dynamic rendering (which made every
 *  hit a Fluid invocation with private no-store responses). */
export const revalidate = 3600;

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { owner, repo } = await params;
  const title = `${repo} - Download | Yatko`;
  const description = `Download the latest release of ${owner}/${repo}`;
  return {
    title,
    description,
    openGraph: {
      title,
      description,
      type: "website",
    },
    twitter: {
      card: "summary_large_image",
      title,
      description,
    },
  };
}

export default async function ReleasePage({ params }: Props) {
  const { owner, repo } = await params;
  const result = await getRelease(owner, repo);
  if (!result.ok) {
    return result.notFound ? (
      <NotFoundCard owner={owner} repo={repo} repoExists={result.repoExists} />
    ) : (
      <ReleaseError message={result.message} />
    );
  }
  // README streams in separately; releases are usually already on the release
  // payload (backend embeds them). Checksums stay non-blocking via Suspense.
  const readmePromise = getReadme(owner, repo);
  const releasesPromise = Array.isArray(result.data.releases)
    ? Promise.resolve(result.data.releases)
    : getReleases(owner, repo);
  const checksumsPromise = getChecksums(result.data.assets);
  return (
    <ReleasePageBody
      owner={owner}
      repo={repo}
      release={result.data}
      readmePromise={readmePromise}
      releasesPromise={releasesPromise}
      checksumsPromise={checksumsPromise}
    />
  );
}
