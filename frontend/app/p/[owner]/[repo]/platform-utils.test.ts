import { afterEach, describe, expect, test } from "bun:test";
import {
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
