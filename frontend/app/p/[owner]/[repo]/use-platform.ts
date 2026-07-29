"use client";

import { useSyncExternalStore } from "react";
import {
  detectArch,
  detectPlatform,
  type Arch,
  type Platform,
} from "./platform-utils";

// A value that never changes after the first client read: subscribe is a no-op.
const noopSubscribe = () => () => {};

export type DetectedPlatform = { platform: Platform; arch: Arch };

/**
 * Visitor platform/arch. `null` during SSR / the first server snapshot so
 * callers can show a skeleton instead of a wrong-OS flash. Release pages no
 * longer read User-Agent on the server (that forced dynamic rendering).
 *
 * Platform and arch are read as separate primitive snapshots — returning a
 * fresh `{ platform, arch }` object from getSnapshot would fail Object.is
 * every time and trip React error #185 (infinite re-renders).
 */
export function usePlatform(): DetectedPlatform | null {
  const platform = useSyncExternalStore(
    noopSubscribe,
    detectPlatform,
    (): null => null,
  );
  const arch = useSyncExternalStore(
    noopSubscribe,
    detectArch,
    (): null => null,
  );
  if (platform === null || arch === null) return null;
  return { platform, arch };
}
