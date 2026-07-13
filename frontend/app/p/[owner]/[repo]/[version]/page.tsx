import type { Metadata } from "next";
import { ReleasePageBody } from "../release-page";
import { getRelease } from "../page";

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
  return <ReleasePageBody owner={owner} repo={repo} release={release} />;
}
