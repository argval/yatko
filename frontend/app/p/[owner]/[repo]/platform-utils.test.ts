import { afterEach, describe, expect, test } from "bun:test";
import {
  detectArchFromUA,
  detectArchFromWebGL,
  detectPlatformFromUA,
  formatDetectedLabel,
  resetDetectArchFromWebGLCache,
} from "./platform-utils";

afterEach(() => {
  resetDetectArchFromWebGLCache();
});

describe("formatDetectedLabel", () => {
  test("renders macOS arm64 like the download page copy", () => {
    expect(formatDetectedLabel("macos", "arm64")).toBe("macOS arm64");
  });

  test("omits arch when it is unknown", () => {
    expect(formatDetectedLabel("linux", "")).toBe("Linux");
  });

  test("pretty-prints 386 as x86", () => {
    expect(formatDetectedLabel("windows", "386")).toBe("Windows x86");
  });
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
