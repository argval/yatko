# Asset Picker — shared contract

**Deep module:** `backend/picker` (Go) combines filename clues and observed
artifact metadata, then returns an `AssetDecision`. Known OS, CPU, and explicit
libc conflicts are excluded before format preferences. A CPU token alone does
not prove OS compatibility. Equal candidates abstain instead of using asset
order. Decisions carry `selected`, `incompatible`, `unknown`, or `ambiguous`
status, confidence, reasons, and alternatives.

`/dl` and `/api/link` auto-select only when confidence is not `low`. The page
uses the same decisions from the `picks` map on `/api/release`, including
abstention reasons. Clicks still go through `/dl`. An unspecified architecture
does not manufacture a CPU fact; exclusive installer formats retain the
existing OS-based selection policy when no CPU conflict is known.

**Shared facts:** each API asset includes `artifact` with supported target
tuples (`platform`, `arch`, `libc`), role, evidence, inspection status, and
conflicts. Missing fields remain unknown. Targets are tuples so metadata for
Windows/x86 and macOS/ARM cannot become an invented Windows/ARM build.
All Downloads uses these facts for labels and filters; the TypeScript filename
classifier only supports older responses. The browser never ranks assets.

**Observations:** the GitHub client reads published cargo-dist/Electron
metadata and bounded file contents. Entries must refer to assets in the
selected release. File inspection never executes or writes downloaded files.
The handler caps enrichment at three seconds and four content inspections.
Each inspection allows six requests, 4 MiB of downloaded bytes, 8 MiB of
expanded content, and two binary entries. Observations are cached by asset
identity/update time/digest; failures remain explicit.

Content readers support ELF, PE, Mach-O, ZIP, tar, gzip/bzip2 archives, and
Debian control metadata without xz/zstd compression. DMG, MSI, RPM, xz, zstd,
and 7z content remain unsupported. Static ELF headers can identify CPU without
proving Linux; missing OS stays unknown. Known unsupported CPUs and operating
systems remain constraints. Libraries, debug files, and updater helpers cannot
become the primary download. Cargo-dist and Electron update files are the
supported publisher schemas; unknown cargo-dist kinds are excluded.

This is structured evidence processing, with filename rules as a fallback.
No language model runs in request handling. README context still uses a fixed
vocabulary, so unfamiliar headings and prose-only prerequisites can be missed.

**Alias table:** `catalog.json` — platforms, architectures, libc, variants,
formats. Overlapping arch aliases keep the longest span (`arm64` beats `arm`;
`arm-` beats `arm` on rustc triples). Bare `arm` means ARM32 except in macOS
names or exclusive Mac formats. `linux`+`android` names target Android.
An untagged archive is unknown, not automatically source code. Explicit bounded
`source`/`src` tokens still identify source archives.
Edit this file, then copy it to:

- `backend/picker/catalog.json` (`go:embed` for the container image)
- `frontend/lib/picker-catalog.json` (Vercel frontend root cannot see `shared/`)

Tests fail if the three copies drift.

**Real-release corpus:** `corpus.json` — snapshots of public GitHub releases with
labeled picks and independent `targetLabels`. `go test ./picker/ -run
TestCorpusMetrics -v` reports top-1, abstention, false auto-select,
wrong-platform, wrong-CPU, and unlabeled-selection counts. The metrics do not
ask the classifier to verify its own output. Mark a case
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

README install commands are parsed from Markdown structure and statically
checked shell syntax. Headings and nearby text constrain OS/CPU targets;
build steps stay out, stated prerequisites remain attached, and multiline
commands keep their original text. Browser client hints and manual target
controls share one target across the CTA, download list, and install methods.
