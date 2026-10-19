# Asset Picker — shared contract

**Deep module:** `backend/picker` (Go) combines filename clues and observed
artifact metadata, then returns an `AssetDecision`. Known OS, CPU, and explicit
libc conflicts are excluded before format preferences. A CPU token alone does
not prove OS compatibility: OS evidence filters before CPU evidence, so an
OS-unknown `app-arm64.tar.gz` cannot displace a confirmed-OS `App.dmg`.
Companion `cli` builds only compete when nothing else fits the OS. On macOS a
universal build wins over single-CPU builds. Cross-CPU fallbacks come only
from the `emulation` table in `decide.go`, closest first: Windows x64 → x86;
Windows ARM64 → x64 → x86; Apple Silicon → Intel (Rosetta). A native
build ranks above an emulated one. A known compatible CPU ranks above an
unknown CPU, and nothing falls back
"upward" (x86 never gets x64). Bare `x86` means 32-bit except on macOS, where
it means Intel 64-bit. Equal candidates abstain instead of using asset order. Decisions carry `selected`, `incompatible`, `unknown`, or `ambiguous`
status, confidence, reasons, and alternatives.

**Primary decider:** with `TYPESAFE_API_KEY` set, Jev (`backend/jev`) chooses
among the release's real files for the visitor's UA, client hints and explicit
choices; this rule picker answers only when Jev errors, replies malformed or is
slow. Code guards every Jev pick with the same artifact facts (`ValidateSelection`)
and Jev never supplies a URL. Without a key, this picker is primary.

`/dl` and `/api/link` auto-select only when confidence is not `low`. In
rule-only mode the page reads the `picks` map on `/api/release`; with Jev it asks
`/api/link` with the visitor's context. Clicks go through `/dl` with the displayed
filename in `?asset=`, which must still be the validated choice for that visitor;
the handler cannot silently switch to another file as inspection fills the cache. Independent page/link requests can still see different evidence
while the cache fills. A known visitor CPU requires compatible CPU evidence,
even for an OS-specific installer. An unspecified visitor CPU remains unspecified.

**Shared facts:** each API asset includes `artifact` with supported target
tuples (`platform`, `arch`, `libc`), role, evidence, inspection status, and
conflicts. Missing fields remain unknown. Targets are tuples so metadata for
Windows/x86 and macOS/ARM cannot become an invented Windows/ARM build.
All Downloads uses these facts for labels and filters; an asset without them
matches no target. The browser never ranks assets.

**Observations:** the GitHub client reads published cargo-dist, Electron, and
Tauri metadata and bounded file contents. Entries must refer to assets in the
selected release (by name, download URL, or GitHub REST asset URL). File inspection never executes or writes downloaded files.
The handler caps enrichment at three seconds and four live content
inspections per request; cached observations are free, and OS-unknown assets
are probed first, then `.apk`, then CPU-unknown. A timeout is not a failure:
the asset keeps its filename and manifest facts.
Each inspection allows six requests, 4 MiB of downloaded bytes, 8 MiB of
expanded content, and two binary entries. Observations are cached by asset
identity/update time/digest; failures remain explicit.

Content readers support ELF, PE, Mach-O, ZIP, tar, gzip/bzip2/xz/zstd, 7z,
RPM and MSI headers, DMG trailers (macOS, CPU unknown), APK (Android vs
Alpine), and Debian control metadata. xz BCJ filter chains and encrypted 7z
stay unsupported. Static ELF headers can identify CPU without
proving Linux; missing OS stays unknown. Known unsupported CPUs and operating
systems and byte order remain constraints. Recognized libraries, debug files,
and updater sidecars stay out of the primary download. Cargo-dist, Electron, and Tauri
(`latest.json`, including `OS-ARCH-BUNDLE` keys) are the supported publisher
schemas; unknown cargo-dist kinds and custom Tauri keys stay opaque.

This is structured evidence processing; Jev decides from these facts and the
filename. README context still uses a fixed vocabulary, so unfamiliar headings
can be missed.

**Alias table:** `catalog.json` — platforms, architectures, libc, variants,
formats. Overlapping arch aliases keep the longest span (`arm64` beats `arm`;
`arm-` beats `arm` on rustc triples). Bare `arm` means ARM32 except in macOS
names or exclusive Mac formats. `linux`+`android` names target Android.
An untagged archive is unknown, not automatically source code. Explicit bounded
`source`/`src` tokens still identify source archives.
Edit this file, then copy it to `backend/picker/catalog.json` (`go:embed` for
the container image). Tests fail if the copies drift.

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
2. Copy the catalog to the backend path above.
3. Run `go test ./picker/` and `bun test` in `frontend/`.

Optional fixture fields: `prefer` (extension key), `libc` (`musl` | `gnu` |
`static`), and `userAgent` (Linux deb/rpm tiebreak when `prefer` is unset).

Do not add browser-only ranking shortcuts (keyword fallbacks, softer arch
filters) — they create silent `/dl` vs button drift.

Jev may choose only among the release's real assets that pass the code guard;
no model or enricher ever invents a download URL.

README install commands are parsed from Markdown structure and statically
checked shell syntax. Headings and nearby text constrain OS/CPU targets;
build steps stay out, stated prerequisites remain attached, and multiline
commands keep their original text. Browser client hints and manual target
controls share one target across the CTA, download list, and install methods.


## Coverage limits and verification

The filename catalog also recognizes Flatpak, Snap, MSIX, APPX, and tar. A format
name alone proves no CPU support. Android APK inspection currently establishes
Android from its manifest, but does not decode the native CPU inventory. Two
APK corpus cases therefore record known abstentions for specific CPUs.

Companion detection recognizes bounded `cli` and `inso` tokens. Glued names such
as `HandBrakeCLI` and a proxy sharing a release still need publisher roles or
more product context. The resolver must not invent repo-specific aliases from a
binary header. Encrypted archives, unsupported 7z filter chains (including BCJ),
and oversized inspections can remain unknown. Windows installer stub CPU
evidence does not establish every payload requirement.

README extraction does not verify script host ownership or crawl linked install
documentation. It checks supported HTTPS fetch-and-run syntax, preserves stated
targets, and excludes known build/usage sections. Unknown terminology can still
be missed. The UI asks visitors to review README commands before running them.

The final coverage pass includes 139 frontend tests, the full backend race suite,
Go vet/build, TypeScript, and the frontend production build. The frozen release
corpus has 18 releases and 127 target cases: 107 selections, 20 abstentions, two
known APK misses, and no false auto-selections or wrong-OS/CPU selections. These
are fixture results, not a claim that all public releases are covered. Independent
live audits covered 36 release repositories and 35 READMEs; regression probes
cover the defects found there. ZIP/APK preflight now rejects hostile directory
counts before `archive/zip` allocation, including through the public inspectors.
