import type { MetadataRoute } from "next";
import { SITE_URL } from "@/lib/site";

/** Crawlable marketing surface only — release pages are unbounded (`/p/` and
 *  the github-swap `/:owner/:repo` rewrite) and stay out of the sitemap. */
export default function sitemap(): MetadataRoute.Sitemap {
  const lastModified = new Date();
  return [
    {
      url: SITE_URL,
      lastModified,
      changeFrequency: "weekly",
      priority: 1,
    },
    {
      url: `${SITE_URL}/privacy`,
      lastModified,
      changeFrequency: "yearly",
      priority: 0.3,
    },
  ];
}
