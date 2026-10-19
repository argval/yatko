import { afterEach, describe, expect, test } from "bun:test";
import {
  installCommandMatchesTarget,
  archFromHints,
  assetMatchesTarget,
  assetTargetLabel,
  detectPlatformFromUA,
  type Asset,
  detectArchFromUA,
  detectArchFromWebGL,
  resetDetectArchFromWebGLCache,
} from "./platform-utils";

afterEach(() => {
  resetDetectArchFromWebGLCache();
});

describe("detectPlatformFromUA", () => {
  test("reads darwin from a curl target triple", () => {
    expect(detectPlatformFromUA("curl/8.4.0 (aarch64-apple-darwin24.0)")).toBe("macos");
  });

  test("reads mingw as Windows", () => {
    expect(detectPlatformFromUA("curl/8.5.0 (x86_64-w64-mingw32)")).toBe("windows");
  });
});

describe("detectArchFromUA", () => {
  test("matches Go's bounded ARM detection without matching unrelated words", () => {
    for (const ua of ["arm", "ARM", "Mozilla/5.0 (X11; Linux arm)", "curl/8.5.0 (arm-unknown-linux-gnueabihf)", "linux_arm", "arm32"]) {
      expect(detectArchFromUA(ua)).toBe("arm");
    }
    for (const ua of ["charm-client/1.0", "farm", "armada", "armv8"]) {
      expect(detectArchFromUA(ua)).toBe("");
    }
    expect(detectArchFromUA("arm arm64")).toBe("arm64");
    expect(detectArchFromUA("arm aarch64")).toBe("arm64");
  });

  test("reads arm64 from explicit UA tokens", () => {
    expect(
      detectArchFromUA(
        "Mozilla/5.0 (Macintosh; ARM64 Mac OS X 10_15_7) AppleWebKit/537.36 arm64",
      ),
    ).toBe("arm64");
  });

  test("Macintosh Intel UA has no arch signal (Apple Silicon lie)", () => {
    expect(
      detectArchFromUA(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
      ),
    ).toBe("");
  });
});

describe("detectArchFromWebGL", () => {
  test("maps Apple GPU renderers to arm64", () => {
    const prev = globalThis.document;
    const getExtension = () => ({ UNMASKED_RENDERER_WEBGL: 0x9246 });
    const getParameter = () => "ANGLE (Apple, Apple M1 Pro, OpenGL 4.1)";
    (globalThis as { document?: Document }).document = {
      createElement: () =>
        ({
          getContext: () => ({ getExtension, getParameter }),
        }) as unknown as HTMLCanvasElement,
    } as unknown as Document;

    expect(detectArchFromWebGL()).toBe("arm64");
    // Memoized
    expect(detectArchFromWebGL()).toBe("arm64");

    (globalThis as { document?: Document }).document = prev;
  });

  test("maps Intel GPU renderers to amd64", () => {
    const prev = globalThis.document;
    (globalThis as { document?: Document }).document = {
      createElement: () =>
        ({
          getContext: () => ({
            getExtension: () => ({ UNMASKED_RENDERER_WEBGL: 1 }),
            getParameter: () => "Intel Iris OpenGL Engine",
          }),
        }) as unknown as HTMLCanvasElement,
    } as unknown as Document;

    expect(detectArchFromWebGL()).toBe("amd64");
    (globalThis as { document?: Document }).document = prev;
  });
});

test("structured architecture needs bitness for ambiguous x86 and ARM hints", () => {
  expect(archFromHints({ architecture: "arm", bitness: "64" })).toBe("arm64");
  expect(archFromHints({ architecture: "arm", bitness: "32" })).toBe("arm");
  expect(archFromHints({ architecture: "x86", bitness: "32" })).toBe("386");
  expect(archFromHints({ architecture: "x86", bitness: "64" })).toBe("amd64");
  expect(archFromHints({ architecture: "x86" })).toBe("");
  expect(detectPlatformFromUA("unknown")).toBe("");
  expect(detectPlatformFromUA("macOS")).toBe("macos");
});

const inspectedZip: Asset = {
  name: "download.zip", browser_download_url: "https://example.com/download.zip", size: 42, download_count: 0,
  artifact: {
    targets: [{ platform: "windows", arch: "arm64" }], role: "application", status: "inspected",
    evidence: [{ source: "archive", field: "entry", value: "tool.exe" }],
  },
};

test("opaque archives use inspected targets rather than their outer filename", () => {
  expect(assetTargetLabel(inspectedZip)).toBe("Windows · arm64");
  expect(assetMatchesTarget(inspectedZip, { platform: "windows", arch: "arm64" })).toBe(true);
  expect(assetMatchesTarget(inspectedZip, { platform: "windows", arch: "amd64" })).toBe(false);
  expect(assetMatchesTarget(inspectedZip, { platform: "", arch: "" })).toBe(false);
});

test("unknown and conflicting evidence never inherit filename compatibility", () => {
  const asset = { ...inspectedZip, name: "tool-windows-arm64.zip", artifact: { ...inspectedZip.artifact!, targets: [] } };
  expect(assetMatchesTarget(asset, { platform: "windows", arch: "arm64" })).toBe(false);
  expect(assetTargetLabel(asset)).toBe("Target unknown");
  const conflict = { ...inspectedZip, artifact: { ...inspectedZip.artifact!, conflicts: ["different platforms"] } };
  expect(assetMatchesTarget(conflict, { platform: "windows", arch: "arm64" })).toBe(false);
  expect(assetTargetLabel(conflict)).toBe("Conflicting target evidence");
});

