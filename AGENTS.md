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

## Learned Workspace Facts

- Yatko is positioned as a drop-in release-download URL: replace `github.com` with `yatko.app` for the same owner/repo path.
- Vercel Speed Insights is enabled on the application.
- The download-badge feature was removed from the product.
- Release-page markdown (blurb, notes, About) goes through shared `RepoMarkdown` in `frontend/app/p/[owner]/[repo]/markdown.tsx` (GFM, raw HTML, sanitize, URL rewrite) with `@tailwindcss/typography`.
- Frontend icons live in a local `icons.tsx`; `lucide-react` is not a dependency.
- Release pages include a back control to the Yatko homepage (`/`).
- `architecture-review` is the integration branch for architecture deepen PRs.
- Production deploys track the `prod` branch (typically merged from `main`).
- Search cache uses a longer soft TTL with prefix reuse; an in-process L1 LRU sits in front of Redis for hot keys.
- Install-command extraction from README fences must accept both CommonMark triple-backtick and tilde (`~~~`) fences.
- Bare versioned tarballs (e.g. `.tar.xz`) with no OS/arch token are treated as source archives, not installable binaries, in both the Go picker and the frontend.
- Release checksums come from downloadable checksum assets (names matching checksum/sha*sums or `*.sha256` / `*.sha512` / `*.md5`), fetched and parsed into a filename→hash map.
