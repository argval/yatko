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

describe("detectArchFromUA", () => {
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
  const { artifact: _, ...legacy } = inspectedZip;
  expect(assetMatchesTarget({ ...legacy, name: "tool-windows-arm64.zip" }, { platform: "windows", arch: "arm64" })).toBe(true);
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
