import { describe, expect, test } from "bun:test";
import {
  findChecksumAsset,
  isChecksumAssetName,
  parseChecksumText,
} from "./parse-checksums";
import type { Asset } from "./pick-asset";

function asset(name: string): Asset {
  return { name, browser_download_url: `https://example.com/${name}`, size: 1, download_count: 0 };
}

describe("isChecksumAssetName", () => {
  test("matches common manifests", () => {
    expect(isChecksumAssetName("SHA256SUMS")).toBe(true);
    expect(isChecksumAssetName("checksums.txt")).toBe(true);
    expect(isChecksumAssetName("tool.sha256")).toBe(true);
    expect(isChecksumAssetName("tool.tar.gz")).toBe(false);
  });
});

describe("findChecksumAsset", () => {
  test("returns the checksum asset", () => {
    const assets = [asset("app-linux.tar.gz"), asset("SHA256SUMS")];
    expect(findChecksumAsset(assets)?.name).toBe("SHA256SUMS");
  });

  test("returns undefined when none", () => {
    expect(findChecksumAsset([asset("app.zip")])).toBeUndefined();
  });
});

describe("parseChecksumText", () => {
  test("parses spaced and starred filenames", () => {
    const text = `
abc123  app-linux.tar.gz
def456 *app-darwin.zip
ignored
ghi789  ./nested/app.exe
`.trim();
    expect(parseChecksumText(text)).toEqual({
      "app-linux.tar.gz": "abc123",
      "app-darwin.zip": "def456",
      "./nested/app.exe": "ghi789",
    });
  });

  test("empty input yields empty map", () => {
    expect(parseChecksumText("")).toEqual({});
  });
});
