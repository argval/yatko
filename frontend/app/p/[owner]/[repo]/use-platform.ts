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

/** False during SSR / first server snapshot; true after client hydration. */
export function useIsClient(): boolean {
  return useSyncExternalStore(noopSubscribe, () => true, () => false);
}

// usePlatform detects the visitor's platform/arch on the client. Optional
// initial* values are only used as the SSR snapshot (release pages no longer
// read User-Agent on the server — that forced dynamic rendering and burned
// Fluid quota). Prefer pairing with useIsClient() to avoid a wrong-OS flash.
export function usePlatform(initialPlatform?: Platform, initialArch?: Arch): [Platform, Arch] {
  const platform = useSyncExternalStore(
    noopSubscribe,
    detectPlatform,
    () => initialPlatform ?? ("windows" as Platform),
  );
  const arch = useSyncExternalStore(
    noopSubscribe,
    detectArch,
    () => initialArch ?? ("" as Arch),
  );
  return [platform, arch];
}
