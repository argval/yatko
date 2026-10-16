"use client";

import { useSyncExternalStore } from "react";
import { archFromHints, browserHints, detectArch, detectPlatform, detectPlatformFromUA, type Arch, type Platform } from "./platform-utils";

export type DetectedPlatform = { platform: Platform | ""; arch: Arch };
const storageKey = "yatko:download-target";
const listeners = new Set<() => void>();
let automatic: DetectedPlatform | null = null;
let override: DetectedPlatform | null = null;
let snapshot: DetectedPlatform | null = null;
let generation = 0;

function publish() {
  const next = override ?? automatic;
  if (snapshot?.platform === next?.platform && snapshot?.arch === next?.arch) return;
  snapshot = next;
  listeners.forEach((listener) => listener());
}

/** Validate persisted preferences because storage can contain older or edited data. */
function storedTarget(): DetectedPlatform | null {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(storageKey) ?? "null");
    if (!value || typeof value !== "object" || !("platform" in value) || !("arch" in value)) return null;
    const { platform, arch } = value;
    if ((platform !== "windows" && platform !== "macos" && platform !== "linux" && platform !== "android" && platform !== "ios") ||
        (arch !== "" && arch !== "amd64" && arch !== "arm64" && arch !== "arm" && arch !== "386")) return null;
    return { platform, arch };
  } catch {
    return null;
  }
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  if (listeners.size === 1) {
    const currentGeneration = ++generation;
    automatic = { platform: detectPlatform(), arch: detectArch() };
    override = storedTarget();
    publish();
    const hints = browserHints();
    // The low-entropy UA is only a fallback while structured hardware hints load.
    if (hints?.getHighEntropyValues) {
      void hints.getHighEntropyValues(["architecture", "bitness", "platform"]).then((values) => {
        if (generation !== currentGeneration) return;
        automatic = { platform: detectPlatformFromUA(values.platform ?? "") || detectPlatform(), arch: archFromHints(values) || detectArch() };
        publish();
      }).catch(() => { /* Permission or browser policy can deny client hints. */ });
    }
  }
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) generation += 1;
  };
}

function clientSnapshot(): DetectedPlatform | null {
  if (snapshot === null && typeof navigator !== "undefined") {
    automatic = { platform: detectPlatform(), arch: detectArch() };
    snapshot = automatic;
  }
  return snapshot;
}

/** One stable snapshot shares automatic detection and manual choices across release sections. */
export function usePlatform(): DetectedPlatform | null {
  return useSyncExternalStore(subscribe, clientSnapshot, (): null => null);
}

/** Manual choices persist only in this browser; null restores automatic detection. */
export function setPlatformTarget(target: DetectedPlatform | null): void {
  override = target;
  try {
    if (target) localStorage.setItem(storageKey, JSON.stringify(target));
    else localStorage.removeItem(storageKey);
  } catch { /* Private browsing can disable storage. The in-memory choice still works. */ }
  publish();
}
