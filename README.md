# Yatko

**Clean download links for any public GitHub release.**

Skip the releases page. Give users a one-click download that automatically detects their platform and architecture.

```
https://yatko.app/dl/cli/cli
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
- **Redis cache** — ETag-revalidated cache via Upstash, graceful no-op fallback for local dev
- **Per-IP rate limiting** — protects the API from abuse, independent of the GitHub cache

See [Caching & GitHub rate limits](#caching--github-rate-limits) for how caching and rate limiting actually work.

## Stack

| Layer | Tech |
|-------|------|
| Backend | Go + Gin, deployed as a Vercel container-image Service |
| Frontend | Next.js 16 (App Router, React 19), deployed as a Vercel Service |
| Cache | Upstash Redis (serverless) |
| Styling | Tailwind CSS v4 |

Both services live in one Vercel project and share a domain — see [Deployment](#deployment).

## Deployment

One Vercel project, two [Services](https://vercel.com/docs/services) sharing the `yatko.app` domain:

- `frontend/` — the Next.js app, Vercel's default Node.js runtime
- `backend/` — the Go API, built as a container image (`backend/Dockerfile`)

Root-level `vercel.json` defines both services and the rewrites that route `/dl`, `/badge`, `/api`, `/health` to the backend and everything else to the frontend.

- **`prod`** is the production branch — only commits here deploy to `yatko.app`.
- **`main`** is the default working branch — pushes here build Preview deployments only and never touch production. Fast-forward `main` into `prod` (or open a PR) to ship.

## Project structure

```
yatko/
├── backend/
│   ├── github/       # GitHub API client (releases, README)
│   ├── cache/        # Redis cache layer
│   ├── picker/       # Platform + arch asset selection logic
│   ├── middleware/   # Per-IP rate limiting
│   ├── handlers/     # Gin route handlers
│   └── main.go
└── frontend/
    └── app/
        ├── page.tsx                    # Homepage with search
        └── p/[owner]/[repo]/
            ├── page.tsx                # Release landing page
            ├── release-page.tsx        # Shared landing-page layout
            ├── [version]/page.tsx      # Versioned landing page
            ├── download-button.tsx     # Platform-aware download button
            ├── download-section.tsx    # Button + checksum wrapper
            ├── version-selector.tsx    # Version dropdown
            ├── prerelease-toggle.tsx   # Pre-release opt-in
            ├── all-downloads.tsx       # Filterable asset list
            ├── install-commands.tsx    # Copyable install commands
            ├── share-links.tsx         # Copyable Yatko URLs
            ├── asset-checksum.tsx      # SHA256 display
            ├── status-card.tsx         # Release status/metadata card
            ├── collapsible-card.tsx    # Shared collapsible section
            ├── opengraph-image.tsx     # OG image generation
            ├── twitter-image.tsx       # Twitter card image
            ├── use-releases.ts         # Shared releases hook
            ├── use-copy.ts             # Clipboard-copy hook
            └── platform-utils.ts       # Platform/arch detection
```

## Running locally

Run both services together:

```bash
./dev.sh
```

Or start each independently:

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
RATE_LIMIT_RPM=120             # Per-IP request budget per minute (also skipped if UPSTASH_REDIS_URL unset)
PORT=8080
```

### Frontend

```bash
cd frontend
bun install
bun run dev
```

Environment variables:

```env
BACKEND_URL=http://localhost:8080  # Server-side only — every backend call is server-side, there's no client-side fetch
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

This is separate from the **per-IP HTTP rate limit** (`RATE_LIMIT_RPM`, default 120 req/min) that protects the API itself from abuse — it also no-ops without `UPSTASH_REDIS_URL`.

## Badge

```markdown
![version](https://yatko.app/badge/owner/repo)
```
