import type { Metadata } from "next";
import { ReleasePageBody } from "../release-page";
import { getRelease, getReleases, getChecksums } from "../page";
import { ReleaseError } from "../release-error";

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
  if (!result.ok) return <ReleaseError message={result.message} />;
  const releasesPromise = getReleases(owner, repo);
  const checksumsPromise = getChecksums(result.data.assets);
  return (
    <ReleasePageBody
      owner={owner}
      repo={repo}
      release={result.data}
      releasesPromise={releasesPromise}
      checksumsPromise={checksumsPromise}
    />
  );
}
