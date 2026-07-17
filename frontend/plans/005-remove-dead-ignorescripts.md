# 005 — Remove the dead `ignoreScripts` field from package.json

- **Status**: DONE (valid JSON confirmed; ignoreScripts removed, trustedDependencies intact)
- **Commit**: d5b66a3
- **Severity**: LOW
- **Category**: Maintainability & architecture (dead config)
- **Rule**: Beyond the scan — surfaced by ponytail-audit, not React Doctor
- **Estimated scope**: 1 file, −4 lines

## Problem

`frontend/package.json` declares a top-level `ignoreScripts` array. Neither Bun
(this project's package manager — see CLAUDE.md `bun install`) nor npm reads a
top-level `ignoreScripts` **array** field: Bun gates postinstall scripts via
`trustedDependencies`, and npm's equivalent is the `ignore-scripts` *boolean* in
`.npmrc`/config, not a package.json array. The field is inert.

`frontend/package.json:32` — current

    "ignoreScripts": [
      "sharp",
      "unrs-resolver"
    ],
    "trustedDependencies": [
      "sharp",
      "unrs-resolver"
    ]

The two lists name the **same** packages while asking for opposite behavior
("ignore their scripts" vs. "trust their scripts"). Only `trustedDependencies` is
honored, so `ignoreScripts` is dead and misleading — a reader could believe
`sharp`'s postinstall is suppressed when it is not.

## Target

Delete the `ignoreScripts` array; keep `trustedDependencies` as-is.

`frontend/package.json` — after:

    "trustedDependencies": [
      "sharp",
      "unrs-resolver"
    ]

(The `}` that previously followed `trustedDependencies` stays; just remove the
`ignoreScripts` key and its array.)

## Repo conventions to follow

- Preserve the file's 2-space indentation and trailing-newline style.
- Do not reorder or reformat unrelated keys.

## Steps

1. In `frontend/package.json`, delete the `"ignoreScripts": [ ... ],` block
   (lines 32–35), leaving `trustedDependencies` intact.
2. Confirm the JSON is still valid (no trailing comma left dangling before a `}`).

## Boundaries

- Do NOT touch `trustedDependencies` — it is the one Bun actually honors and
  removing it would re-enable `sharp`/`unrs-resolver` build scripts.
- Do NOT change dependency versions or any other field.
- STOP if `package.json` has drifted from commit d5b66a3.

## Verification

- **Mechanical**: `cd frontend && node -e "JSON.parse(require('fs').readFileSync('package.json','utf8'))"` — parses without error. (Optional confirmation of the claim: Bun's docs list `trustedDependencies` but no top-level `ignoreScripts` field — https://bun.sh/docs/install/lifecycle.)
- **Behavior check**: `cd frontend && bun install` still resolves and `bun run build`
  still succeeds — behavior is unchanged because the removed field did nothing.
- **Done when**: JSON is valid, `bun install` + `bun run build` succeed, and only
  the dead field was removed.
