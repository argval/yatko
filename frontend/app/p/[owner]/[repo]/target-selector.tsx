"use client";

import { useState } from "react";
import { platformLabels, type Arch, type Platform } from "./platform-utils";
import { setPlatformTarget, type DetectedPlatform } from "./use-platform";

const selectClass = "min-h-11 rounded-lg border border-border bg-background px-3 text-sm text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground active:bg-foreground/5";
const linkClass = "min-h-11 rounded-lg px-2 text-xs text-muted focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground active:bg-foreground/5";

const archLabels: Record<Exclude<Arch, "">, string> = {
  amd64: "x64 / Intel 64",
  arm64: "ARM64 / Apple Silicon",
  "386": "x86 / 32-bit",
  arm: "ARM / 32-bit",
};

/** Short CPU name for the summary line, in the words each OS uses. */
function shortArch(platform: Platform, arch: Arch): string {
  if (!arch) return "";
  if (platform === "macos") return arch === "arm64" ? "Apple Silicon" : arch === "amd64" ? "Intel" : archLabels[arch];
  return { amd64: "x64", arm64: "ARM64", "386": "x86 (32-bit)", arm: "ARM (32-bit)" }[arch];
}

/** Detection stays invisible behind a quiet "Change" link; the native OS/CPU
 *  controls open on request, or by themselves when detection cannot decide. */
export function TargetSelector({ target, needsChoice }: { target: DetectedPlatform; needsChoice: boolean }) {
  const [open, setOpen] = useState(needsChoice);
  // Auto-open once each time a choice becomes necessary; never auto-close, so
  // the controls don't vanish mid-choice, and "Hide" still works afterwards.
  const [prompted, setPrompted] = useState(needsChoice);
  if (needsChoice !== prompted) {
    setPrompted(needsChoice);
    if (needsChoice) setOpen(true);
  }

  const current = target.platform
    ? [platformLabels[target.platform], shortArch(target.platform, target.arch)].filter(Boolean).join(" · ")
    : "";

  return (
    <div className="flex flex-col items-center gap-1">
      <button type="button" aria-expanded={open} aria-controls="download-target" onClick={() => setOpen(!open)} className={linkClass}>
        {current ? <>Not on {current}? </> : <>Choose your device. </>}
        <span className="underline underline-offset-4">{open ? "Hide" : "Change"}</span>
      </button>
      {open && (
        <div id="download-target" className="flex flex-wrap items-center justify-center gap-2">
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
              {Object.entries(archLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
            </select>
          </label>
          <button type="button" onClick={() => setPlatformTarget(null)} className={`${linkClass} underline underline-offset-4`}>
            Detect automatically
          </button>
        </div>
      )}
    </div>
  );
}
