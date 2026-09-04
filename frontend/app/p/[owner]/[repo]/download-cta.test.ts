import { describe, expect, test } from "bun:test";
import { downloadCta } from "./download-cta";
import type { Asset } from "./platform-utils";

const apk: Asset = {
  name: "termux.apk",
  browser_download_url: "https://example.com/termux.apk",
  size: 1024,
  download_count: 10,
};

describe("downloadCta", () => {
  test("points at /dl as soon as the platform is known, even before the pick returns", () => {
    expect(
      downloadCta({
        platform: "android",
        arch: "arm64",
        tagName: "v0.118.0",
        primaryAsset: undefined,
        hasAssets: true,
        owner: "termux",
        repo: "termux-app",
      }),
    ).toEqual({
      href: "/dl/termux/termux-app/v0.118.0?platform=android&arch=arm64",
      label: "Download for Android",
      external: false,
    });
  });

  test("keeps /dl after a confirmed pick so Go still decides on click", () => {
    expect(
      downloadCta({
        platform: "android",
        arch: "arm64",
        tagName: "v0.118.0",
        primaryAsset: apk,
        hasAssets: true,
        owner: "termux",
        repo: "termux-app",
      }),
    ).toEqual({
      href: "/dl/termux/termux-app/v0.118.0?platform=android&arch=arm64",
      label: "Download for Android",
      external: false,
    });
  });

  test("does not claim a platform download when Go abstains", () => {
    expect(
      downloadCta({
        platform: "ios",
        arch: "arm64",
        tagName: "v0.118.0",
        primaryAsset: null,
        hasAssets: true,
        owner: "termux",
        repo: "termux-app",
      }),
    ).toEqual({
      href: "#downloads",
      label: "See all downloads",
      external: false,
    });
  });

  test("falls back to GitHub when the release has no assets", () => {
    expect(
      downloadCta({
        platform: "ios",
        arch: "arm64",
        tagName: "v0.118.0",
        primaryAsset: null,
        hasAssets: false,
        owner: "termux",
        repo: "termux-app",
      }),
    ).toEqual({
      href: "https://github.com/termux/termux-app/releases/latest",
      label: "View Release on GitHub",
      external: true,
    });
  });
});
