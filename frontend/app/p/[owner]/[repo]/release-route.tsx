import type { Metadata } from "next";
import { ReleasePageBody } from "./release-page";
import { getChecksums, getReadme, getRelease, getReleases, getRepoMeta } from "./backend";
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
    ? `Download ${owner}/${repo} ${version}`
    : `Download ${owner}/${repo}`;
  const description = version
    ? `Download ${owner}/${repo} ${version} on Yatko (yatko.app) — the right GitHub release asset for your OS and architecture.`
    : `Download the latest ${owner}/${repo} release on Yatko (yatko.app) — swap github.com for clean, platform-aware download links.`;
  // Latest releases use the github.com → yatko.app path. Version pins stay
  // under /p/ (no bare /:owner/:repo/:version rewrite).
  const canonicalPath = version
    ? `/p/${owner}/${repo}/${version}`
    : `/${owner}/${repo}`;
  // opengraph-image.tsx sits on the [repo] segment (sibling of [[...version]]),
  // so Next won't auto-inject og:image into the page head — set it explicitly.
  // Use the github-swap path (rewritten to /p/.../opengraph-image) so the image
  // URL is not under robots Disallow:/p/.
  const ogImage = {
    url: `/${owner}/${repo}/opengraph-image`,
    width: 1200,
    height: 630,
    alt: `${owner}/${repo} on Yatko`,
  };
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
      images: [ogImage],
    },
    twitter: {
      card: "summary_large_image",
      title,
      description,
      images: [ogImage.url],
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
      <ReleaseError
        message={result.message}
        owner={owner}
        repo={repo}
        version={version}
      />
    );
  }
  // Repo metadata, version list, and checksums are all non-critical.
  const repoMetaPromise = getRepoMeta(owner, repo);
  const releasesPromise = getReleases(owner, repo);
  const checksumsPromise = getChecksums(result.data.assets);
  return (
    <ReleasePageBody
      owner={owner}
      repo={repo}
      release={result.data}
      repoMetaPromise={repoMetaPromise}
      readmePromise={readmePromise}
      releasesPromise={releasesPromise}
      checksumsPromise={checksumsPromise}
    />
  );
}
