import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { Window } from "happy-dom";
import { createElement, useState, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { PlatformSwitcher } from "./platform-switcher";
import type { Arch, Platform } from "./platform-utils";

let windowRef: Window;
let root: Root | null = null;
let container: HTMLElement;

function installDom() {
  windowRef = new Window({ url: "https://yatko.app/p/cli/cli" });
  (globalThis as { window?: Window }).window = windowRef;
  (globalThis as { document?: Document }).document =
    windowRef.document as unknown as Document;
  (globalThis as { HTMLElement?: typeof HTMLElement }).HTMLElement =
    windowRef.HTMLElement as unknown as typeof HTMLElement;
  (globalThis as { Element?: typeof Element }).Element =
    windowRef.Element as unknown as typeof Element;
  (globalThis as { Node?: typeof Node }).Node =
    windowRef.Node as unknown as typeof Node;
  (globalThis as { Text?: typeof Text }).Text =
    windowRef.Text as unknown as typeof Text;
}

async function teardownDom() {
  root?.unmount();
  root = null;
  await new Promise((resolve) => setTimeout(resolve, 0));
  windowRef.close();
  delete (globalThis as { window?: Window }).window;
  delete (globalThis as { document?: Document }).document;
}

function render(node: ReactNode): Promise<void> {
  container = windowRef.document.createElement("div");
  windowRef.document.body.appendChild(container);
  root = createRoot(container as unknown as Element);
  root.render(node);
  return new Promise((resolve) => setTimeout(resolve, 0));
}

beforeEach(() => {
  installDom();
});

afterEach(async () => {
  await teardownDom();
});

const DETECTED = { platform: "macos" as Platform, arch: "arm64" as Arch };

function Harness() {
  const [chosen, setChosen] = useState(DETECTED);
  return createElement(PlatformSwitcher, {
    detected: DETECTED,
    platform: chosen.platform,
    arch: chosen.arch,
    onChange: setChosen,
  });
}

describe("PlatformSwitcher", () => {
  test("shows the detected OS and arch", async () => {
    await render(createElement(Harness));
    expect(container.textContent).toContain("Detected:");
    expect(container.textContent).toContain("macOS arm64");
    expect(container.textContent).not.toContain("Use detected");
  });

  test("switching OS updates the label and offers a reset", async () => {
    await render(createElement(Harness));
    const platformSelect = container.querySelector("#yatko-platform") as HTMLSelectElement;
    expect(platformSelect).toBeTruthy();
    platformSelect.value = "linux";
    platformSelect.dispatchEvent(new windowRef.Event("change", { bubbles: true }));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(container.textContent).toContain("Selected:");
    expect(container.textContent).toContain("Linux arm64");
    expect(container.textContent).toContain("Use detected");
  });
});
