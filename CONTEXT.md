# Yatko domain glossary

Terms used when talking about architecture and product behavior. Prefer these names over handler or file names.

## Asset Picker

Chooses the single best GitHub release asset for a visitor’s **platform** (OS) and **arch** (CPU). Used by `/dl` redirects and by the release-page download button so both resolve to the same binary.

Canonical implementation: Go module `backend/picker`. The TypeScript port is a thin same-algorithm adapter for the browser. Shared golden fixtures in `shared/picker/fixtures.json` are the cross-runtime test surface — both adapters must pass them.

## Platform

Visitor OS for asset matching: `windows`, `macos`, `linux`, or `unknown` (Go). Unknown yields no pick and `/dl` falls back to the GitHub release page.

## Arch

Visitor CPU for asset matching: `amd64`, `arm64`, `arm`, `386`, or empty/unknown (ignore arch filter).

## HTTP Rate Limit

Per-IP fixed-window throttle on public API and `/dl` routes. Own module (`backend/ratelimit`); not part of Cache — they only share Redis as transport.

## Search Autocomplete

Homepage repo typeahead: normalize query, reuse shorter warm cache prefixes while typing, fetch/warm via GitHub Search. Own module (`backend/search`); the HTTP handler is wire-only.

## Install Command

A shell one-liner scraped from README fences, tagged with an install platform (`macos` / `windows` / `linux` / `universal`). Types and extraction live in the pure Install Command module; the client card is a presentation adapter that imports downward.

## Checksum Map

Filename → hash map parsed from a release checksum manifest. Pure module (`parse-checksums.ts`); `getChecksums` is the fetch adapter. Display (`AssetChecksum`) only truncates and copies.
