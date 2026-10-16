"use client";

import { platformLabels, type Arch, type Platform } from "./platform-utils";
import { setPlatformTarget, type DetectedPlatform } from "./use-platform";

const selectClass = "min-h-11 rounded-lg border border-border bg-background px-3 text-sm text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground active:bg-foreground/5";

/** Native controls let visitors correct reduced browser hints or choose another device. */
export function TargetSelector({ target }: { target: DetectedPlatform }) {
  return (
    <div className="flex flex-wrap items-center justify-center gap-2">
      <label className="flex items-center gap-2 text-xs text-muted">
        OS
        <select aria-label="Download operating system" className={selectClass} value={target.platform}
          onChange={(event) => setPlatformTarget({ platform: event.target.value as Platform, arch: "" })}>
          <option value="" disabled>Select OS</option>
          {Object.entries(platformLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select>
      </label>
      <label className="flex items-center gap-2 text-xs text-muted">
        CPU
        <select aria-label="Download CPU architecture" className={selectClass} value={target.arch}
          onChange={(event) => setPlatformTarget({ ...target, arch: event.target.value as Arch })}>
          <option value="">Not specified</option>
          <option value="amd64">x64 / Intel 64</option>
          <option value="arm64">ARM64 / Apple Silicon</option>
          <option value="386">x86 / 32-bit</option>
          <option value="arm">ARM / 32-bit</option>
        </select>
      </label>
      <button type="button" onClick={() => setPlatformTarget(null)}
        className="min-h-11 rounded-lg px-2 text-xs text-muted underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground active:bg-foreground/5">
        Detect automatically
      </button>
    </div>
  );
}
