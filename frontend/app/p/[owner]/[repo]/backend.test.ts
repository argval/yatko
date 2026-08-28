import { afterEach, describe, expect, test } from "bun:test";
import { getChecksums } from "./backend";
import type { Asset } from "./pick-asset";

const sha256 = "a".repeat(64);
const linuxSha256 = "b".repeat(64);
const originalFetch = globalThis.fetch;

function asset(name: string): Asset {
  return { name, browser_download_url: `https://example.com/${name}`, size: 1, download_count: 0 };
}

afterEach(() => {
  globalThis.fetch = originalFetch;
});

describe("getChecksums", () => {
  test("merges SHA256 manifests and sidecars for release assets only", async () => {
    globalThis.fetch = (async (input: string | URL | Request) => {
      const url = input.toString();
      if (url.endsWith("SHA256SUMS")) return new Response(`${sha256}  app-macos.zip`);
      if (url.endsWith("app-linux.tar.gz.sha256")) return new Response(linuxSha256);
      return new Response(null, { status: 404 });
    }) as typeof fetch;

    const checksums = await getChecksums([
      asset("app-macos.zip"),
      asset("app-linux.tar.gz"),
      asset("SHA256SUMS"),
      asset("app-linux.tar.gz.sha256"),
      asset("SHA512SUMS"),
    ]);

    expect(checksums).toEqual({
      "app-macos.zip": sha256,
      "app-linux.tar.gz": linuxSha256,
    });
  });
});
