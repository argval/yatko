# Yatko domain glossary

Terms used when talking about architecture and product behavior. Prefer these names over handler or file names.

## HTTP Rate Limit

Per-IP fixed-window throttle on public API and `/dl` routes. Own module (`backend/ratelimit`); not part of Cache — they only share Redis as transport.

## Search Autocomplete

Homepage repo typeahead: normalize query, reuse shorter warm cache prefixes while typing, fetch/warm via GitHub Search. Own module (`backend/search`); the HTTP handler is wire-only.

## Install Command

A shell one-liner scraped from README fences, tagged with an install platform (`macos` / `windows` / `linux` / `universal`). Types and extraction live in the pure Install Command module; the client card is a presentation adapter that imports downward.
