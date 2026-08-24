/** Homepage “Try one” chips and release-page ISR build seeds. Keep as
 *  static owner/repo pairs only — `generateStaticParams` must not fetch. */
export const EXAMPLE_REPOS = [
  { owner: "cli", repo: "cli" },
  { owner: "neovim", repo: "neovim" },
  { owner: "astral-sh", repo: "uv" },
  { owner: "BurntSushi", repo: "ripgrep" },
] as const;
