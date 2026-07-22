# Yatko domain glossary

Terms used when talking about architecture and product behavior. Prefer these names over handler or file names.

## Asset Picker

Chooses the single best GitHub release asset for a visitor’s **platform** (OS) and **arch** (CPU). Used by `/dl` redirects and by the release-page download button so both resolve to the same binary.

Canonical implementation: Go module `backend/picker`. The TypeScript port is a thin same-algorithm adapter for the browser. Shared golden fixtures in `shared/picker/fixtures.json` are the cross-runtime test surface — both adapters must pass them.

## Platform

Visitor OS for asset matching: `windows`, `macos`, `linux`, or `unknown` (Go). Unknown yields no pick and `/dl` falls back to the GitHub release page.

## Arch

Visitor CPU for asset matching: `amd64`, `arm64`, `arm`, `386`, or empty/unknown (ignore arch filter).
