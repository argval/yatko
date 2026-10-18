"use client";

import { useSyncExternalStore } from "react";
import { archFromHints, browserHints, detectArch, detectPlatform, detectPlatformFromUA, type Arch, type Platform } from "./platform-utils";

export type DetectedPlatform = { platform: Platform | ""; arch: Arch };
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

function subscribe(listener: () => void) {
  listeners.add(listener);
  if (listeners.size === 1) {
    const currentGeneration = ++generation;
    automatic = { platform: detectPlatform(), arch: detectArch() };
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
    if (listeners.size === 0) {
      generation += 1;
      // A new mount must detect the current browser before rendering controls.
      automatic = null;
      snapshot = null;
    }
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

/** Manual choices last until refresh; null restores automatic detection immediately. */
export function setPlatformTarget(target: DetectedPlatform | null): void {
  override = target;
  publish();
}
