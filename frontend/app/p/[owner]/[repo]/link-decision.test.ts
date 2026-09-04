import { describe, expect, test } from "bun:test";
import {
  assetFromLinkPick,
  downloadLinkPath,
  fetchLinkPick,
  parseLinkPick,
} from "./link-decision";
import type { Asset } from "./pick-asset";

const assets: Asset[] = [
  {
    name: "tool-darwin-arm64.dmg",
    browser_download_url: "https://github.com/example/tool/releases/download/v1.0.0/tool-darwin-arm64.dmg",
    size: 42,
    download_count: 9,
  },
];

describe("downloadLinkPath", () => {
  test("pins platform and arch so Mac ARM is not inferred from the UA", () => {
    expect(downloadLinkPath("cli", "cli", "v2.1.0", "macos", "arm64")).toBe(
      "/api/link/cli/cli/v2.1.0?platform=macos&arch=arm64",
    );
  });

  test("omits empty arch and encodes the tag", () => {
    expect(downloadLinkPath("owner", "repo", "v1.0.0+build", "linux", "")).toBe(
      "/api/link/owner/repo/v1.0.0%2Bbuild?platform=linux",
    );
  });
});

describe("parseLinkPick", () => {
  test("reads filename and url from a successful link payload", () => {
    expect(
      parseLinkPick({
        url: "https://example.com/tool-darwin-arm64.dmg",
        filename: "tool-darwin-arm64.dmg",
        size: 42,
        confidence: "high",
      }),
    ).toEqual({
      filename: "tool-darwin-arm64.dmg",
      url: "https://example.com/tool-darwin-arm64.dmg",
      size: 42,
    });
  });

  test("rejects low-confidence payloads even if a filename is present", () => {
    expect(
      parseLinkPick({
        url: "https://example.com/lib.jar",
        filename: "lib.jar",
        size: 1,
        confidence: "low",
      }),
    ).toBeNull();
  });
});

describe("assetFromLinkPick", () => {
  test("uses the release asset when the filename matches", () => {
    const got = assetFromLinkPick(assets, {
      filename: "tool-darwin-arm64.dmg",
      url: "https://other.example/ignored",
      size: 1,
    });
    expect(got).toEqual(assets[0]);
  });

  test("does not invent a pick while the request is in flight", () => {
    expect(assetFromLinkPick(assets, undefined)).toBeUndefined();
  });

  test("maps abstention to null", () => {
    expect(assetFromLinkPick(assets, null)).toBeNull();
  });
});

describe("fetchLinkPick", () => {
  test("treats HTTP errors as abstention", async () => {
    const orig = globalThis.fetch;
    globalThis.fetch = (async () => new Response("{}", { status: 404 })) as typeof fetch;
    try {
      expect(await fetchLinkPick("/api/link/x/y/v1")).toBeNull();
    } finally {
      globalThis.fetch = orig;
    }
  });

  test("parses a 200 payload", async () => {
    const orig = globalThis.fetch;
    globalThis.fetch = (async () =>
      new Response(
        JSON.stringify({
          url: "https://example.com/tool-darwin-arm64.dmg",
          filename: "tool-darwin-arm64.dmg",
          size: 42,
          confidence: "high",
        }),
        { status: 200 },
      )) as typeof fetch;
    try {
      expect(await fetchLinkPick("/api/link/x/y/v1")).toEqual({
        filename: "tool-darwin-arm64.dmg",
        url: "https://example.com/tool-darwin-arm64.dmg",
        size: 42,
      });
    } finally {
      globalThis.fetch = orig;
    }
  });
});
