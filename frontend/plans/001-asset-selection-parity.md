# 001 — Make the download button and /dl redirect pick the same asset

- **Status**: DONE (implemented on commit d5b66a3; tsc + react-doctor --scope changed clean, 6/6 logic assertions pass)
- **Commit**: d5b66a3
- **Severity**: MEDIUM
- **Category**: Bugs & correctness (with a Maintainability consolidation)
- **Rule**: Beyond the scan
- **Estimated scope**: 2 files (`platform-utils.ts`, `download-section.tsx`), ~+35/−36 lines net-neutral

## Problem

The primary "Download for X" button and the `/dl/:owner/:repo` smart redirect can
resolve to **different binaries** for the same visitor. The button links directly
to the asset chosen by the frontend `pickAssets`, while `/dl` is chosen by the Go
backend `picker.PickAssetForArch`. The two rank assets differently:

- **Backend** treats architecture as a *hard filter* — when any asset explicitly
  matches the requested arch, it discards all others before ranking by extension.

  `backend/picker/asset.go:138` — current

      if arch != UnknownArch {
          var archMatches []scored
          for _, c := range candidates {
              if c.archHit {
                  archMatches = append(archMatches, c)
              }
          }
          if len(archMatches) > 0 {
              candidates = archMatches   // arch wins over extension
          }
      }

- **Frontend** ranks by *extension first* and only uses arch as a tiebreak within
  the same extension.

  `frontend/app/p/[owner]/[repo]/download-section.tsx:18` — current

      function archScore(name: string, arch: Arch): number {
        if (!arch) return 0;
        const isArm = name.includes("arm64") || name.includes("aarch64");
        const isAmd = name.includes("amd64") || name.includes("x86_64") || name.includes("x64");
        if (arch === "arm64") {
          if (isArm) return 0;
          if (isAmd) return 2;
          return 1;
        }
        if (isAmd) return 0;
        if (isArm) return 2;
        return 1;
      }

      function pickAssets(assets: Asset[], platform: Platform, arch: Arch): Asset[] {
        const exts = platformExtensions[platform];
        const keywords = platformKeywords[platform];
        const results: { asset: Asset; extRank: number; archRank: number }[] = [];
        for (const asset of assets) {
          const name = asset.name.toLowerCase();
          if (isSource(name)) continue;
          if (mentionsOtherPlatform(name, platform)) continue;

          let extRank = exts.findIndex((ext) => name.endsWith(ext));
          if (extRank === -1) {
            // No recognized extension (e.g. bare goreleaser binaries) - fall back to
            // a platform keyword match, ranked below any extension match.
            if (!keywords.some((kw) => hasBoundedKeyword(name, kw))) continue;
            extRank = exts.length;
          }
          results.push({ asset, extRank, archRank: archScore(name, arch) });
        }
        results.sort((a, b) => a.extRank - b.extRank || a.archRank - b.archRank);  // BUG: ext dominates arch
        return results.map((r) => r.asset);
      }

  `frontend/app/p/[owner]/[repo]/download-section.tsx:69` — current

      const [platform, arch] = usePlatform();
      const primaryAsset = pickAssets(assets, platform, arch)[0] ?? null;

**Concrete failure**: a macOS arm64 visitor, release assets `foo-x86_64.dmg` and
`foo-arm64.pkg`. Frontend sorts `.dmg` (extRank 0, archRank 2) before `.pkg`
(extRank 1, archRank 0) → button links to the **Intel** `.dmg`. Backend filters to
the arch match → `/dl` gives the **arm64** `.pkg`. The Share card advertises `/dl`
as "the right binary for the user's platform", so the two paths contradict each
other on the app's main call to action. Same class of bug for Linux repos that
ship one arch as a bare binary and another as `.AppImage`/`.deb`.

