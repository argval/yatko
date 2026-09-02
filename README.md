<p align="center">
  <img src="frontend/assets/brand/yatko-mark-256.png" alt="Yatko logo" width="96" height="96" />
</p>

# Yatko

**[yatko.app](https://yatko.app)** — clean download links for any public GitHub release so that you don't have to be called a ["Smelly Nerd"](https://www.reddit.com/r/github/s/7YaS7nTVup) anymore.

Swap the domain — that's it. Same `owner/repo` path as GitHub, one click to the right binary for your users' platform and architecture.

```
github.com/cli/cli  →  yatko.app/cli/cli
```

<p align="center">
  <a href="https://www.producthunt.com/products/yatko?embed=true&utm_source=badge-featured&utm_medium=badge&utm_campaign=badge-yatko" target="_blank" rel="noopener noreferrer">
    <img src="https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1227238&theme=light&t=1787658051325" alt="Yatko – The download button GitHub forgot to add on Product Hunt" width="250" height="54" />
  </a>
</p>

Website: [https://yatko.app](https://yatko.app) · Source: [github.com/argval/yatko](https://github.com/argval/yatko)

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
| `/code/:owner/:repo` | Downloads the default branch as a ZIP source archive (`?tag=v1.2.3` pins a release tag) |
| `/api/link/:owner/:repo` | JSON with resolved download URL — for CI/scripts (same query overrides as `/dl`) |
| `/api/releases/:owner/:repo` | List of recent releases (tag, date, prerelease flag) |
| `/api/readme/:owner/:repo` | Raw README markdown (install commands / About section) |
| `/api/search?q=` | GitHub repo search — homepage autocomplete suggestions |
| `/badge.svg` | Store-style “Get it on Yatko” badge for READMEs (`height="54"`) |

## Features

- **Repo search** — type a name on the homepage and pick from live GitHub suggestions
- **Platform detection** — Windows / macOS / Linux from User-Agent
- **Architecture detection** — amd64 / arm64 / arm / 386 from User-Agent and `navigator.userAgentData`
- **Format & libc overrides** — pin package type (`?prefer=deb`) or musl/gnu/static (`?libc=musl`) on `/dl` and `/api/link`
- **Source code downloads** — grab a ZIP of the default branch or the release currently on screen
- **Version selector** — browse and switch between recent releases in the UI
- **Pre-release toggle** — opt into alpha/beta builds when available
- **Asset checksums** — automatically fetches and displays SHA256 for the selected binary, with a copyable verify command
- **Quick Install** — extracts package manager commands from the README (`pip`, `npm`, `cargo`, `brew`, `winget`, `choco`, `scoop`, `apt`, and more)
- **Platform filter** — "My platform only" toggle in the All Downloads list
- **Download counts** — shows per-asset download counts from GitHub
- **Share links** — copyable Yatko URLs for smart download, landing page, API, a README Markdown snippet, and an embed button
- **Embed button** — store-style “Get it on Yatko” badge for READMEs
- **Dark mode** — system preference detection + manual toggle

---

## Embed

Drop this in your README to send visitors to your Yatko download page:

<p>
  <a href="https://yatko.app/cli/cli">
    <img alt="Get it on Yatko" src="frontend/public/badge.svg" height="54" />
  </a>
</p>

```html
<a href="https://yatko.app/OWNER/REPO"><img alt="Get it on Yatko" src="https://yatko.app/badge.svg" height="54"></a>
```

## License

Yatko is free and open-source software under the [MIT License](LICENSE).
