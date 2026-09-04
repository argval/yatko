import { describe, expect, test } from "bun:test";
import fixtures from "../../../../../shared/picker/fixtures.json";
import sharedCatalog from "../../../../../shared/picker/catalog.json";
import localCatalog from "../../../../lib/picker-catalog.json";
import {
  classify,
  decideBestAsset,
  pickBestAsset,
  type Arch,
  type Asset,
  type Libc,
  type Platform,
} from "./pick-asset";

function asset(name: string): Asset {
  return { name, browser_download_url: `https://example.com/${name}`, size: 1, download_count: 0 };
}

type FixtureCase = {
  name: string;
  platform: string;
  arch: string;
  prefer?: string;
  libc?: string;
  userAgent?: string;
  assets: string[];
  expected: string | null;
};

test("frontend catalog matches shared/picker/catalog.json", () => {
  expect(localCatalog).toEqual(sharedCatalog);
});

describe("pickBestAsset shared fixtures", () => {
  for (const tc of fixtures.cases as FixtureCase[]) {
    test(tc.name, () => {
      const assets = tc.assets.map(asset);
      const got = pickBestAsset(assets, tc.platform as Platform, tc.arch as Arch, {
        prefer: tc.prefer,
        libc: (tc.libc ?? "") as Libc,
        userAgent: tc.userAgent,
      });
      if (tc.expected === null) {
        expect(got).toBeNull();
        return;
      }
      expect(got?.name).toBe(tc.expected);
    });
  }
});

test("classify does not invent a platform for release.zip", () => {
  const f = classify("release.zip");
  expect(f.platforms).toEqual([]);
  expect(f.arches).toEqual([]);
  expect(f.source).toBe(true);
});

test("generic jar abstains", () => {
  const d = decideBestAsset([asset("lib-1.0.0.jar")], "windows", "amd64");
  expect(d.shouldAutoSelect).toBe(false);
  expect(d.confidence).toBe("low");
});


function asset(name: string): Asset {
  return { name, browser_download_url: `https://example.com/${name}`, size: 1, download_count: 0 };
}

type FixtureCase = {
  name: string;
  platform: string;
  arch: string;
  prefer?: string;
  libc?: string;
  userAgent?: string;
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
        userAgent: tc.userAgent,
      });
      if (tc.expected === null) {
        expect(got).toBeNull();
        return;
      }
      expect(got?.name).toBe(tc.expected);
    });
  }
});
