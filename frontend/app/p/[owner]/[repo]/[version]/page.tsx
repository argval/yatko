import type { Metadata } from "next";
import { ReleasePageBody } from "../release-page";
import { getChecksums, getReadme, getRelease, getReleases, platformFromRequest } from "../backend";
import { ReleaseError } from "../release-error";
import { NotFoundCard } from "../not-found";

type Props = {
  params: Promise<{ owner: string; repo: string; version: string }>;
};

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { owner, repo, version } = await params;
  return {
    title: `${repo} ${version} - Download | Yatko`,
    description: `Download ${owner}/${repo} version ${version}`,
  };
}

export default async function VersionedReleasePage({ params }: Props) {
  const { owner, repo, version } = await params;
  const result = await getRelease(owner, repo, version);
  if (!result.ok) {
    return result.notFound ? (
      <NotFoundCard owner={owner} repo={repo} repoExists={result.repoExists} />
    ) : (
      <ReleaseError message={result.message} />
    );
  }
  const [platform, arch] = await platformFromRequest();
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
