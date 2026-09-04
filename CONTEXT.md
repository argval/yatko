# Yatko domain glossary

Terms used when talking about architecture and product behavior. Prefer these names over handler or file names.

## Asset Picker

Chooses the single best GitHub release asset for a visitor’s **platform** (OS) and **arch** (CPU). Used by `/dl` redirects and by the release-page download button so both resolve to the same binary.

Canonical implementation: Go module `backend/picker` (`Classify` → `DecideAsset`).
`/dl` and `/api/link` auto-select only when confidence is not low. The release-page
download button points at `/dl` as soon as platform/arch is known (Go decides on
click). Filename and checksum come from the `picks` table embedded in
`/api/release` (same `DecideAsset` results) so they paint without a second RTT;
`/api/link` remains a fallback when that table is absent. The TypeScript
`classify` helper is only for All-downloads labels. Shared fixtures in
`shared/picker/fixtures.json` are exercised by Go tests.

## Platform

Visitor OS for asset matching: `windows`, `macos`, `linux`, `android`, `ios`, or `unknown` (Go). Unknown yields no pick and `/dl` falls back to the GitHub release page.

## Arch

Visitor CPU for asset matching: `amd64`, `arm64`, `arm`, `386`, or empty/unknown (ignore arch filter).

## HTTP Rate Limit

Per-IP fixed-window throttle on public API and `/dl` routes. Own module (`backend/ratelimit`); not part of Cache — they only share Redis as transport.

## Search Autocomplete

Homepage repo typeahead: normalize to slug or bare; slug hits one Search + GetRepo ensure; bare dual-fetches owner browse + keyword and merges. Own module (`backend/search`); the HTTP handler is wire-only.

## Install Command

A shell one-liner scraped from README fences, tagged with an install platform (`macos` / `windows` / `linux` / `universal`). Types and extraction live in the pure Install Command module; the client card is a presentation adapter that imports downward.

## Checksum Map

Filename → hash map parsed from a release checksum manifest. Pure module (`parse-checksums.ts`); `getChecksums` is the fetch adapter. Display (`AssetChecksum`) only truncates and copies.
