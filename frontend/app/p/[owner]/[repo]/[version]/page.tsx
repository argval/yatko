import type { Metadata } from "next";
import { ReleasePageBody } from "../release-page";
import { getRelease, getReleases, getChecksums } from "../page";

type Props = {
  params: Promise<{ owner: string; repo: string; version: string }>;
};

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { owner, repo, version } = await params;
  return {
    title: `${repo} ${version} - Download | Yoink`,
    description: `Download ${owner}/${repo} version ${version}`,
  };
}

export default async function VersionedReleasePage({ params }: Props) {
  const { owner, repo, version } = await params;
  const release = await getRelease(owner, repo, version);
  const [releases, checksums] = await Promise.all([
    getReleases(owner, repo),
    getChecksums(release.assets),
  ]);
  return (
    <ReleasePageBody owner={owner} repo={repo} release={release} releases={releases} checksums={checksums} />
  );
}
