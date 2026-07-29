import type { Metadata } from "next";
import {
  releasePageMetadata,
  renderReleasePage,
} from "./release-route";

type Props = {
  params: Promise<{ owner: string; repo: string }>;
};

/** Cache the release page HTML/RSC payload for an hour. Platform is detected
 *  client-side so we avoid headers()-forced dynamic rendering (which made every
 *  hit a Fluid invocation with private no-store responses). Must be a numeric
 *  literal — Next rejects imported segment config values. Keep aligned with
 *  BACKEND_FETCH_REVALIDATE_SECONDS / Redis soft TTL. */
export const revalidate = 3600;

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { owner, repo } = await params;
  return releasePageMetadata({ owner, repo });
}

export default async function ReleasePage({ params }: Props) {
  const { owner, repo } = await params;
  return renderReleasePage({ owner, repo });
}
