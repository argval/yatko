# 003 — Reuse the shared platformLabels map instead of redefining it

- **Status**: DONE (tsc + react-doctor --scope changed clean)
- **Commit**: d5b66a3
- **Severity**: LOW
- **Category**: Maintainability & architecture
- **Rule**: Beyond the scan (duplicated domain data)
- **Estimated scope**: 1 file, ~−5/+4 lines

## Problem

`install-commands.tsx` redefines a `platformLabels` map that is 3/4 identical to
the one already exported from `platform-utils.ts`; it only adds a `"universal"`
key. Two maps drift independently (e.g. if "macOS" is ever recased).

`frontend/app/p/[owner]/[repo]/platform-utils.ts:17` — existing shared map

    export const platformLabels: Record<Platform, string> = {
      windows: "Windows",
      macos: "macOS",
      linux: "Linux",
    };

`frontend/app/p/[owner]/[repo]/install-commands.tsx:15` — current duplicate

    const platformLabels: Record<InstallPlatform, string> = {
      macos: "macOS",
      windows: "Windows",
      linux: "Linux",
      universal: "Universal",
    };

`InstallPlatform` is `"macos" | "windows" | "linux" | "universal"` — i.e. the three
`Platform` keys plus `"universal"`.

## Target

`frontend/app/p/[owner]/[repo]/install-commands.tsx` — import the shared map and
extend it with just the extra key:

    import { usePlatform, platformLabels as basePlatformLabels } from "./platform-utils";

    // ...
    const platformLabels: Record<InstallPlatform, string> = {
      ...basePlatformLabels,
      universal: "Universal",
    };

`usePlatform` is already imported from `./platform-utils` in this file, so the
import edit only adds `platformLabels as basePlatformLabels` to the existing
import statement (line 6).

## Repo conventions to follow

- The file already imports `usePlatform` from `./platform-utils` (line 6) — extend
  that import rather than adding a second statement.
- Keep the `InstallPlatform` type and `Record<InstallPlatform, string>` annotation.

## Steps

1. At `install-commands.tsx:6`, add `platformLabels as basePlatformLabels` to the
   existing `import { usePlatform } from "./platform-utils";`.
2. At `install-commands.tsx:15`, replace the literal map with the spread form from
   the Target.
3. Re-read the diff; confirm the three usages of `platformLabels[...]` in this file
   still resolve.

## Boundaries

- Do NOT change `InstallPlatform` or `platform-utils.ts`.
- Do NOT rename the local `platformLabels` (keep call sites unchanged).
- Do NOT add dependencies.
- STOP if either file has drifted from commit d5b66a3.

## Verification

- **Mechanical**: `cd frontend && npx tsc --noEmit`; `npx react-doctor@latest --scope changed` — score not lower.
- **Behavior check**: Open a release page whose README contains install commands
  (e.g. a repo with a `brew install` / `npm install` block), expand "CLI
  Installation", and confirm each command's platform chip still reads
  "Windows"/"macOS"/"Linux"/"Universal" as before.
- **Done when**: labels render identically, typecheck passes, score not lower.
