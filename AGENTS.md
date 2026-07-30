## Learned User Preferences

- Keep README feature- and marketing-focused; avoid documenting caching, rate limiting, and deployment internals.
- Emphasize the one-click URL swap (`github.com/owner/repo` → `yatko.app/owner/repo`) in user-facing docs.
- Do not reintroduce download badges unless explicitly requested.
- Open repo and other external links in a new tab.
- Homepage navigation (Go, examples, suggestions) should show immediate loading feedback so clicks feel registered.
- Prefer `@tailwindcss/typography` prose classes over long hand-rolled arbitrary-variant markdown style strings.
- Prefer a small local `icons.tsx` over adding `lucide-react` unless the icon set grows substantially.
- Land architectural deepen/refactor work on feature branches targeting `architecture-review`, not straight onto `main`.
- When asked to ship to production, prefer landing on `main` first, then merging/pushing to the `prod` branch.
- Keep homepage and github-swap release URLs (`/:owner/:repo`) crawlable; disallow `/api/` and `/dl/`; general crawlers may disallow `/p/`, but social preview bots must stay allowlisted for unfurls.
- Share and copy “landing page” links as `yatko.app/{owner}/{repo}` (not `/p/...`) so Twitter/OG cards resolve.

## Learned Workspace Facts

- Yatko is positioned as a drop-in release-download URL: replace `github.com` with `yatko.app` for the same owner/repo path.
- Release-page markdown (blurb, notes, About) goes through shared `RepoMarkdown` in `frontend/app/p/[owner]/[repo]/markdown.tsx` (GFM, raw HTML, sanitize, URL rewrite) with `@tailwindcss/typography`.
- `architecture-review` is the integration branch for architecture deepen PRs.
- Production deploys track the `prod` branch (typically merged from `main`).
- Search cache uses a longer soft TTL with prefix reuse; an in-process L1 LRU sits in front of Redis for hot keys.
- Install-command extraction from README fences must accept both CommonMark triple-backtick and tilde (`~~~`) fences.
- Bare versioned tarballs (e.g. `.tar.xz`) with no OS/arch token are treated as source archives, not installable binaries, in both the Go picker and the frontend.
- Release checksums come from downloadable checksum assets (names matching checksum/sha*sums or `*.sha256` / `*.sha512` / `*.md5`), fetched and parsed into a filename→hash map.
- Production Redis is Upstash via Vercel Marketplace; the Go backend prefers `REDIS_URL`, then `KV_URL`, then `UPSTASH_REDIS_URL`.
- HTTP rate limiting uses process-local windows when Redis is unset or unreachable (does not fail open); `/health` stays HTTP 200 with redis/rate_limit/github budget fields (`github_token` boolean only).
- Crawling is configured in `frontend/app/robots.ts`: allow `/`, disallow `/api/` and `/dl/`; general crawlers also disallow `/p/`; social preview bots are allowlisted for unfurls. Site includes `/privacy` plus a quiet footer with a GitHub non-affiliation disclaimer.
- Vercel Container Registry for the Go backend is capped at 50 images; a full registry blocks deploys until unused images are pruned.

## Cursor Cloud specific instructions

Two services (see `CLAUDE.md` for the full command list and architecture): Go backend in `backend/` on `:8080`, Next.js frontend in `frontend/` on `:3000`. `./dev.sh` runs both together; end-to-end testing needs both.

- Toolchain: the backend needs Go 1.25+ (`backend/go.mod` pins `go 1.25.1`) and the frontend uses **Bun**, not npm. The VM snapshot ships Go 1.25 (symlinked at `/usr/local/bin/go`, ahead of the older distro `/usr/bin/go`) and Bun (`/usr/local/bin/bun`); the update script only refreshes project deps.
- No env vars are required for local dev: `BACKEND_URL` defaults to `http://localhost:8080`, and the backend hits GitHub's public API unauthenticated. Redis is fully optional (cache no-ops without it; rate limiting falls back to process-local windows). Go tests use embedded miniredis.
- Non-obvious caveat: heavy testing against real repos can hit GitHub's anonymous rate limits (60 req/hr). Set `GITHUB_TOKEN` (classic PAT, no scopes) on the backend to raise limits if resolving many repos.
- Frontend has no lint script; `bun test` is the only frontend test entry. Backend: `go test ./...` and `go build ./...`.
