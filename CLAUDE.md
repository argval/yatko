# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Yatko turns a GitHub repo into clean, permanent download links (`yatko.app/dl/:owner/:repo`) that auto-detect the visitor's platform/arch and redirect to the right release asset. One Vercel project, deployed as two [Services](https://vercel.com/docs/services) sharing a domain:

- `backend/` — Go + Gin API, deployed as a container-image service (`backend/Dockerfile`)
- `frontend/` — Next.js 16 (App Router, React 19), deployed as the default Node.js service

Root-level `vercel.json` defines both services and the rewrites that route `/dl`, `/api`, `/health` to the backend and everything else to the frontend. See the root `README.md` for the public route table and feature list — don't duplicate that here.

## Commands

Run both services together for local dev:

```bash
./dev.sh
```

### Backend (`backend/`)

```bash
go run .                          # start the API on :8080
go test ./...                     # run all tests
go test ./cache/ -run TestName    # run a single test
go build ./...                    # compile check
```

Useful local env vars: `GITHUB_TOKEN` (raises GitHub rate limit from 60→5000 req/hr), `UPSTASH_REDIS_URL` (cache no-ops without it), `CACHE_TTL_SECONDS`, `PORT`.

### Frontend (`frontend/`)

```bash
bun install
bun run dev      # Next.js dev server on :3000
bun run build
bun run start
```

Requires `BACKEND_URL` (server-side only — every backend call in `frontend/` is server-side, there's no client-side fetch to it) pointing at the backend. In production this is injected automatically by the Vercel service binding declared in `vercel.json`; for local dev outside `./dev.sh`, set it to `http://localhost:8080`.

There is no lint script and no test runner configured on the frontend.

## Backend architecture

Request flow for a download: `main.go` wires a single `github.Client` and `cache.Cache` into each handler; handlers stay thin and delegate to these packages.

- **`github/`** — GitHub REST API client (releases by tag/latest, README fetch). Returns `*github.APIError` with a `StatusCode` on failure; `handlers/errors.go` maps that to the outward-facing HTTP status (404/403/429/502).
- **`cache/`** — Redis-backed cache wrapping every GitHub fetch through `cache.FetchCached`. Falls back to a graceful no-op when `UPSTASH_REDIS_URL` is unset, so it works without Redis in local dev. Design (read before changing cache behavior):
  - **Soft TTL** (`CACHE_TTL_SECONDS`, default 15 min): serve cached value as-is, zero network calls.
  - **Revalidation**: after soft TTL, next request uses `If-None-Match`; a `304` does not count against GitHub's rate limit and extends freshness.
  - **Hard TTL** (24h): how long a stale entry (and its ETag) is kept so revalidation stays possible.
  - **Request coalescing**: concurrent misses for the same key are deduplicated via `singleflight`.
  - **Stale-while-error**: if GitHub is unreachable or rate-limited, serve the last known-good value when one exists.
  - **Rate-limit budget guard**: the GitHub client refuses new requests once `X-RateLimit-Remaining` drops below a reserve (200), so bursts of cold repos fail fast with 429 instead of starving already-cached ones.
- **`picker/`** — pure platform/arch detection and asset-matching logic (User-Agent parsing → OS/arch → best-matching release asset). No I/O.
- **`handlers/`** — Gin route handlers, one file per route family (`redirect`, `link`, `page`, `releases`). Each handler fetches a release via `cache.FetchCached(ctx, cache, key, ghClientCall)`, then hands assets to `picker` if it needs to pick one.
- **`middleware/`** — per-IP HTTP rate limit (`RATE_LIMIT_RPM`, default 120/min); also no-ops without `UPSTASH_REDIS_URL`. Separate from the GitHub cache/rate-limit budget above.

No CORS policy: frontend and backend are Vercel services sharing one origin, so only same-origin requests ever reach the API. `main.go` trusts the `X-Forwarded-For` header for the per-IP rate limiter's client identity — safe because Vercel's edge overwrites that header and never forwards a client-supplied value.

## Frontend architecture

App Router structure: `app/page.tsx` is the homepage/search; `app/p/[owner]/[repo]/` is the release landing page, with `[version]/page.tsx` for a pinned version. Route-scoped components (download button, version selector, checksum display, install-command extraction, share links, etc.) live alongside the pages they belong to rather than in a shared `components/` tree — `use-releases.ts` and `platform-utils.ts` in that directory are the shared hook/util for that route family.

The frontend calls the Go backend (`BACKEND_URL`/`NEXT_PUBLIC_BACKEND_URL`), not GitHub directly.

**Important:** `frontend/AGENTS.md` (pulled in via `frontend/CLAUDE.md`) warns that this Next.js version has breaking API/convention changes from training data — check `node_modules/next/dist/docs/` before writing Next.js-specific code in `frontend/`.