test("source and metadata remain visible with their role but do not match the application filter", () => {
  const source: Asset = { ...inspectedZip, artifact: { ...inspectedZip.artifact!, role: "source" } };
  expect(assetTargetLabel(source)).toBe("Source code");
  expect(assetMatchesTarget(source, { platform: "windows", arch: "arm64" })).toBe(false);
});

test("install command filtering requires a known matching target", () => {
  const command = { platform: "linux", arches: ["arm64"] };
  expect(installCommandMatchesTarget(command, { platform: "linux", arch: "arm64" })).toBe(true);
  expect(installCommandMatchesTarget(command, { platform: "linux", arch: "amd64" })).toBe(false);
  expect(installCommandMatchesTarget(command, { platform: "linux", arch: "" })).toBe(false);
  expect(installCommandMatchesTarget({ platform: "universal" }, { platform: "", arch: "" })).toBe(false);
  expect(installCommandMatchesTarget({ platform: "unknown" }, { platform: "linux", arch: "arm64" })).toBe(false);
  expect(installCommandMatchesTarget({ platform: "universal" }, { platform: "windows", arch: "amd64" })).toBe(true);
});


test("partial target labels keep missing architecture explicit", () => {
  expect(assetTargetLabel({ ...inspectedZip, artifact: { ...inspectedZip.artifact!, targets: [{ platform: "macos" }] } })).toBe("macOS · CPU unknown");
});


test("multi-OS install methods match each stated OS and retain CPU limits", () => {
  const command = { platform: "macos", platforms: ["macos", "linux"], arches: ["arm64"] };
  expect(installCommandMatchesTarget(command, { platform: "macos", arch: "arm64" })).toBe(true);
  expect(installCommandMatchesTarget(command, { platform: "linux", arch: "arm64" })).toBe(true);
  expect(installCommandMatchesTarget(command, { platform: "windows", arch: "arm64" })).toBe(false);
  expect(installCommandMatchesTarget(command, { platform: "linux", arch: "amd64" })).toBe(false);
  expect(installCommandMatchesTarget(command, { platform: "", arch: "arm64" })).toBe(false);
  expect(installCommandMatchesTarget({ platform: "universal", platforms: ["linux"] }, { platform: "windows", arch: "amd64" })).toBe(false);
});

test("target filters keep emulated builds, matching the Go fallback table", () => {
  const stub: Asset = { ...inspectedZip, name: "Obsidian-1.14.4.exe", artifact: { ...inspectedZip.artifact!, targets: [{ platform: "windows", arch: "386" }] } };
  expect(assetMatchesTarget(stub, { platform: "windows", arch: "amd64" })).toBe(true);
  expect(assetMatchesTarget(stub, { platform: "windows", arch: "arm64" })).toBe(true);
  expect(assetMatchesTarget(inspectedZip, { platform: "windows", arch: "386" })).toBe(false);
  const x64: Asset = { ...stub, artifact: { ...stub.artifact!, targets: [{ platform: "windows", arch: "amd64" }] } };
  expect(assetMatchesTarget(x64, { platform: "windows", arch: "arm64" })).toBe(true);
  expect(assetMatchesTarget(x64, { platform: "windows", arch: "386" })).toBe(false);
  const intelMac: Asset = { ...stub, artifact: { ...stub.artifact!, targets: [{ platform: "macos", arch: "amd64" }] } };
  expect(assetMatchesTarget(intelMac, { platform: "macos", arch: "arm64" })).toBe(true);
  const armMac: Asset = { ...stub, artifact: { ...stub.artifact!, targets: [{ platform: "macos", arch: "arm64" }] } };
  expect(assetMatchesTarget(armMac, { platform: "macos", arch: "amd64" })).toBe(false);
});

test("a known visitor CPU requires evidence of a compatible artifact CPU", () => {
  const partial: Asset = { ...inspectedZip, artifact: { ...inspectedZip.artifact!, targets: [{ platform: "windows" }] } };
  expect(assetMatchesTarget(partial, { platform: "windows", arch: "amd64" })).toBe(false);
  expect(assetMatchesTarget(partial, { platform: "windows", arch: "" })).toBe(true);
});

test("CPU and libc compatibility must come from the same observed target", () => {
  const asset: Asset = { ...inspectedZip, artifact: { ...inspectedZip.artifact!, targets: [
    { platform: "linux", arch: "amd64", libc: "gnu" },
    { platform: "linux", arch: "arm64", libc: "musl" },
  ] } };
  expect(assetMatchesTarget(asset, { platform: "linux", arch: "amd64", libc: "musl" })).toBe(false);
  expect(assetMatchesTarget(asset, { platform: "linux", arch: "arm64", libc: "musl" })).toBe(true);
  const failed: Asset = { ...asset, artifact: { ...asset.artifact!, status: "failed" } };
  expect(assetMatchesTarget(failed, { platform: "linux", arch: "arm64" })).toBe(false);
});

test("assets without server facts never match a target, whatever their filename", () => {
  const { artifact: _, ...bare } = inspectedZip;
  expect(assetMatchesTarget({ ...bare, name: "tool-windows-x64.exe" }, { platform: "windows", arch: "amd64" })).toBe(false);
  expect(assetTargetLabel({ ...bare, name: "tool-windows-x64.exe" })).toBe("Target unknown");
});
