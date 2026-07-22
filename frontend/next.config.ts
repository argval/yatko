import type { NextConfig } from "next";

const backendURL = process.env.BACKEND_URL || "http://localhost:8080";

const nextConfig: NextConfig = {
  reactCompiler: true,
  async rewrites() {
    // Local/dev: proxy API + download routes to the Go backend. On Vercel,
    // root vercel.json already routes /api, /dl, /health to the backend
    // service before Next.js sees them.
    return {
      beforeFiles: [
        { source: "/api/:path*", destination: `${backendURL}/api/:path*` },
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

export default nextConfig;
