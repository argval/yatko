import assert from "node:assert/strict";
import { createInterface } from "node:readline";
import { Readable } from "node:stream";
import { fileURLToPath } from "node:url";

const frontendDir = fileURLToPath(new URL("../", import.meta.url));
const nextCli = "node_modules/next/dist/bin/next";
const owner = "release-smoke";
const release = {
  tag_name: "v1.2.3-smoke",
  name: "Smoke test release",
  body: "",
  published_at: "2026-01-01T00:00:00Z",
  prerelease: false,
  assets: [{
    name: "app-linux-x86_64.tar.gz",
    browser_download_url: "https://example.invalid/app-linux-x86_64.tar.gz",
    size: 2048,
    download_count: 1,
  }],
};

// Serve stable releases and failure responses without calling GitHub or Vercel.
const backend = Bun.serve({
  hostname: "127.0.0.1",
  port: 0,
  fetch(request) {
    const path = new URL(request.url).pathname;
    switch (path) {
      case `/api/release/${owner}/healthy`:
        return Response.json(release);
      case `/api/release/${owner}/empty-assets`:
        return Response.json({ ...release, assets: [] });
      case `/api/release/${owner}/unavailable`:
        return new Response(null, { status: 503 });
      default:
        return Response.json({ reason: "no_releases" }, { status: 404 });
    }
  },
});

const env = {
  ...process.env,
  BACKEND_URL: backend.url.origin,
  NEXT_TELEMETRY_DISABLED: "1",
};

// Next logs the port assigned by the OS when it starts on port zero.
function frontendOrigin(server) {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error("Next.js did not start within 20 seconds")), 20_000);
    const lines = createInterface({ input: Readable.fromWeb(server.stdout) });
    lines.on("line", (line) => {
      console.log(line);
      const address = line.match(/Local:\s+(http:\/\/127\.0\.0\.1:\d+)/)?.[1];
      if (address) {
        clearTimeout(timeout);
        resolve(address);
      }
    });
    server.exited.then((code) => {
      clearTimeout(timeout);
      reject(new Error(`Next.js exited with code ${code}`));
    }, reject);
  });
}

let frontend;
try {
  const build = Bun.spawn(["node", nextCli, "build"], {
    cwd: frontendDir, env, stdout: "inherit", stderr: "inherit",
  });
  assert.equal(await build.exited, 0, "Next.js build failed");

  frontend = Bun.spawn(["node", nextCli, "start", "--hostname", "127.0.0.1", "--port", "0"], {
    cwd: frontendDir, env, stdout: "pipe", stderr: "inherit",
  });
  const origin = await frontendOrigin(frontend);
  const probes = [
    { repo: "unavailable", text: "Something went wrong", hasRelease: false },
    { repo: "no-release", text: "No releases yet", hasRelease: false },
    { repo: "empty-assets", text: release.tag_name, hasRelease: false },
    { repo: "healthy", text: release.tag_name, hasRelease: true },
  ];

  for (const { repo, text, hasRelease } of probes) {
    const slug = `${owner}/${repo}`;
    const response = await fetch(`${origin}/${slug}`, {
      redirect: "error",
      signal: AbortSignal.timeout(20_000),
    });
    assert.equal(response.status, 200, `${slug} should render a page`);
    const html = await response.text();
    assert.ok(html.includes(text), `${slug} should render ${text}`);
    assert.equal(html.includes(`data-yatko-release="${slug}"`), hasRelease, `${slug} release signal`);
    console.log(`Passed /${slug}`);
  }
} finally {
  frontend?.kill();
  backend.stop(true);
  await frontend?.exited;
}
