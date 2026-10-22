import { describe, expect, test } from "bun:test";
import {
  assetFromLinkPick,
  downloadLinkPath,
  downloadRedirectPath,
  fetchLinkPick,
  parseLinkPick,
  pickFromReleaseTable,
} from "./link-decision";
import type { Asset } from "./platform-utils";
import type { VisitorContext } from "./visitor-context";

const assets: Asset[] = [
  {
    name: "tool-darwin-arm64.dmg",
    browser_download_url: "https://github.com/example/tool/releases/download/v1.0.0/tool-darwin-arm64.dmg",
    size: 42,
    download_count: 9,
  },
];

test("recommendation and pinned download preserve the same device evidence", () => {
  const context: VisitorContext = {
    platformSource: "manual", archSource: "manual",
    clientHints: { platform: "Windows", architecture: "arm", bitness: "64", platformVersion: "15.0.0" },
  };
  const lookup = new URL(downloadLinkPath("o", "r", "v1", "linux", "amd64", context), "https://yatko.app");
  const click = new URL(downloadRedirectPath("o", "r", "v1", "linux", "amd64", "app.tar.gz", context), "https://yatko.app");
  expect(click.searchParams.get("asset")).toBe("app.tar.gz");
  click.searchParams.delete("asset");
  expect(click.search).toBe(lookup.search);
  expect(lookup.searchParams.get("platform_source")).toBe("manual");
  expect(lookup.searchParams.get("platform")).toBe("linux");
  expect(lookup.searchParams.get("hint_platform")).toBe("Windows");
  expect(lookup.searchParams.get("hint_architecture")).toBe("arm");
  expect(lookup.searchParams.get("hint_platform_version")).toBe("15.0.0");
});

describe("downloadLinkPath", () => {
  test("pins platform and arch so Mac ARM is not inferred from the UA", () => {
    expect(downloadLinkPath("cli", "cli", "v2.1.0", "macos", "arm64")).toBe(
      "/api/link/cli/cli/v2.1.0?platform=macos&arch=arm64",
    );
  });

  test("pins unknown arch and encodes the tag", () => {
    expect(downloadLinkPath("owner", "repo", "v1.0.0+build", "linux", "")).toBe(
      "/api/link/owner/repo/v1.0.0%2Bbuild?platform=linux&arch=unknown",
    );
  });
});

describe("downloadRedirectPath", () => {
  test("mirrors /api/link query params on /dl", () => {
    expect(downloadRedirectPath("cli", "cli", "v2.1.0", "macos", "arm64")).toBe(
      "/dl/cli/cli/v2.1.0?platform=macos&arch=arm64",
    );
  });

  test("pins unknown arch and encodes the tag", () => {
    expect(downloadRedirectPath("owner", "repo", "v1.0.0+build", "linux", "")).toBe(
      "/dl/owner/repo/v1.0.0%2Bbuild?platform=linux&arch=unknown",
    );
  });
});

describe("pickFromReleaseTable", () => {
  const picks = {
    "macos/arm64": {
      filename: "tool-darwin-arm64.dmg",
      url: "https://example.com/arm64.dmg",
      size: 42,
    },
    macos: {
      filename: "tool-darwin-universal.dmg",
      url: "https://example.com/uni.dmg",
      size: 80,
    },
  };

  test("returns the arch-specific entry when present", () => {
    expect(pickFromReleaseTable(picks, "macos", "arm64")).toEqual(picks["macos/arm64"]);
  });

  test("uses the platform-only key when arch is empty", () => {
    expect(pickFromReleaseTable(picks, "macos", "")).toEqual(picks.macos);
  });

  test("does not fall back across arches", () => {
    expect(pickFromReleaseTable(picks, "macos", "amd64")).toBeNull();
  });

  test("abstains when the table has no match for this visitor", () => {
    expect(pickFromReleaseTable(picks, "linux", "amd64")).toBeNull();
  });

  test("signals missing table so the caller can soft-fetch", () => {
    expect(pickFromReleaseTable(undefined, "macos", "arm64")).toBeUndefined();
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

  test("rejects a miss payload that explains how the platform was chosen", () => {
    expect(
      parseLinkPick({
        url: "https://github.com/acme/tool/releases/tag/v1.0.0",
        platform: "linux",
        arch: "amd64",
        platform_source: "default",
        arch_source: "default",
        confidence: "low",
        error: "no suitable asset found for platform",
      }),
    ).toBeNull();
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


test("unsuccessful decisions preserve their status and explanation", () => {
  const decision = parseLinkPick({ status: "ambiguous", reasons: ["multiple compatible bundles", 2], filename: "wrong.exe", url: "https://example.com/wrong.exe" });
  expect(decision).toEqual({ status: "ambiguous", reasons: ["multiple compatible bundles"] });
  expect(assetFromLinkPick(assets, decision)).toBeNull();
});

test("release table failures do not become download assets", () => {
  expect(assetFromLinkPick(assets, { status: "unknown", reasons: ["inspection failed"] })).toBeNull();
  expect(assetFromLinkPick(assets, { filename: assets[0]!.name, url: "https://example.com/a", confidence: "low" })).toBeNull();
});


test("404 JSON preserves a typed abstention but cannot become a selected file", async () => {
  const original = globalThis.fetch;
  try {
    globalThis.fetch = (async () => new Response(JSON.stringify({ status: "unknown", reasons: ["inspection failed"] }), { status: 404 })) as typeof fetch;
    expect(await fetchLinkPick("/api/link/x/y/v1")).toEqual({ status: "unknown", reasons: ["inspection failed"] });
    globalThis.fetch = (async () => new Response(JSON.stringify({ status: "selected", filename: "a.exe", url: "https://example.com/a.exe" }), { status: 404 })) as typeof fetch;
    expect(await fetchLinkPick("/api/link/x/y/v1")).toBeNull();
  } finally { globalThis.fetch = original; }
});
