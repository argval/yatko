/**
 * Regression for React #185: useSyncExternalStore getSnapshot must be
 * Object.is-stable. Returning a fresh `{ platform, arch }` object each call
 * infinite-loops and crashes release pages in production.
 */
import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { Window } from "happy-dom";
import { createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { detectArch, detectPlatform, resetDetectArchFromWebGLCache } from "./platform-utils";
import { usePlatform } from "./use-platform";

const MAC_UA =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";
const WIN_UA =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";

let windowRef: Window;
let root: Root | null = null;
let container: HTMLElement;
let activeUA = MAC_UA;

function installDom() {
  windowRef = new Window({ url: "https://yatko.app/p/cli/cli" });
  Object.defineProperty(windowRef.navigator, "userAgent", {
    configurable: true,
    get: () => activeUA,
  });
  (globalThis as { window?: Window }).window = windowRef;
  (globalThis as { document?: Document }).document =
    windowRef.document as unknown as Document;
  (globalThis as { navigator?: Navigator }).navigator =
    windowRef.navigator as unknown as Navigator;
  (globalThis as { HTMLElement?: typeof HTMLElement }).HTMLElement =
    windowRef.HTMLElement as unknown as typeof HTMLElement;
  (globalThis as { Element?: typeof Element }).Element =
    windowRef.Element as unknown as typeof Element;
  (globalThis as { Node?: typeof Node }).Node =
    windowRef.Node as unknown as typeof Node;
  (globalThis as { Text?: typeof Text }).Text =
    windowRef.Text as unknown as typeof Text;
  (globalThis as { DocumentFragment?: typeof DocumentFragment }).DocumentFragment =
    windowRef.DocumentFragment as unknown as typeof DocumentFragment;
  (globalThis as { MutationObserver?: typeof MutationObserver }).MutationObserver =
    windowRef.MutationObserver as unknown as typeof MutationObserver;
}

async function teardownDom() {
  root?.unmount();
  root = null;
  // Let React finish any scheduled work before closing happy-dom.
  await new Promise((resolve) => setTimeout(resolve, 0));
  windowRef.close();
  delete (globalThis as { window?: Window }).window;
  delete (globalThis as { document?: Document }).document;
  delete (globalThis as { navigator?: Navigator }).navigator;
}

function render(node: ReactNode): Promise<void> {
  container = windowRef.document.createElement("div");
  windowRef.document.body.appendChild(container);
  root = createRoot(container as unknown as Element);
  root.render(node);
  return new Promise((resolve) => setTimeout(resolve, 0));
}

beforeEach(() => {
  activeUA = MAC_UA;
  installDom();
  // Avoid a sticky WebGL memo from a prior test leaking into this suite.
  resetDetectArchFromWebGLCache();
});

afterEach(async () => {
  await teardownDom();
});

describe("usePlatform", () => {
  test("primitive getSnapshots stay Object.is-stable across calls", () => {
    expect(Object.is(detectPlatform(), detectPlatform())).toBe(true);
    expect(Object.is(detectArch(), detectArch())).toBe(true);
    // The pre-fix pattern: a new object every call fails Object.is and
    // trips React error #185 ("getSnapshot should be cached").
    const a = { platform: detectPlatform(), arch: detectArch() };
    const b = { platform: detectPlatform(), arch: detectArch() };
    expect(Object.is(a, b)).toBe(false);
  });

  test("settles without React #185 and reports client platform/arch", async () => {
    let renders = 0;
    let last: ReturnType<typeof usePlatform> | undefined;

    function Probe() {
      renders += 1;
      last = usePlatform();
      return createElement(
        "div",
        { "data-renders": String(renders) },
        last?.platform ?? "null",
      );
    }

    await render(createElement(Probe));

    expect(renders).toBeLessThan(10);
    expect(last).toEqual({ platform: "macos", arch: "" });
    expect(container.textContent).toBe("macos");
  });

  test("detects windows/amd64 from a Windows Chrome UA", async () => {
    activeUA = WIN_UA;
    let last: ReturnType<typeof usePlatform> | undefined;
    function Probe() {
      last = usePlatform();
      return createElement("div", null, last?.platform ?? "null");
    }
    await render(createElement(Probe));

    expect(last).toEqual({ platform: "windows", arch: "amd64" });
  });
});
