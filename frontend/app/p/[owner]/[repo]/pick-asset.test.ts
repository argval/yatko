import { describe, expect, test } from "bun:test";
import sharedCatalog from "../../../../../shared/picker/catalog.json";
import localCatalog from "../../../../lib/picker-catalog.json";
import { classify, primaryPlatform } from "./pick-asset";
import { assetPlatformLabel } from "./platform-utils";

test("frontend catalog matches shared/picker/catalog.json", () => {
  expect(localCatalog).toEqual(sharedCatalog);
});

test("classify does not invent a platform for release.zip", () => {
  const f = classify("release.zip");
  expect(f.platforms).toEqual([]);
  expect(f.arches).toEqual([]);
  expect(f.source).toBe(true);
});

test("classify keeps longest arch alias", () => {
  expect(classify("tool-linux.arm32.zip").arches).toEqual(["arm"]);
  expect(classify("tool-linux.arm64.zip").arches).toEqual(["arm64"]);
  expect(classify("tool-arm-unknown-linux-gnueabihf.gz").arches).toEqual(["arm"]);
  expect(classify("Dopamine-3.0.10-arm.dmg").arches).toEqual(["arm64"]);
});

describe("primaryPlatform / assetPlatformLabel", () => {
  test("linux+android zip is Android", () => {
    expect(primaryPlatform(classify("bun-linux-aarch64-android.zip"))).toBe("android");
    expect(assetPlatformLabel("bun-linux-aarch64-android.zip")).toBe("Android");
  });

  test("plain linux zip is Linux", () => {
    expect(assetPlatformLabel("bun-linux-aarch64.zip")).toBe("Linux");
  });

  test("exclusive extensions fill in when the name has no OS token", () => {
    expect(assetPlatformLabel("Setup.exe")).toBe("Windows");
    expect(assetPlatformLabel("Dopamine-3.0.10-arm.dmg")).toBe("macOS");
    expect(assetPlatformLabel("app.apk")).toBe("Android");
  });

  test("bare archives stay unlabeled", () => {
    expect(assetPlatformLabel("release.zip")).toBeNull();
  });
});
