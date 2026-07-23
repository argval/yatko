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
  test("labels a matched asset with the visitor platform", () => {
    expect(
      downloadCta({
        platform: "android",
        primaryAsset: apk,
        hasAssets: true,
        owner: "termux",
        repo: "termux-app",
      }),
    ).toEqual({
      href: apk.browser_download_url,
      label: "Download for Android",
      external: false,
    });
  });

  test("does not claim a platform download when no matching binary exists", () => {
    expect(
      downloadCta({
        platform: "ios",
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
