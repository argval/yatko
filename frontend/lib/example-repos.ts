/** Homepage “Try one” chips and release-page ISR build seeds. Keep as
 *  static owner/repo pairs only — `generateStaticParams` must not fetch.
 *  GUI desktop apps lead so the homepage shows tools non-developers
 *  recognize; existing CLI examples stay after them. */
export const EXAMPLE_REPOS = [
  { owner: "godotengine", repo: "godot" },
  { owner: "zed-industries", repo: "zed" },
  { owner: "logseq", repo: "logseq" },
  { owner: "audacity", repo: "audacity" },
  { owner: "cli", repo: "cli" },
  { owner: "neovim", repo: "neovim" },
  { owner: "astral-sh", repo: "uv" },
  { owner: "BurntSushi", repo: "ripgrep" },
] as const;
