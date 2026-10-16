"use client";

import {
  archLabels,
  formatDetectedLabel,
  platformLabels,
  type Arch,
  type Platform,
} from "./platform-utils";

const platforms: Platform[] = ["macos", "windows", "linux", "android", "ios"];
const arches: Exclude<Arch, "">[] = ["amd64", "arm64", "arm", "386"];

const selectClass =
  "px-2.5 py-1 text-sm rounded-lg border border-border bg-surface text-foreground focus:outline-none focus:ring-2 focus:ring-foreground/15 cursor-pointer";

export function PlatformSwitcher({
  detected,
  platform,
  arch,
  onChange,
}: {
  detected: { platform: Platform; arch: Arch };
  platform: Platform;
  arch: Arch;
  onChange: (next: { platform: Platform; arch: Arch }) => void;
}) {
  const overridden = platform !== detected.platform || arch !== detected.arch;

  return (
    <div className="flex flex-col items-center gap-1.5">
      <p className="text-sm text-muted">
        {overridden ? "Selected:" : "Detected:"}{" "}
        <span className="text-foreground/80">{formatDetectedLabel(platform, arch)}</span>
      </p>
      <div className="flex items-center gap-2">
        <select
          id="yatko-platform"
          value={platform}
          onChange={(e) => {
            const next = e.target.value;
            if (next in platformLabels) {
              onChange({ platform: next as Platform, arch });
            }
          }}
          className={selectClass}
          aria-label="Operating system"
        >
          {platforms.map((p) => (
            <option key={p} value={p}>
              {platformLabels[p]}
            </option>
          ))}
        </select>
        <select
          id="yatko-arch"
          value={arch}
          onChange={(e) => {
            const next = e.target.value;
            if (next === "" || next in archLabels) {
              onChange({ platform, arch: next as Arch });
            }
          }}
          className={selectClass}
          aria-label="Architecture"
        >
          <option value="">Auto</option>
          {arches.map((a) => (
            <option key={a} value={a}>
              {archLabels[a]}
            </option>
          ))}
        </select>
      </div>
      {overridden && (
        <button
          type="button"
          onClick={() => onChange(detected)}
          className="text-xs text-muted underline underline-offset-4 hover:text-foreground transition-colors"
        >
          Use detected
        </button>
      )}
    </div>
  );
}