This is also the third copy of the asset-scoring logic (Go `picker`, the shared
primitives in `platform-utils.ts`, and this private `pickAssets`). The Go↔TS split
is intentional and `ponytail:`-flagged (cross-language, can't share). The *second
TS copy* is not — folding it into `platform-utils.ts` beside the primitives it
already imports removes the divergence at the source.

## Target

Move `archScore` and a new `pickBestAsset` into `platform-utils.ts`, exporting
`pickBestAsset`, with arch as a **hard filter** mirroring the backend. Keep the
bare-binary keyword fallback (the backend lacks it, but it's a real feature the
frontend must not lose).

`frontend/app/p/[owner]/[repo]/platform-utils.ts` — add near the other matching
helpers (after `mentionsOtherPlatform`):

    // archScore ranks how well an asset filename matches the requested arch:
    // 0 = explicit match, 1 = no arch signal, 2 = explicit wrong arch.
    function archScore(name: string, arch: Arch): number {
      if (!arch) return 0;
      const isArm = name.includes("arm64") || name.includes("aarch64");
      const isAmd = name.includes("amd64") || name.includes("x86_64") || name.includes("x64");
      if (arch === "arm64") {
        if (isArm) return 0;
        if (isAmd) return 2;
        return 1;
      }
      if (isAmd) return 0;
      if (isArm) return 2;
      return 1;
    }

    // pickBestAsset selects the single best release asset for a platform/arch,
    // mirroring the Go backend's picker.PickAssetForArch so the download button
    // and the /dl redirect always resolve to the same binary. Arch is a HARD
    // filter: when any asset explicitly matches the requested arch, only those
    // are ranked by extension — otherwise a wrong-arch asset with a nicer
    // extension would win. Returns null when nothing matches the platform.
    export function pickBestAsset(assets: Asset[], platform: Platform, arch: Arch): Asset | null {
      const exts = platformExtensions[platform];
      const keywords = platformKeywords[platform];
      const results: { asset: Asset; extRank: number; archRank: number }[] = [];
      for (const asset of assets) {
        const name = asset.name.toLowerCase();
        if (isSource(name)) continue;
        if (mentionsOtherPlatform(name, platform)) continue;

        let extRank = exts.findIndex((ext) => name.endsWith(ext));
        if (extRank === -1) {
          // No recognized extension (e.g. bare goreleaser binaries) - fall back
          // to a platform keyword match, ranked below any extension match.
          if (!keywords.some((kw) => hasBoundedKeyword(name, kw))) continue;
          extRank = exts.length;
        }
        results.push({ asset, extRank, archRank: archScore(name, arch) });
      }
      if (results.length === 0) return null;

      // Hard arch filter first, then lowest extension rank (archRank tiebreak
      // only matters in the no-exact-match fallback, preferring arch-neutral
      // assets over explicitly-wrong-arch ones).
      const archMatches = arch ? results.filter((r) => r.archRank === 0) : [];
      const pool = archMatches.length > 0 ? archMatches : results;
      pool.sort((a, b) => a.extRank - b.extRank || a.archRank - b.archRank);
      return pool[0].asset;
    }

`frontend/app/p/[owner]/[repo]/download-section.tsx` — delete the local
`archScore` and `pickAssets`, import `pickBestAsset`, and call it:

    import { Suspense, use } from "react";
    import {
      usePlatform,
      pickBestAsset,
      type Platform,
      type Arch,
      type Asset,
    } from "./platform-utils";
    import { DownloadButton } from "./download-button";
    import { AssetChecksum } from "./asset-checksum";

    // ...inside DownloadSection:
    const [platform, arch] = usePlatform();
    const primaryAsset = pickBestAsset(assets, platform, arch);

## Repo conventions to follow

- Imitate the existing exported helpers in `platform-utils.ts` (`mentionsOtherPlatform`,
  `assetPlatformLabel`) for placement, comment style, and lowercase-name handling.
- `platform-utils.ts` already carries the `ponytail:` header comment about keeping
  parity with `backend/picker/asset.go` — the new `pickBestAsset` is exactly what
  that comment is protecting; leave the header intact.
- Keep the `Arch` empty-string convention (`""` = unknown) already used throughout.

## Steps

1. In `platform-utils.ts`, add `archScore` (unexported) and `pickBestAsset`
   (exported) after `mentionsOtherPlatform` (around line 106), using the Target
   code verbatim. `archScore`, `isSource`, `mentionsOtherPlatform`, `hasBoundedKeyword`,
   `platformExtensions`, `platformKeywords` are all already in this module — no new imports.
2. In `download-section.tsx`, delete `archScore` (lines 18–30) and `pickAssets`
   (lines 32–52). Replace the import block (lines 4–14) so only `usePlatform`,
   `pickBestAsset`, and the three types remain. Change line 70 to
   `const primaryAsset = pickBestAsset(assets, platform, arch);`.
3. Re-read the diff: confirm no now-unused imports remain in `download-section.tsx`
   and that `AllDownloads`/`assetPlatformLabel` (a separate code path) is untouched.

## Boundaries

- Do NOT change the backend Go picker — the frontend is the side that's wrong.
- Do NOT change `AllDownloads` filtering or `assetPlatformLabel`; they're unrelated.
- Do NOT alter the bare-binary keyword fallback behavior — it must survive.
- Do NOT add dependencies.
- STOP if `download-section.tsx` or `platform-utils.ts` has drifted from commit
  d5b66a3; report the drift instead of improvising.

## Verification

- **Mechanical**:
  - `cd frontend && npx react-doctor@latest --scope changed` — no new diagnostics, score not lower.
  - `cd frontend && npx tsc --noEmit` (there is no lint/test runner configured on the frontend — see CLAUDE.md).
- **Behavior check**: There is no frontend test runner, so verify by hand against a
  real multi-arch macOS/Linux repo. Open `/p/<owner>/<repo>` on (or with a UA
  spoofing) an arm64 machine for a repo whose latest release ships mixed-extension
  per-arch assets (e.g. one that has both `*_x86_64.dmg` and `*_arm64.pkg`, or
  `*_amd64` bare + `*_arm64.AppImage`). Confirm the "Download for macOS/Linux"
  button's filename (shown under the button) now matches the arch, and that it is
  the **same** asset `curl -sI localhost:8080/dl/<owner>/<repo>` (with the same UA)
  redirects to. Before this fix they differ; after, they agree.
- **Done when**: button filename == `/dl` redirect target for the mixed-arch case,
  typecheck passes, and score is not lower.
