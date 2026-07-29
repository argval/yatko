import type { MetadataRoute } from "next";

/** Link-unfurl bots that need /p/* + opengraph-image to show preview cards. */
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
        disallow: ["/api/", "/dl/", "/p/"],
      },
      {
        // Social unfurlers must fetch /p/* HTML + opengraph-image so share
        // cards work. Keep /api and /dl blocked (no useful preview there).
        userAgent: SOCIAL_PREVIEW_BOTS,
        allow: ["/p/"],
        disallow: ["/api/", "/dl/"],
      },
    ],
  };
}
