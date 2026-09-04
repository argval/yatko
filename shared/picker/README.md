# Asset Picker — shared contract

**Deep module:** `backend/picker` (Go) — classify filenames into structured
facts, then rank and return an `AssetDecision` (asset, confidence, reasons,
alternatives). `/dl` and `/api/link` auto-select only when confidence is not
`low`. The release-page download button calls `/api/link` after detecting
platform/arch; it does not rank on the client.

**Test adapter:** `frontend/app/p/[owner]/[repo]/pick-asset.ts` mirrors ranking
so `fixtures.json` can catch catalog drift. Production download decisions
must not use it.

**Alias table:** `catalog.json` — platforms, architectures, libc, variants,
formats. Edit this file, then copy it to:

- `backend/picker/catalog.json` (`go:embed` for the container image)
- `frontend/lib/picker-catalog.json` (Vercel frontend root cannot see `shared/`)

Tests fail if the three copies drift.

**Test surface:** `fixtures.json` — both runtimes must agree on every case,
including `expected: null` for abstention.

When changing ranking or aliases:

1. Change `catalog.json` and/or Go first; update/add fixtures.
2. Copy the catalog to the backend and frontend paths above.
3. Mirror ranking changes in the TypeScript adapter.
4. Run `go test ./picker/` and `bun test` in `frontend/`.

Optional fixture fields: `prefer` (extension key), `libc` (`musl` | `gnu` |
`static`), and `userAgent` (Linux deb/rpm tiebreak when `prefer` is unset).

Do not add browser-only ranking shortcuts (keyword fallbacks, softer arch
filters) — they create silent `/dl` vs button drift.

An LLM or other enricher must stay off the `/dl` path. It may only propose
new catalog aliases or cached facts, never invent a download URL.
