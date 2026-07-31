import type { MetadataRoute } from "next";

/** Crawlable marketing surface only — release pages are unbounded (`/p/` and
 *  the github-swap `/:owner/:repo` rewrite) and stay out of the sitemap. */
export default function sitemap(): MetadataRoute.Sitemap {
  const base = "https://yatko.app";
  return [
    {
      url: base,
      changeFrequency: "weekly",
      priority: 1,
    },
    {
      url: `${base}/privacy`,
      changeFrequency: "yearly",
      priority: 0.3,
    },
  ];
}
