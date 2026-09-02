## Learned User Preferences

- Keep README feature- and marketing-focused; avoid documenting caching, rate limiting, and deployment internals.
- Emphasize the one-click URL swap (`github.com/owner/repo` → `yatko.app/owner/repo`) in user-facing docs; homepage “How it works” should lead with a Swap the domain card and no section subtitle.
- Keep the store-style “Get it on Yatko” README embed (`/badge.svg`, Share → Embed button) linking to `yatko.app/{owner}/{repo}`; do not reintroduce other download-count/shields badges unless asked.
- Open repo and other external links in a new tab.
- Homepage navigation (Go, examples, suggestions) should show immediate loading feedback so clicks feel registered.
- Prefer `@tailwindcss/typography` prose classes over long hand-rolled arbitrary-variant markdown style strings, a small local `icons.tsx` for UI glyphs, and imported official brand marks (e.g. X via `react-icons` in `frontend/`) rather than hand-drawn SVGs; don’t add `lucide-react` unless the icon set grows substantially.
- Land architectural deepen/refactor work on feature branches targeting `architecture-review`, not straight onto `main`.
- When asked to ship to production, prefer landing on `main` first, then merging/pushing to the `prod` branch.
- Keep homepage and github-swap release URLs (`/:owner/:repo`) crawlable; disallow `/api/` and `/dl/`; general crawlers may disallow `/p/`, but social preview bots must stay allowlisted for unfurls.
- Share and copy “landing page” links as `yatko.app/{owner}/{repo}` (not `/p/...`) so Twitter/OG cards resolve.
- Secondary pages (e.g. privacy) should use the same fixed top-left Yatko back control as release pages (`BackToYatko`), not a one-off back link.
- CLI/install-command platform filtering should match the download button’s OS/arch detection; keep official `curl | bash` / `iex` one-liners (still reject `$()`, backticks, and `&&`); drop generic README build steps such as bare `npm`/`yarn`/`pnpm install` (keep `npm install -g`).

## Learned Workspace Facts

- Yatko is positioned as a drop-in release-download URL: replace `github.com` with `yatko.app` for the same owner/repo path. Production primary host is apex `yatko.app`; `www.yatko.app` redirects to apex (308).
- Release-page markdown (blurb, notes, About) goes through shared `RepoMarkdown` in `frontend/app/p/[owner]/[repo]/markdown.tsx` (GFM, raw HTML, sanitize, URL rewrite) with `@tailwindcss/typography`.
- Production deploys track the `prod` branch via Vercel (previews on PRs/branches); GitHub Actions CI runs backend/frontend tests and builds on push/PR and does not deploy.
- Homepage search is slug vs bare: `owner/repo` (or repo URL) → `user:owner in:name <repo>` + GetRepo ensure; bare token or owner URL → dual Search (`user:<q>` + quoted `in:name`), merge, rank exact repo name then owned-by-q then stars. Dashes never choose a path. Always `archived:false`. Cache key `search:v8:`.
- Install-command extraction from README fences must accept both CommonMark triple-backtick and tilde (`~~~`) fences, keep official curl/iex one-liners, filter by the same OS/arch as the download button, and drop bare `npm`/`yarn`/`pnpm install` build steps (keep global installs like `npm install -g`).
- Bare versioned archives (e.g. `.tar.xz`, `.zip`) with no OS/arch token are treated as source archives, not installable binaries, in both the Go picker and the frontend. Platform/arch-tagged zips stay eligible. Bounded `arm` (e.g. `*-arm.dmg`) is Apple Silicon; browser Mac UAs often omit ARM, so the frontend fills arch from the WebGL GPU renderer when UA arch is empty. `/dl` and `/api/link` accept `?platform=`, `?prefer=` (deb/rpm/appimage/msi/dmg/…), and `?libc=` (musl/gnu/static); script UAs get 404 JSON on miss, browsers keep a GitHub HTML 302. Checksums come from downloadable checksum/sha*sums or `*.sha256` / `*.sha512` / `*.md5` assets.
- Production Redis is Upstash via Vercel Marketplace over the Redis protocol (`REDIS_URL`, then `KV_URL`, then `UPSTASH_REDIS_URL`); do not switch the Go backend to Upstash REST/`KV_REST_API_*`.
- HTTP rate limiting uses process-local windows when Redis is unset or unreachable (does not fail open); `/health` stays HTTP 200 with redis/rate_limit/github budget fields (`github_token` boolean only).
- Crawling is configured in `frontend/app/robots.ts`: allow `/`, disallow `/api/` and `/dl/`; general crawlers also disallow `/p/`; social preview bots are allowlisted for unfurls. Release OG images live at `/{owner}/{repo}/opengraph-image` (not under `/p/`) so the general `/p/` disallow does not block card images; they load Outfit from a local TTF (do not fetch Google Fonts at build time — Satori cannot parse woff2). Favicon is the static Y-mark `.ico`/PNG, not a generated `icon.tsx`. Site includes `/privacy` (GitHub non-affiliation lives there); footer is Privacy + Source only. Top-right chrome links GitHub and X (`GITHUB_REPO_URL` / `TWITTER_URL` in `frontend/lib/site.ts`).
- Vercel “Images Storage” is OCI container images for the Go backend (`runtime: container`), not Redis or GitHub avatars; the registry is capped at ~50 images and a full registry blocks deploys until unused tags are pruned.
- Vercel BotID protects `/api/search` only; do not extend challenges to `/p/*` or `/dl/` — those URL-swap paths must succeed on the first request before any client challenge can run.
- Frontend is TypeScript 7 with Next `experimental.useTypeScriptCli: true` (Next’s default JS compiler API is missing in TS 7).

## Cursor Cloud specific instructions

Two services (see `CLAUDE.md` for the full command list and architecture): Go backend in `backend/` on `:8080`, Next.js frontend in `frontend/` on `:3000`. `./dev.sh` runs both together; end-to-end testing needs both.

- Toolchain: the backend needs Go 1.25+ (`backend/go.mod` pins `go 1.25.1`) and the frontend uses **Bun**, not npm. The VM snapshot ships Go 1.25 (symlinked at `/usr/local/bin/go`, ahead of the older distro `/usr/bin/go`) and Bun (`/usr/local/bin/bun`); the update script only refreshes project deps.
- No env vars are required for local dev: `BACKEND_URL` defaults to `http://localhost:8080`, and the backend hits GitHub's public API unauthenticated. Redis is fully optional (cache no-ops without it; rate limiting falls back to process-local windows). Go tests use embedded miniredis.
- Non-obvious caveat: heavy testing against real repos can hit GitHub's anonymous rate limits (60 req/hr). Set `GITHUB_TOKEN` (classic PAT, no scopes) on the backend to raise limits if resolving many repos.
- Frontend has no lint script; `bun test` is the only frontend test entry. Backend: `go test ./...` and `go build ./...`.
