import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactCompiler: true,
  async rewrites() {
    // yatko.app/:owner/:repo mirrors github.com/:owner/:repo, transparently
    // serving the /p/:owner/:repo release page (URL bar stays as-is).
    return [{ source: "/:owner/:repo", destination: "/p/:owner/:repo" }];
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
