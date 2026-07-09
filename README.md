# Yoink

**Clean download links for any public GitHub release.**

Skip the releases page. Give users a one-click download that automatically detects their platform and architecture.

```
https://yoink.dev/dl/cli/cli
```

---

## What it does

Most GitHub projects bury downloads in a releases page with 15+ assets. Yoink solves this with smart URLs that do the right thing automatically:

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
- **Share links** — copyable Yoink URLs for smart download, landing page, badge, and API
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
yoink/
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
            ├── share-links.tsx         # Copyable Yoink URLs
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
FRONTEND_ORIGIN=https://...    # Extra CORS origin
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

Without `GITHUB_TOKEN`, GitHub allows 60 unauthenticated requests/hour **per IP** — easy to exhaust while testing multiple repos back-to-back. Setting `GITHUB_TOKEN` raises that to 5,000/hr, and thanks to the caching behavior above, that's enough headroom for a large number of actively-shared links, even at high traffic, as long as `UPSTASH_REDIS_URL` is configured in production (without it, caching silently no-ops and every request hits GitHub directly — fine for local dev, not for production).

## Deploying

### Backend (Fly.io)

```bash
cd backend
fly launch        # first time
fly deploy        # subsequent deploys
fly secrets set GITHUB_TOKEN=ghp_... UPSTASH_REDIS_URL=rediss://...
```

### Frontend (Vercel)

Push to `main` — Vercel auto-deploys. Set in the Vercel dashboard:

```
BACKEND_URL=https://your-backend.fly.dev
NEXT_PUBLIC_BACKEND_URL=https://your-backend.fly.dev
```

## API examples

```bash
# Resolved download URL as JSON
curl https://yoink.dev/api/link/cli/cli
# {"url":"...","filename":"gh_2.x.x_macOS_arm64.zip","platform":"macos","arch":"arm64","version":"v2.x.x"}

# Override platform and arch
curl "https://yoink.dev/api/link/cli/cli?platform=linux&arch=amd64"

# Specific version
curl https://yoink.dev/api/link/cli/cli/v2.40.0

# List recent releases
curl https://yoink.dev/api/releases/cli/cli

# Direct download (follows redirect)
curl -L https://yoink.dev/dl/cli/cli -o gh.zip
```

## Badge

```markdown
![version](https://yoink.dev/badge/owner/repo)
```
