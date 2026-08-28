import type { MetadataRoute } from "next";
import { SITE_URL } from "@/lib/site";

/** Link-unfurl bots that need release pages + OG images for preview cards. */
const SOCIAL_PREVIEW_BOTS = [
  "Twitterbot",
  "facebookexternalhit",
  "LinkedInBot",
  "Slackbot",
  "Slackbot-LinkExpanding",
  "Discordbot",
  "TelegramBot",
  "WhatsApp",
  "SkypeUriPreview",
  "Applebot", // Messages / Spotlight link previews
];

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        // /p/* is an unbounded owner/repo space — general crawlers mostly
        // cause cold Fluid invocations with little SEO value. Users reach
        // release pages via direct links (github.com → yatko.app URL swap).
        disallow: ["/api/", "/code/", "/dl/", "/p/"],
      },
      {
        // Social unfurlers need the github-swap URL (/:owner/:repo), /p/*
        // HTML, and opengraph/twitter images. Keep direct/API URLs blocked.
        userAgent: SOCIAL_PREVIEW_BOTS,
        allow: ["/"],
        disallow: ["/api/", "/code/", "/dl/"],
      },
    ],
    sitemap: `${SITE_URL}/sitemap.xml`,
    host: SITE_URL.replace(/^https?:\/\//, ""),
  };
}
