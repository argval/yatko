/**
 * Integration: the device controls stay hidden behind "Change" when detection
 * and the picker agree, and open by themselves only when a choice is needed.
 */
import { afterEach, beforeEach, expect, test } from "bun:test";
import { Window } from "happy-dom";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { DownloadSection } from "./download-section";
import type { LinkPick } from "./link-decision";
import { resetDetectArchFromWebGLCache } from "./platform-utils";
import { setPlatformTarget } from "./use-platform";

const MAC_UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36";
const WIN_UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36";
const CROS_UA = "Mozilla/5.0 (X11; CrOS x86_64 16002.44.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36";

const browserGlobals = ["window", "document", "navigator", "HTMLElement", "Element", "Node", "Text", "localStorage", "IS_REACT_ACT_ENVIRONMENT"] as const;
let savedGlobals: [string, PropertyDescriptor | undefined][];
let windowRef: Window;
let root: Root;
let container: HTMLElement;
let activeUA = MAC_UA;

beforeEach(() => {
  savedGlobals = browserGlobals.map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)]);
  windowRef = new Window({ url: "https://yatko.app/p/acme/tool" });
  Object.defineProperty(windowRef.navigator, "userAgent", { configurable: true, get: () => activeUA });
  Object.assign(globalThis, {
    window: windowRef, document: windowRef.document, navigator: windowRef.navigator,
    HTMLElement: windowRef.HTMLElement, Element: windowRef.Element, Node: windowRef.Node, Text: windowRef.Text,
    localStorage: windowRef.localStorage, IS_REACT_ACT_ENVIRONMENT: true,
  });
  resetDetectArchFromWebGLCache();
  setPlatformTarget(null);
  container = windowRef.document.createElement("div") as unknown as HTMLElement;
  windowRef.document.body.appendChild(container as never);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  setPlatformTarget(null);
  windowRef.close();
  for (const [key, descriptor] of savedGlobals) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor);
    else Reflect.deleteProperty(globalThis, key);
  }
});

const asset = (name: string) => ({ name, size: 1, download_count: 0, browser_download_url: `https://github.com/acme/tool/releases/download/v1/${name}` });

async function renderSection(picks: Record<string, LinkPick>) {
  await act(async () => {
    root.render(createElement(DownloadSection, {
      owner: "acme", repo: "tool", tagName: "v1", publishedDate: "Oct 16, 2026",
      assets: [asset("tool-mac-x64.dmg"), asset("tool-mac-arm64.dmg"), asset("tool-win-x64.exe")],
      picks, checksumsPromise: Promise.resolve({}),
    }));
  });
  await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
}

const toggle = () => container.querySelector("button[aria-controls='download-target']");
const controlsOpen = () => container.querySelectorAll("select").length === 2;

test("a resolved pick keeps the controls behind Change", async () => {
  activeUA = WIN_UA;
  await renderSection({ "windows/amd64": { status: "selected", filename: "tool-win-x64.exe", confidence: "high" } });
  expect(toggle()?.textContent).toBe("Not on Windows · x64? Change");
  expect(controlsOpen()).toBe(false);
});

test("a Mac with unknown CPU and separate CPU builds opens the controls", async () => {
  activeUA = MAC_UA; // Mac UAs never state the CPU; happy-dom has no WebGL fallback
  await renderSection({ macos: { status: "ambiguous", alternatives: [] } });
  expect(controlsOpen()).toBe(true);
  expect(toggle()?.getAttribute("aria-expanded")).toBe("true");
});

test("an undetected OS (ChromeOS) opens the controls with a browse fallback", async () => {
  activeUA = CROS_UA;
  await renderSection({});
  expect(controlsOpen()).toBe(true);
  expect(toggle()?.textContent).toBe("Choose your device. Hide");
  expect(container.textContent).toContain("Browse downloads");
});


test("a new mount does not inherit an earlier unknown device snapshot", async () => {
  activeUA = CROS_UA;
  await renderSection({});
  expect(controlsOpen()).toBe(true);
  await act(async () => root.unmount());
  activeUA = WIN_UA;
  root = createRoot(container);
  await renderSection({ "windows/amd64": { status: "selected", filename: "tool-win-x64.exe", confidence: "high" } });
  expect(toggle()?.textContent).toBe("Not on Windows · x64? Change");
  expect(controlsOpen()).toBe(false);
});
