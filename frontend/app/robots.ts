import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      // /p/* is an unbounded owner/repo space — crawlers mostly cause cold
      // Fluid invocations with little SEO value. Users reach release pages
      // via direct links (github.com → yatko.app URL swap).
      disallow: ["/api/", "/dl/", "/p/"],
    },
  };
}
