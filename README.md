# Yatko

**Clean download links for any public GitHub release.**

Swap the domain — that's it. Same `owner/repo` path as GitHub, one click to the right binary for your users' platform and architecture.

```
github.com/cli/cli  →  yatko.app/cli/cli
```

Need a direct download (no landing page)? Add `/dl`:

```
https://yatko.app/dl/cli/cli
```

---

## What it does

Most GitHub projects bury downloads in a releases page with 15+ assets. Yatko gives you the same URL shape as GitHub, plus smart links that pick the right asset automatically:

| URL | What it does |
|-----|-------------|
| `/:owner/:repo` | Landing page — same path as `github.com/owner/repo`, just swap the domain |
| `/p/:owner/:repo/:version` | Landing page for a specific release tag |
| `/dl/:owner/:repo` | Detects platform + arch, redirects straight to the right binary |
| `/dl/:owner/:repo/:version` | Same, but for a specific release tag |
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

## Badge

```markdown
![version](https://yatko.app/badge/owner/repo)
```
