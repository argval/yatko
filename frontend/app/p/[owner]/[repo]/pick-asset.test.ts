import { describe, expect, test } from "bun:test";
import fixtures from "../../../../../shared/picker/fixtures.json";
import { pickBestAsset, type Arch, type Asset, type Libc, type Platform } from "./pick-asset";

function asset(name: string): Asset {
  return { name, browser_download_url: `https://example.com/${name}`, size: 1, download_count: 0 };
}

type FixtureCase = {
  name: string;
  platform: string;
  arch: string;
  prefer?: string;
  libc?: string;
  assets: string[];
  expected: string | null;
};

describe("pickBestAsset shared fixtures", () => {
  for (const tc of fixtures.cases as FixtureCase[]) {
    test(tc.name, () => {
      const assets = tc.assets.map(asset);
      const got = pickBestAsset(assets, tc.platform as Platform, tc.arch as Arch, {
        prefer: tc.prefer,
        libc: (tc.libc ?? "") as Libc,
      });
      if (tc.expected === null) {
        expect(got).toBeNull();
        return;
      }
      expect(got?.name).toBe(tc.expected);
    });
  }
});
