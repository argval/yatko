import { afterEach, beforeEach, expect, test } from "bun:test";
import { Window } from "happy-dom";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { TargetSelector } from "./target-selector";
import type { DetectedPlatform } from "./use-platform";

const browserGlobals = ["window", "document", "navigator", "HTMLElement", "Element", "Node", "Text", "localStorage", "IS_REACT_ACT_ENVIRONMENT"] as const;
let savedGlobals: [string, PropertyDescriptor | undefined][];
let windowRef: Window;
let root: Root;
let container: HTMLElement;

beforeEach(() => {
  savedGlobals = browserGlobals.map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)]);
  windowRef = new Window({ url: "https://yatko.app/p/cli/cli" });
  Object.assign(globalThis, {
    window: windowRef, document: windowRef.document, navigator: windowRef.navigator,
    HTMLElement: windowRef.HTMLElement, Element: windowRef.Element, Node: windowRef.Node,
    IS_REACT_ACT_ENVIRONMENT: true,
  });
  container = windowRef.document.createElement("div") as unknown as HTMLElement;
  windowRef.document.body.appendChild(container as never);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  windowRef.close();
  for (const [key, descriptor] of savedGlobals) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor);
    else Reflect.deleteProperty(globalThis, key);
  }
});

const render = (target: DetectedPlatform, needsChoice: boolean) =>
  act(async () => root.render(createElement(TargetSelector, { target, needsChoice })));
const toggle = () => container.querySelector("button[aria-expanded]")!;
const hasSelects = () => container.querySelectorAll("select").length === 2;

test("detection stays behind a quiet Change link until the visitor asks", async () => {
  await render({ platform: "macos", arch: "arm64" }, false);
  expect(hasSelects()).toBe(false);
  expect(toggle().textContent).toBe("Not on macOS · Apple Silicon? Change");
  expect(toggle().getAttribute("aria-expanded")).toBe("false");
  await act(async () => (toggle() as HTMLButtonElement).click());
  expect(hasSelects()).toBe(true);
  expect(toggle().getAttribute("aria-expanded")).toBe("true");
});

test("controls open by themselves when a choice is needed, and Hide still works", async () => {
  await render({ platform: "macos", arch: "" }, true);
  expect(hasSelects()).toBe(true);
  await act(async () => (toggle() as HTMLButtonElement).click());
  expect(hasSelects()).toBe(false);
  await render({ platform: "macos", arch: "" }, true); // same need: stays hidden
  expect(hasSelects()).toBe(false);
  await render({ platform: "linux", arch: "" }, false);
  await render({ platform: "macos", arch: "" }, true); // new need: opens again
  expect(hasSelects()).toBe(true);
});

test("summary uses each OS's own CPU words", async () => {
  await render({ platform: "windows", arch: "arm64" }, false);
  expect(toggle().textContent).toBe("Not on Windows · ARM64? Change");
  await render({ platform: "", arch: "" }, true);
  expect(toggle().textContent).toBe("Choose your device. Hide");
});
