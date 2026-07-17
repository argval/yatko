# 002 — Replace hand-rolled compact number with Intl.NumberFormat

- **Status**: DONE (tsc + react-doctor --scope changed clean; note: compact suffix is now uppercase `K` and drops the forced `.0`, e.g. `1.2M` / `847`)
- **Commit**: d5b66a3
- **Severity**: LOW
- **Category**: Maintainability & architecture
- **Rule**: Beyond the scan (stdlib reinvention)
- **Estimated scope**: 1 file, ~−5/+2 lines

## Problem

`formatDownloadCount` hand-rolls compact "M/k" number formatting that the platform
already ships as `Intl.NumberFormat` with `notation: "compact"`.

`frontend/app/p/[owner]/[repo]/all-downloads.tsx:7` — current

    function formatDownloadCount(n: number): string {
      if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
      if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
      return `${n}`;
    }

    // used at line 61:
    {formatDownloadCount(asset.download_count)}↓

The hand-rolled version always shows one decimal (`1.0M`, `2.0k`) and doesn't
group thousands below 1000. `Intl` gives `1M`, `2.5K`, `999` and is locale-aware.

## Target

`frontend/app/p/[owner]/[repo]/all-downloads.tsx` — replace the function with a
module-scope formatter:

    const compactCount = new Intl.NumberFormat("en", {
      notation: "compact",
      maximumFractionDigits: 1,
    });

    // used at line 61:
    {compactCount.format(asset.download_count)}↓

Note `Intl` renders the compact suffix as an uppercase `K`/`M`; if preserving the
current lowercase `k` matters, keep the hand-rolled helper and close this plan as
WONTFIX — the visual delta is the only reason not to switch.

## Repo conventions to follow

- Module-scope constants for static values are the established pattern here
  (`platformLabels`, `EXAMPLES`, `CHECKSUM_RE`). Define the formatter once at
  module scope, not inside the component.
- Keep the `↓` glyph and surrounding markup exactly as-is.

## Steps

1. At `all-downloads.tsx:7`, delete `formatDownloadCount` and add the
   `compactCount` `Intl.NumberFormat` constant at module scope (below the imports).
2. At `all-downloads.tsx:61`, change the call to `compactCount.format(asset.download_count)`.
3. Re-read the diff; confirm the `formatSize` import/usage is untouched.

## Boundaries

- Do NOT touch `formatSize` (a different, byte-oriented formatter that has no
  stdlib equivalent).
- Do NOT add dependencies or change the `↓` affordance.
- STOP if the file has drifted from commit d5b66a3.

## Verification

- **Mechanical**: `cd frontend && npx tsc --noEmit`; `npx react-doctor@latest --scope changed` — score not lower.
- **Behavior check**: Open a release page for a popular repo with high download
  counts (e.g. `/p/BurntSushi/ripgrep`), expand "All Downloads", and confirm the
  per-asset count still renders in compact form (e.g. `1.2M↓`) and small counts
  render as plain integers.
- **Done when**: counts render compactly, typecheck passes, score not lower.
