import type { Metadata } from "next";
import {
  releasePageMetadata,
  renderReleasePage,
} from "../release-route";

type Props = {
  params: Promise<{ owner: string; repo: string; version: string }>;
};

/** See parent route: keep versioned pages cacheable without reading request headers.
 *  Literal required for Next segment config static analysis. */
export const revalidate = 3600;

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { owner, repo, version } = await params;
  return releasePageMetadata({ owner, repo, version });
}

export default async function VersionedReleasePage({ params }: Props) {
  const { owner, repo, version } = await params;
  return renderReleasePage({ owner, repo, version });
}
