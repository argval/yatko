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
 */
export function usePlatform(): DetectedPlatform | null {
  return useSyncExternalStore(
    noopSubscribe,
    (): DetectedPlatform => ({
      platform: detectPlatform(),
      arch: detectArch(),
    }),
    (): null => null,
  );
}
