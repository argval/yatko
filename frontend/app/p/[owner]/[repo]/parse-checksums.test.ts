import { describe, expect, test } from "bun:test";
import {
  findChecksumAssets,
  isChecksumAssetName,
  parseChecksumText,
} from "./parse-checksums";
import type { Asset } from "./pick-asset";

const sha256 = "a".repeat(64);

function asset(name: string): Asset {
  return { name, browser_download_url: `https://example.com/${name}`, size: 1, download_count: 0 };
}

describe("isChecksumAssetName", () => {
  test("matches common manifests", () => {
    expect(isChecksumAssetName("SHA256SUMS")).toBe(true);
    expect(isChecksumAssetName("checksums.txt")).toBe(true);
    expect(isChecksumAssetName("tool.sha256")).toBe(true);
    expect(isChecksumAssetName("tool.SHA256")).toBe(true);
    expect(isChecksumAssetName("SHA512SUMS")).toBe(false);
    expect(isChecksumAssetName("checksums-sha-512.txt")).toBe(false);
    expect(isChecksumAssetName("tool.md5")).toBe(false);
    expect(isChecksumAssetName("tool.tar.gz")).toBe(false);
  });
});

describe("findChecksumAssets", () => {
  test("prefers SHA256 manifests, then sidecars, then generic manifests", () => {
    const assets = [
      asset("checksums.txt"),
      asset("app-linux.tar.gz.sha256"),
      asset("SHA512SUMS"),
      asset("SHA256SUMS"),
    ];
    expect(findChecksumAssets(assets).map((candidate) => candidate.name)).toEqual([
      "SHA256SUMS",
      "app-linux.tar.gz.sha256",
      "checksums.txt",
    ]);
  });

  test("returns no candidates when none match", () => {
    expect(findChecksumAssets([asset("app.zip")])).toEqual([]);
  });
});

describe("parseChecksumText", () => {
  test("parses GNU and BSD SHA256 manifests without losing filename spaces", () => {
    const text = `
${sha256}  app linux.tar.gz
${sha256} *app-darwin.zip
SHA256 (./nested/app.exe) = ${sha256}
${"b".repeat(128)}  app-with-sha512.tar.gz
ignored
`.trim();
    expect(parseChecksumText(text)).toEqual({
      "app linux.tar.gz": sha256,
      "app-darwin.zip": sha256,
      "nested/app.exe": sha256,
    });
  });

  test("uses a standalone SHA256 only for a named sidecar", () => {
    expect(parseChecksumText(sha256, "app-linux.tar.gz")).toEqual({
      "app-linux.tar.gz": sha256,
    });
    expect(parseChecksumText(sha256)).toEqual({});
  });

  test("empty input yields empty map", () => {
    expect(parseChecksumText("")).toEqual({});
  });
});
