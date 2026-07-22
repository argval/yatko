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

// usePlatform detects the visitor's platform/arch. When initial* is provided
// (from the request User-Agent on the server), SSR and the first client paint
// match — avoiding a Windows→macOS/Linux flash. After hydration, navigator
// (including userAgentData) can refine the arch.
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
