import type { NextConfig } from "next";
import { withBotId } from "botid/next/config";

const backendURL = process.env.BACKEND_URL || "http://localhost:8080";

const nextConfig: NextConfig = {
  reactCompiler: true,
  // TypeScript 7 ships a native compiler without the classic JS API that
  // Next's default typecheck uses — run project-local `tsc` instead.
  experimental: {
    useTypeScriptCli: true,
  },
  async rewrites() {
    // Local/dev proxy to the Go backend. /api/search is an App Router route
    // (BotID gate) — put the catch-all API proxy in afterFiles so check_fs
    // serves that route first; everything else under /api proxies upstream.
    // On Vercel, root vercel.json routes search → frontend and other /api →
    // backend before Next.js sees the request.
    return {
      beforeFiles: [
        { source: "/dl/:path*", destination: `${backendURL}/dl/:path*` },
        { source: "/health", destination: `${backendURL}/health` },
      ],
      afterFiles: [
        { source: "/api/:path*", destination: `${backendURL}/api/:path*` },
        // yatko.app/:owner/:repo mirrors github.com/:owner/:repo, transparently
        // serving the /p/:owner/:repo release page (URL bar stays as-is).
        // OG/Twitter image routes stay off /p/ so robots Disallow:/p/ does not
        // block unfurl image fetches for scrapers that honor the * rule.
        {
          source: "/:owner/:repo/opengraph-image",
          destination: "/p/:owner/:repo/opengraph-image",
        },
        {
          source: "/:owner/:repo/twitter-image",
          destination: "/p/:owner/:repo/twitter-image",
        },
        { source: "/:owner/:repo", destination: "/p/:owner/:repo" },
      ],
      fallback: [],
    };
  },
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "github.com",
        pathname: "/*.png",
      },
      {
        // github.com/<owner>.png 302s here - Next won't follow a redirect
        // to a host that isn't itself allow-listed.
        protocol: "https",
        hostname: "avatars.githubusercontent.com",
      },
    ],
  },
};

export default withBotId(nextConfig);
