<!-- deploy-gating test commit: confirms main no longer triggers production, safe to ignore -->

# Yatko

**Clean download links for any public GitHub release.**

Skip the releases page. Give users a one-click download that automatically detects their platform and architecture.

```
https://yatko.dev/dl/cli/cli
```

---

## What it does

Most GitHub projects bury downloads in a releases page with 15+ assets. Yatko solves this with smart URLs that do the right thing automatically:

| URL | What it does |
|-----|-------------|
| `/dl/:owner/:repo` | Detects platform + arch, redirects to the right binary |
| `/dl/:owner/:repo/:version` | Same, but for a specific release tag |
| `/p/:owner/:repo` | Landing page with download button, release notes, all assets |
| `/p/:owner/:repo/:version` | Landing page for a specific version |
| `/badge/:owner/:repo` | Dynamic SVG version badge for READMEs |
| `/api/link/:owner/:repo` | JSON with resolved download URL — for CI/scripts |
| `/api/releases/:owner/:repo` | List of recent releases (tag, date, prerelease flag) |

## Features

- **Platform detection** — Windows / macOS / Linux from User-Agent
- **Architecture detection** — amd64 / arm64 / arm / 386 from User-Agent and `navigator.userAgentData`
- **Version selector** — browse and switch between recent releases in the UI
- **Pre-release toggle** — opt into alpha/beta builds when available
- **Asset checksums** — automatically fetches and displays SHA256 for the selected binary
- **Quick Install** — extracts package manager commands from the README (`pip`, `npm`, `cargo`, `brew`, `winget`, `choco`, `scoop`, `apt`, and more)
- **Platform filter** — "My platform only" toggle in the All Downloads list
- **Download counts** — shows per-asset download counts from GitHub
- **Share links** — copyable Yatko URLs for smart download, landing page, badge, and API
- **Version badges** — embed in any README
- **Dark mode** — system preference detection + manual toggle
- **Redis cache** — ETag-revalidated cache via Upstash (see [Caching & GitHub rate limits](#caching--github-rate-limits)), graceful no-op fallback for local dev

## Stack

| Layer | Tech |
|-------|------|
| Backend | Go + Gin, deployed on Fly.io |
| Frontend | Next.js 16 (App Router, React 19), deployed on Vercel |
| Cache | Upstash Redis (serverless) |
| Styling | Tailwind CSS v4 |

## Project structure

```
yatko/
├── backend/
│   ├── github/       # GitHub API client (releases, README)
│   ├── cache/        # Redis cache layer
│   ├── picker/       # Platform + arch asset selection logic
│   ├── handlers/     # Gin route handlers
│   └── main.go
└── frontend/
    └── app/
        ├── page.tsx                    # Homepage with search
        └── p/[owner]/[repo]/
            ├── page.tsx                # Release landing page
            ├── [version]/page.tsx      # Versioned landing page
            ├── download-button.tsx     # Platform-aware download button
            ├── download-section.tsx    # Button + checksum wrapper
            ├── version-selector.tsx    # Version dropdown
            ├── prerelease-toggle.tsx   # Pre-release opt-in
            ├── all-downloads.tsx       # Filterable asset list
            ├── install-commands.tsx    # Copyable install commands
            ├── share-links.tsx         # Copyable Yatko URLs
            ├── asset-checksum.tsx      # SHA256 display
            ├── use-releases.ts         # Shared releases hook
            └── platform-utils.ts      # Platform/arch detection
```

## Running locally

### Backend

```bash
cd backend
go run .
```

Optional environment variables:

```env
GITHUB_TOKEN=ghp_...           # Raises rate limit from 60 to 5000 req/hr
UPSTASH_REDIS_URL=rediss://... # Redis cache (skipped if unset)
CACHE_TTL_SECONDS=900          # How long a cached value is served before revalidating (default 15 min)
PORT=8080
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

Environment variables:

```env
BACKEND_URL=http://localhost:8080            # Server-side fetch
NEXT_PUBLIC_BACKEND_URL=http://localhost:8080 # Client-side fetch (version selector)
```

## Caching & GitHub rate limits

Every unique `owner/repo` (and `owner/repo@tag`) is cached independently, so traffic volume to a *single* shared link barely affects GitHub API usage — a link getting hit by thousands of visitors in the same cache window still costs at most a handful of GitHub requests, not one per visitor.

- **Soft TTL** (`CACHE_TTL_SECONDS`, default 15 min): how long a cached value is served as-is, with zero network calls.
- **Revalidation**: once the soft TTL lapses, the next request revalidates via a conditional GitHub request (`If-None-Match`). If nothing changed, GitHub returns `304 Not Modified` — which **does not** count against the rate limit — and the cached value's freshness window is simply extended.
- **Hard TTL** (24h): how long a stale entry (and its ETag) is kept around in Redis so revalidation stays possible instead of falling back to a full fetch.
- **Request coalescing**: concurrent cache misses for the same key are deduplicated (via `singleflight`), so a sudden burst of traffic to a cold link triggers one GitHub fetch, not N.
- **Stale-while-error**: if GitHub is unreachable or rate-limited, the last known-good cached value is served instead of an error, as long as one exists.
- **Rate-limit budget guard**: the GitHub client tracks the last-seen `X-RateLimit-Remaining` and refuses new requests once headroom drops below a reserve (200). Combined with stale-while-error, this means a burst of *never-before-seen* repos hitting an exhausted quota fails fast with a 429 instead of also starving already-cached repos, which keep serving their last known-good value uninterrupted.

Without `GITHUB_TOKEN`, GitHub allows 60 unauthenticated requests/hour **per IP** — easy to exhaust while testing multiple repos back-to-back. Setting `GITHUB_TOKEN` raises that to 5,000/hr, and thanks to the caching behavior above, that's enough headroom for a large number of actively-shared links, even at high traffic, as long as `UPSTASH_REDIS_URL` is configured in production (without it, caching silently no-ops and every request hits GitHub directly — fine for local dev, not for production).

## Badge

```markdown
![version](https://yatko.dev/badge/owner/repo)
```
