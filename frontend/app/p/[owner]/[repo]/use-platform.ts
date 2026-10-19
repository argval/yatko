"use client";

import { useSyncExternalStore } from "react";
import { archFromHints, browserHints, detectArch, detectPlatform, detectPlatformFromUA, type Arch, type Platform, type BrowserHints } from "./platform-utils";
import type { VisitorContext } from "./visitor-context";

export type DetectedPlatform = { platform: Platform | ""; arch: Arch; context?: VisitorContext; pending?: boolean };
const listeners = new Set<() => void>();
let automatic: DetectedPlatform | null = null;
let override: DetectedPlatform | null = null;
let snapshot: DetectedPlatform | null = null;
let generation = 0;

function publish() {
  const next = override ?? automatic;
  if (snapshot?.platform === next?.platform && snapshot?.arch === next?.arch && snapshot?.pending === next?.pending &&
      JSON.stringify(snapshot?.context) === JSON.stringify(next?.context)) return;
  snapshot = next;
  listeners.forEach((listener) => listener());
}

/** Keep device observations separate from manual target choices. */
function detectTarget(hints = browserHints(), pending = !!hints?.getHighEntropyValues): DetectedPlatform {
  const platform = detectPlatformFromUA(hints?.platform ?? "");
  const arch = archFromHints(hints);
  return {
    platform: platform || detectPlatform(),
    arch: arch || detectArch(),
    pending,
    context: {
      platformSource: platform ? "client-hints" : "browser",
      archSource: arch ? "client-hints" : "browser",
      clientHints: {
        platform: hints?.platform,
        architecture: hints?.architecture,
        bitness: hints?.bitness,
        platformVersion: hints?.platformVersion,
      },
    },
  };
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  if (listeners.size === 1) {
    const currentGeneration = ++generation;
    automatic = detectTarget();
    publish();
    const hints = browserHints();
    // The low-entropy UA is only a fallback while structured hardware hints load.
    if (hints?.getHighEntropyValues) {
      // Give hints a short head start instead of paying for two immediate decisions.
      const timer = setTimeout(() => {
        if (generation !== currentGeneration || !automatic?.pending) return;
        automatic = { ...automatic, pending: false };
        publish();
      }, 250);
      void hints.getHighEntropyValues(["architecture", "bitness", "platform", "platformVersion"]).then((values: BrowserHints) => {
        if (generation !== currentGeneration) return;
        automatic = detectTarget({ ...hints, ...values }, false);
        publish();
      }).catch(() => {
        if (generation !== currentGeneration || !automatic) return;
        automatic = { ...automatic, pending: false };
        publish();
      }).finally(() => clearTimeout(timer));
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
    automatic = detectTarget();
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
  override = target ? {
    ...target,
    pending: false,
    context: { platformSource: "manual", archSource: "manual", clientHints: automatic?.context?.clientHints },
  } : null;
  publish();
}
