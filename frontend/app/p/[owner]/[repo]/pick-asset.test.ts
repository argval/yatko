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
  expect(f.source).toBe(false);
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
    expect(assetPlatformLabel("app.apk")).toBeNull();
  });

  test("bare archives stay unlabeled", () => {
    expect(assetPlatformLabel("release.zip")).toBeNull();
  });
});

test("bare ARM is 32-bit except on macOS", () => {
  expect(classify("tool-linux-arm.tar.gz").arches).toEqual(["arm"]);
  expect(classify("tool-macos-arm.zip").arches).toEqual(["arm64"]);
  expect(classify("tool-arm64-linux.tgz").arches).toEqual(["arm64"]);
});

test("source tokens have word boundaries", () => {
  expect(classify("tool-source.zip").source).toBe(true);
  expect(classify("resource-windows.zip").source).toBe(false);
});

test("Electron win32 namespace does not add a second CPU", () => {
  expect(classify("tool-win32-x64.zip").arches).toEqual(["amd64"]);
  expect(classify("tool-win32-arm64.zip").arches).toEqual(["arm64"]);
  expect(classify("tool-win32.exe").arches).toEqual(["386"]);
});

test("unsupported CPU constraints remain explicit instead of becoming unknown", () => {
  expect(classify("tool-linux-riscv64.tar.gz").arches).toEqual(["riscv64"]);
  expect(classify("tool-linux-aarch64_be.tar.gz").arches).toEqual(["aarch64_be"]);
  expect(classify("tool-linux-ppc64le.tar.gz").arches).toEqual(["ppc64le"]);
  expect(assetPlatformLabel("tool-freebsd-amd64.zip")).toBeNull();
});

test("universal Mac packages cover both native CPUs", () => {
  expect(classify("tool-universal.dmg").arches).toEqual(["amd64", "arm64"]);
  expect(classify("tool-macos-universal.zip").arches).toEqual(["amd64", "arm64"]);
  expect(classify("tool-universal.zip").arches).toEqual([]);
});

test("Apple CPU clue does not override an explicit Rust target architecture", () => {
  expect(classify("tool-macos-Apple.dmg").arches).toEqual(["arm64"]);
  expect(classify("tool-x86_64-apple-darwin.tar.gz").arches).toEqual(["amd64"]);
});
