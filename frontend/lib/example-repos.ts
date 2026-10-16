/** Homepage “Try one” chips and release-page ISR build seeds. Keep as
 *  static owner/repo pairs only — `generateStaticParams` must not fetch. */
export const EXAMPLE_REPOS = [
  { owner: "godotengine", repo: "godot" },
  { owner: "zed-industries", repo: "zed" },
  { owner: "logseq", repo: "logseq" },
  { owner: "audacity", repo: "audacity" },
] as const;
