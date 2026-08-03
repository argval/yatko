# Yatko

**Clean download links for any public GitHub release so that you don't have to be called a ["Smelly Nerd"](https://www.reddit.com/r/github/s/7YaS7nTVup) anymore.**

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
| `/dl/:owner/:repo` | Detects platform + arch, redirects straight to the right binary (`?platform=`, `?arch=`, `?prefer=`, `?libc=` overrides) |
| `/dl/:owner/:repo/:version` | Same, but for a specific release tag |
| `/api/link/:owner/:repo` | JSON with resolved download URL — for CI/scripts (same query overrides as `/dl`) |
| `/api/releases/:owner/:repo` | List of recent releases (tag, date, prerelease flag) |
| `/api/readme/:owner/:repo` | Raw README markdown (install commands / About section) |
| `/api/search?q=` | GitHub repo search — homepage autocomplete suggestions |

## Features

- **Repo search** — type a name on the homepage and pick from live GitHub suggestions
- **Platform detection** — Windows / macOS / Linux from User-Agent
- **Architecture detection** — amd64 / arm64 / arm / 386 from User-Agent and `navigator.userAgentData`
- **Format & libc overrides** — pin package type (`?prefer=deb`) or musl/gnu/static (`?libc=musl`) on `/dl` and `/api/link`
- **Version selector** — browse and switch between recent releases in the UI
- **Pre-release toggle** — opt into alpha/beta builds when available
- **Asset checksums** — automatically fetches and displays SHA256 for the selected binary, with a copyable verify command
- **Quick Install** — extracts package manager commands from the README (`pip`, `npm`, `cargo`, `brew`, `winget`, `choco`, `scoop`, `apt`, and more)
- **Platform filter** — "My platform only" toggle in the All Downloads list
- **Download counts** — shows per-asset download counts from GitHub
- **Share links** — copyable Yatko URLs for smart download, landing page, API, and a README Markdown snippet
- **Dark mode** — system preference detection + manual toggle
