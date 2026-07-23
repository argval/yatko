import type { Metadata } from "next";
import { ReleasePageBody } from "./release-page";
import { ReleaseError } from "./release-error";
import { NotFoundCard } from "./not-found";
import { getChecksums, getReadme, getRelease, getReleases, platformFromRequest } from "./backend";

type Props = {
  params: Promise<{ owner: string; repo: string }>;
};

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { owner, repo } = await params;
  return {
    title: `${repo} - Download | Yatko`,
    description: `Download the latest release of ${owner}/${repo}`,
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
  const [platform, arch] = await platformFromRequest();
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
      initialPlatform={platform}
      initialArch={arch}
      readmePromise={readmePromise}
      releasesPromise={releasesPromise}
      checksumsPromise={checksumsPromise}
    />
  );
}
