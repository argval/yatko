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
- Secondary pages (e.g. privacy) should use the same fixed top-left Yatko back control as release pages (`BackToYatko`), not a one-off back link.

## Learned Workspace Facts

- Yatko is positioned as a drop-in release-download URL: replace `github.com` with `yatko.app` for the same owner/repo path. Production primary host is apex `yatko.app`; `www.yatko.app` redirects to apex (308).
- Release-page markdown (blurb, notes, About) goes through shared `RepoMarkdown` in `frontend/app/p/[owner]/[repo]/markdown.tsx` (GFM, raw HTML, sanitize, URL rewrite) with `@tailwindcss/typography`.
- `architecture-review` is the integration branch for architecture deepen PRs.
- Production deploys track the `prod` branch via Vercel (previews on PRs/branches); GitHub Actions CI runs backend/frontend tests and builds on push/PR and does not deploy.
- Homepage search is slug vs bare: `owner/repo` (or repo URL) → `user:owner in:name <repo>` + GetRepo ensure; bare token or owner URL → dual Search (`user:<q>` + quoted `in:name`), merge, rank exact repo name then owned-by-q then stars. Dashes never choose a path. Always `archived:false`. Cache key `search:v8:`.
- Install-command extraction from README fences must accept both CommonMark triple-backtick and tilde (`~~~`) fences.
- Bare versioned archives (e.g. `.tar.xz`, `.zip`) with no OS/arch token are treated as source archives, not installable binaries, in both the Go picker and the frontend. Platform/arch-tagged zips stay eligible.
- Release checksums come from downloadable checksum assets (names matching checksum/sha*sums or `*.sha256` / `*.sha512` / `*.md5`), fetched and parsed into a filename→hash map.
- Production Redis is Upstash via Vercel Marketplace over the Redis protocol (`REDIS_URL`, then `KV_URL`, then `UPSTASH_REDIS_URL`); do not switch the Go backend to Upstash REST/`KV_REST_API_*`.
- HTTP rate limiting uses process-local windows when Redis is unset or unreachable (does not fail open); `/health` stays HTTP 200 with redis/rate_limit/github budget fields (`github_token` boolean only).
- Crawling is configured in `frontend/app/robots.ts`: allow `/`, disallow `/api/` and `/dl/`; general crawlers also disallow `/p/`; social preview bots are allowlisted for unfurls. Release OG images live at `/{owner}/{repo}/opengraph-image` (not under `/p/`) so the general `/p/` disallow does not block card images. Site includes `/privacy` (GitHub non-affiliation lives there); footer is Privacy + Source only.
- Vercel Container Registry for the Go backend is capped at 50 images; a full registry blocks deploys until unused images are pruned.

## Cursor Cloud specific instructions

Two services (see `CLAUDE.md` for the full command list and architecture): Go backend in `backend/` on `:8080`, Next.js frontend in `frontend/` on `:3000`. `./dev.sh` runs both together; end-to-end testing needs both.

- Toolchain: the backend needs Go 1.25+ (`backend/go.mod` pins `go 1.25.1`) and the frontend uses **Bun**, not npm. The VM snapshot ships Go 1.25 (symlinked at `/usr/local/bin/go`, ahead of the older distro `/usr/bin/go`) and Bun (`/usr/local/bin/bun`); the update script only refreshes project deps.
- No env vars are required for local dev: `BACKEND_URL` defaults to `http://localhost:8080`, and the backend hits GitHub's public API unauthenticated. Redis is fully optional (cache no-ops without it; rate limiting falls back to process-local windows). Go tests use embedded miniredis.
- Non-obvious caveat: heavy testing against real repos can hit GitHub's anonymous rate limits (60 req/hr). Set `GITHUB_TOKEN` (classic PAT, no scopes) on the backend to raise limits if resolving many repos.
- Frontend has no lint script; `bun test` is the only frontend test entry. Backend: `go test ./...` and `go build ./...`.
