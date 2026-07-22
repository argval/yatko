import type { InstallCommand, InstallPlatform } from "./install-commands";

export function extractInstallCommands(readme: string): InstallCommand[] {
  const commands = new Map<string, InstallPlatform>();
  const codeBlockRe = /```[^\n]*\n([\s\S]*?)```/g;
  const patterns: { platform: InstallPlatform; re: RegExp }[] = [
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(pip install\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(npm install\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(npx\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(yarn add\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(pnpm add\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(cargo install\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(go install\s+.+)/ },
    { platform: "macos", re: /^\s*(?:\$|>)?\s*(brew install\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(gem install\s+.+)/ },
    { platform: "linux", re: /^\s*(?:\$|>)?\s*(apt(?:-get)?\s+install\s+.+)/ },
    { platform: "windows", re: /^\s*(?:\$|>)?\s*(winget install\s+.+)/ },
    { platform: "windows", re: /^\s*(?:\$|>)?\s*(choco install\s+.+)/ },
    { platform: "windows", re: /^\s*(?:\$|>)?\s*(scoop install\s+.+)/ },
    // PowerShell one-liners: `powershell -c "irm …|iex"`, bare `irm …|iex`, etc.
    {
      platform: "windows",
      re: /^\s*(?:\$|>)?\s*((?:powershell|pwsh)(?:\.exe)?\s+(?:-c|-command)\s+.+)/i,
    },
    {
      platform: "windows",
      re: /^\s*(?:\$|>)?\s*((?:irm|iwr|Invoke-RestMethod|Invoke-WebRequest)\s+.+)/i,
    },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(curl\s+.+)/ },
    { platform: "universal", re: /^\s*(?:\$|>)?\s*(wget\s+.+)/ },
  ];
  let match;
  while ((match = codeBlockRe.exec(readme)) !== null) {
    for (const line of match[1].split("\n")) {
      for (const { platform, re } of patterns) {
        const m = line.match(re);
        if (m) commands.set(m[1].trim(), platform);
      }
    }
  }
  return [...commands].map(([command, platform]) => ({ command, platform }));
}
