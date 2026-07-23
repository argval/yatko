// Install Command module — types + README fence extraction.
// Presentation (InstallCommands) imports downward from here; do not reverse that seam.

export type InstallPlatform = "macos" | "windows" | "linux" | "universal";

export type InstallCommand = {
  command: string;
  platform: InstallPlatform;
};

export function extractInstallCommands(readme: string): InstallCommand[] {
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
    // PowerShell one-liners: `powershell -c "irm …|iex"`, bare `irm …|iex`, etc.
    {
      platform: "windows",
      re: /^\s*(?:\$|>)?\s*((?:powershell|pwsh)(?:\.exe)?\s+(?:-c|-command)\s+.+)/i,
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
          commands.set(command, platform);
        }
      }
    }
  }
  return [...commands].map(([command, platform]) => ({ command, platform }));
}
