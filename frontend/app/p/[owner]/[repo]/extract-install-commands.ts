// Install Command module — types + README fence extraction.
// Presentation (InstallCommands) imports downward from here; do not reverse that seam.

export type InstallPlatform = "macos" | "windows" | "linux" | "universal";

export type InstallCommand = {
  command: string;
  platform: InstallPlatform;
};

/**
 * Reject chained / substitution forms so Yatko never elevates them.
 * Common install-script one-liners (curl|bash, irm|iex, iex(irm)) are allowed —
 * those are what most READMEs document as the install path — but never with
 * `&&`, `;`, backticks, or `$(…)`.
 */
export function isUnsafeInstallCommand(command: string): boolean {
  if (/[;`]|&&|\$\(/.test(command)) return true;
  if (isInstallScriptOneLiner(command)) return false;
  return /[|`]|\biex\b/i.test(command);
}

/** curl|sh, wget|bash, irm|iex, iex (irm …), powershell -c "…|iex". */
export function isInstallScriptOneLiner(command: string): boolean {
  const c = command.trim();
  if (/^(?:curl|wget)\b[\s\S]*\|\s*(?:sudo\s+)?(?:ba)?sh\b/i.test(c)) return true;
  if (/^(?:irm|iwr|Invoke-RestMethod|Invoke-WebRequest)\b[\s\S]*\|\s*iex\b/i.test(c)) {
    return true;
  }
  if (/^iex\s*\(\s*(?:irm|iwr|Invoke-RestMethod|Invoke-WebRequest)\b[\s\S]+\)$/i.test(c)) {
    return true;
  }
  if (/^(?:powershell|pwsh)(?:\.exe)?\s+(?:-c|-command)\s+[\s\S]+\biex\b/i.test(c)) {
    return true;
  }
  return false;
}

/** True when the command text looks tied to this owner/repo (not a dep install). */
export function commandMentionsRepo(command: string, owner: string, repo: string): boolean {
  const lower = command.toLowerCase();
  const o = owner.toLowerCase();
  const r = repo.toLowerCase();
  if (!o || !r) return false;
  if (lower.includes(`${o}/${r}`)) return true;
  if (lower.includes(`@${o}/${r}`)) return true;
  if (lower.includes(r)) return true;
  // Primary token of multi-segment names (hermes-agent → hermes) when long enough
  // to avoid matching short noise like "go" / "ai".
  const primary = r.split(/[-_]/)[0];
  if (primary && primary.length >= 4) {
    const re = new RegExp(`(?:^|[^a-z0-9])${escapeRegExp(primary)}(?:[^a-z0-9]|$)`, "i");
    if (re.test(lower)) return true;
  }
  return false;
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function extractInstallCommands(
  readme: string,
  opts?: { owner?: string; repo?: string },
): InstallCommand[] {
  const commands = new Map<string, InstallPlatform>();
  // Support both CommonMark ``` and some READMEs' ~~~ fences (e.g. htop).
  const codeBlockRe = /(?:```|~~~)[^\n]*\n([\s\S]*?)(?:```|~~~)/g;
  // Optional prompt ($/>) and privilege escalation (sudo/doas) before the command.
  const lead = String.raw`^\s*(?:\$|>)?\s*((?:(?:sudo|doas)\s+)?)`;
  const patterns: { platform: InstallPlatform; re: RegExp }[] = [
    { platform: "universal", re: new RegExp(lead + String.raw`(pip install\s+.+)`) },
    { platform: "universal", re: new RegExp(lead + String.raw`(npm install\s+.+)`) },
    { platform: "universal", re: new RegExp(lead + String.raw`(npx\s+.+)`) },
    { platform: "universal", re: new RegExp(lead + String.raw`(yarn add\s+.+)`) },
    { platform: "universal", re: new RegExp(lead + String.raw`(pnpm add\s+.+)`) },
    { platform: "universal", re: new RegExp(lead + String.raw`(cargo install\s+.+)`) },
    { platform: "universal", re: new RegExp(lead + String.raw`(go install\s+.+)`) },
    { platform: "macos", re: new RegExp(lead + String.raw`(brew install\s+.+)`) },
    { platform: "universal", re: new RegExp(lead + String.raw`(gem install\s+.+)`) },
    { platform: "linux", re: new RegExp(lead + String.raw`(apt(?:-get)?\s+install\s+.+)`) },
    { platform: "linux", re: new RegExp(lead + String.raw`(dnf\s+install\s+.+)`) },
    { platform: "linux", re: new RegExp(lead + String.raw`(yum\s+install\s+.+)`) },
    { platform: "linux", re: new RegExp(lead + String.raw`(zypper\s+install\s+.+)`) },
    { platform: "linux", re: new RegExp(lead + String.raw`(pacman\s+-S\s+.+)`) },
    { platform: "windows", re: new RegExp(lead + String.raw`(winget install\s+.+)`) },
    { platform: "windows", re: new RegExp(lead + String.raw`(choco install\s+.+)`) },
    { platform: "windows", re: new RegExp(lead + String.raw`(scoop install\s+.+)`) },
    // PowerShell one-liners: `powershell -c "irm …|iex"`, `iex (irm …)`, bare `irm …|iex`.
    {
      platform: "windows",
      re: /^\s*(?:\$|>)?\s*((?:powershell|pwsh)(?:\.exe)?\s+(?:-c|-command)\s+.+)/i,
    },
    {
      platform: "windows",
      re: /^\s*(?:\$|>)?\s*(iex\s*\(\s*(?:irm|iwr|Invoke-RestMethod|Invoke-WebRequest)\s+.+\))/i,
    },
    {
      platform: "windows",
      re: /^\s*(?:\$|>)?\s*((?:irm|iwr|Invoke-RestMethod|Invoke-WebRequest)\s+.+)/i,
    },
    { platform: "universal", re: new RegExp(lead + String.raw`(curl\s+.+)`) },
    { platform: "universal", re: new RegExp(lead + String.raw`(wget\s+.+)`) },
  ];
  let match;
  while ((match = codeBlockRe.exec(readme)) !== null) {
    for (const line of match[1].split("\n")) {
      for (const { platform, re } of patterns) {
        const m = line.match(re);
        if (m) {
          // m[1] = optional sudo/doas, m[2] = command — or a single capture for PS patterns.
          const command = (m[2] !== undefined ? `${m[1]}${m[2]}` : m[1]).trim();
          if (isUnsafeInstallCommand(command)) continue;
          commands.set(command, platform);
        }
      }
    }
  }
  let results = [...commands].map(([command, platform]) => ({ command, platform }));

  // When the README also documents dependency installs (winget GitHub.cli, etc.),
  // keep only lines that mention this owner/repo — if any such lines exist.
  const owner = opts?.owner?.trim();
  const repo = opts?.repo?.trim();
  if (owner && repo) {
    const relevant = results.filter((c) => commandMentionsRepo(c.command, owner, repo));
    if (relevant.length > 0) results = relevant;
  }

  return results;
}
