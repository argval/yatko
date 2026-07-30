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
  // Latest releases use the github.com → yatko.app path. Version pins stay
  // under /p/ (no bare /:owner/:repo/:version rewrite).
  const canonicalPath = version
    ? `/p/${owner}/${repo}/${version}`
    : `/${owner}/${repo}`;
  return {
    title,
    description,
    alternates: {
      canonical: canonicalPath,
    },
    openGraph: {
      title,
      description,
      type: "website",
      siteName: "Yatko",
      url: canonicalPath,
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
  const releasePromise = getRelease(owner, repo, version);
  // README only needs owner/repo — overlap with release fetch.
  const readmePromise = getReadme(owner, repo);

  const result = await releasePromise;
  if (!result.ok) {
    // readmePromise may still settle unused — fine on error paths.
    return result.notFound ? (
      <NotFoundCard owner={owner} repo={repo} repoExists={result.repoExists} />
    ) : (
      <ReleaseError message={result.message} />
    );
  }
  // Releases are usually already on the release payload (backend embeds them).
  // Checksums stay non-blocking via Suspense and need assets from the release.
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
