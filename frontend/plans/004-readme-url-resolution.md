# 004 — Harden and simplify README relative-URL resolution

- **Status**: DONE (tsc + react-doctor --scope changed clean; 8/8 URL-resolution assertions pass, incl. `../` and `#anchor` fixes)
- **Commit**: d5b66a3
- **Severity**: LOW
- **Category**: Security (defense-in-depth) + stdlib reinvention
- **Rule**: Beyond the scan

## Problem

`resolveReadmeUrl` is the custom `urlTransform` passed to `react-markdown` for the
rendered README. Supplying a custom `urlTransform` **replaces** react-markdown's
built-in scheme sanitizer, and this replacement hand-rolls relative→absolute
joining with a single `./`-strip that mishandles `../`, root-relative `/path`, and
anchor `#frag` URLs.

`frontend/app/p/[owner]/[repo]/release-page.tsx:228` — current

    function resolveReadmeUrl(url: string, owner: string, repo: string): string {
      if (!url) return url;
      if (url.startsWith("http://") || url.startsWith("https://") || url.startsWith("data:")) return url;
      const path = url.replace(/^\.\//, "");
      return `https://raw.githubusercontent.com/${owner}/${repo}/HEAD/${path}`;
    }

Wired at `release-page.tsx:147`:

    urlTransform={(url) => resolveReadmeUrl(url, owner, repo)}

**Not currently exploitable**: the README also runs through `rehype-sanitize`
(`release-page.tsx:135`), whose `href`/`src` protocol allowlist strips `javascript:`,
`data:` (on links), etc. *before* `urlTransform` sees them, so the `data:`
passthrough here is a dead branch for dangerous inputs. This plan removes the
footgun and fixes the relative-path correctness, keeping the sanitizer as the
authoritative backstop.

Correctness gaps today: `../up.png` → `.../HEAD/../up.png` (not normalized);
`/root.png` → `.../HEAD//root.png` (double slash); `#section` → `.../HEAD/#section`
(broken anchor).

## Target

Use the WHATWG `URL` constructor for relative resolution (normalizes `../`,
handles root-relative), and only pass through absolute `http(s)`; drop the `data:`
passthrough so the sanitizer's decision stands.

`frontend/app/p/[owner]/[repo]/release-page.tsx` — replace the function:

    function resolveReadmeUrl(url: string, owner: string, repo: string): string {
      if (!url) return url;
      if (url.startsWith("http://") || url.startsWith("https://")) return url;
      // In-page anchors and mail/other schemes are left for rehype-sanitize to
      // vet; only resolve repo-relative paths against the raw content root.
      if (url.startsWith("#") || url.includes(":")) return url;
      try {
        return new URL(url, `https://raw.githubusercontent.com/${owner}/${repo}/HEAD/`).toString();
      } catch {
        return url;
      }
    }

Rationale for each guard:
- `#…` anchors pass through untouched (they were being mangled before).
- `url.includes(":")` catches any scheme (`mailto:`, `data:`, `javascript:`) and
  defers to `rehype-sanitize` — which is already the authority — instead of this
  function silently absolutizing or passing it. A repo-relative path never
  contains `:` before the first `/`, so this does not misclassify normal paths.
- `new URL(base-relative)` normalizes `./`, `../`, and `/root` correctly.

## Repo conventions to follow

- Keep the function private (not exported) and co-located at the bottom of
  `release-page.tsx` beside `extractInstallCommands`, as it is now.
- Do not touch the `rehype-raw` → `rehype-sanitize` plugin order or the custom
  sanitize schema — that pipeline is the security control and is correct.

## Steps

1. At `release-page.tsx:228`, replace `resolveReadmeUrl` with the Target code.
2. Leave the `urlTransform={(url) => resolveReadmeUrl(url, owner, repo)}` wiring
   at line 147 unchanged.
3. Re-read the diff; confirm no change to the `rehypePlugins` array.

## Boundaries

- Do NOT weaken or reorder the `rehype-sanitize` config — it is the primary XSS
  control; this change is defense-in-depth + correctness only.
- Do NOT export the function or reuse it for the repo `description` render (that
  path intentionally uses react-markdown's default `urlTransform`).
- Do NOT add dependencies.
- STOP if `release-page.tsx` has drifted from commit d5b66a3.

## Verification

- **Mechanical**: `cd frontend && npx tsc --noEmit`; `npx react-doctor@latest --scope changed` — score not lower.
- **Behavior check**: Open a release page for a repo whose README uses relative
  image paths and links (most do, e.g. `/p/BurntSushi/ripgrep`), expand "About",
  and confirm: (a) relative images still load from `raw.githubusercontent.com`,
  (b) a `./`-prefixed link resolves the same as before, (c) an in-page `#anchor`
  link no longer points at `raw.githubusercontent.com`. Optionally test a README
  containing `[x](javascript:alert(1))` locally and confirm the link is inert
  (sanitizer strips it — unchanged from today).
- **Done when**: relative assets still resolve, anchors are left intact, typecheck
  passes, score not lower.
