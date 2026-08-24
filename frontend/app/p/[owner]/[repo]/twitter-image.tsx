import { EXAMPLE_REPOS } from "@/lib/example-repos";

export { default, alt, size, contentType } from "./opengraph-image";

/** Literal required for Next segment config static analysis — keep aligned
 *  with opengraph-image / release page. */
export const revalidate = 3600;

export function generateStaticParams() {
  return EXAMPLE_REPOS.map(({ owner, repo }) => ({ owner, repo }));
}
