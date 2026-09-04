# Asset Picker — shared contract

**Deep module:** `backend/picker` (Go) — classify filenames into structured
facts, then rank and return an `AssetDecision` (asset, confidence, reasons,
alternatives). `/dl` and `/api/link` auto-select only when confidence is not
`low`. The release-page download button points at `/dl` as soon as
platform/arch is known (Go decides on click) and soft-fetches `/api/link`
for filename and checksum. It does not rank on the client.

**Labels:** `frontend/app/p/[owner]/[repo]/pick-asset.ts` classifies filenames
for "All downloads" grouping. It does not rank. Production download
decisions use `/dl` (click) and `/api/link` (filename/checksum).

**Alias table:** `catalog.json` — platforms, architectures, libc, variants,
formats. Overlapping arch aliases keep the longest span (`arm64` beats `arm`;
`arm-` beats `arm` on rustc triples). `linux`+`android` names target android.
Edit this file, then copy it to:

- `backend/picker/catalog.json` (`go:embed` for the container image)
- `frontend/lib/picker-catalog.json` (Vercel frontend root cannot see `shared/`)

Tests fail if the three copies drift.

**Real-release corpus:** `corpus.json` — snapshots of public GitHub releases with
labeled picks. `go test ./picker/ -run TestCorpusMetrics -v` reports top-1,
abstention, false auto-select, and wrong-platform rates. Mark a case
`knownMiss: true` when the label is right but the picker is not; do not
weaken the label to match the picker.

**Unknown tokens:** `UnknownTokens` / `HarvestUnknownTokens` list filename
pieces the catalog does not explain. Review high-frequency tokens from
`TestCorpusUnknownTokenHarvest` and from `picker_shadow` / `picker_miss`
logs, then add aliases to `catalog.json` by hand. Never promote a token
automatically, and never let an enricher invent a download URL.

When changing ranking or aliases:

1. Change `catalog.json` and/or Go first; update/add fixtures.
2. Copy the catalog to the backend and frontend paths above.
3. Run `go test ./picker/` and `bun test` in `frontend/`.

Optional fixture fields: `prefer` (extension key), `libc` (`musl` | `gnu` |
`static`), and `userAgent` (Linux deb/rpm tiebreak when `prefer` is unset).

Do not add browser-only ranking shortcuts (keyword fallbacks, softer arch
filters) — they create silent `/dl` vs button drift.

An LLM or other enricher must stay off the `/dl` path. It may only propose
new catalog aliases or cached facts, never invent a download URL.
