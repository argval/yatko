import type { NextConfig } from "next";
import { withBotId } from "botid/next/config";

const backendURL = process.env.BACKEND_URL || "http://localhost:8080";

const nextConfig: NextConfig = {
  reactCompiler: true,
  async rewrites() {
    // Local/dev: proxy most API + download routes to the Go backend.
    // /api/search stays on Next.js so BotID can verify homepage autocomplete
    // before we proxy to the backend (see app/api/search/route.ts).
    // On Vercel, root vercel.json routes similarly (search → frontend, other
    // /api → backend) before Next.js sees the request.
    return {
      beforeFiles: [
        { source: "/api/release/:path*", destination: `${backendURL}/api/release/:path*` },
        { source: "/api/releases/:path*", destination: `${backendURL}/api/releases/:path*` },
        { source: "/api/readme/:path*", destination: `${backendURL}/api/readme/:path*` },
        { source: "/api/link/:path*", destination: `${backendURL}/api/link/:path*` },
        { source: "/dl/:path*", destination: `${backendURL}/dl/:path*` },
        { source: "/health", destination: `${backendURL}/health` },
      ],
      afterFiles: [
        // yatko.app/:owner/:repo mirrors github.com/:owner/:repo, transparently
        // serving the /p/:owner/:repo release page (URL bar stays as-is).
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
