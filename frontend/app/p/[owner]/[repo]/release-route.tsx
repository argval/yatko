import type { Metadata } from "next";
import { ReleasePageBody } from "./release-page";
import { getChecksums, getReadme, getRelease, getReleases } from "./backend";
import { ReleaseError } from "./release-error";
import { NotFoundCard } from "./not-found";

export function releasePageMetadata({
  owner,
  repo,
  version,
}: {
  owner: string;
  repo: string;
  version?: string;
}): Metadata {
  const title = version
    ? `${repo} ${version} - Download | Yatko`
    : `${repo} - Download | Yatko`;
  const description = version
    ? `Download ${owner}/${repo} version ${version}`
    : `Download the latest release of ${owner}/${repo}`;
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

/** Shared load + error branching for latest and version-pinned release pages. */
export async function renderReleasePage({
  owner,
  repo,
  version,
}: {
  owner: string;
  repo: string;
  version?: string;
}) {
  const result = await getRelease(owner, repo, version);
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
