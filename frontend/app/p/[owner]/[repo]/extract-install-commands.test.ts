import { describe, expect, test } from "bun:test";
import {
  commandMentionsRepo,
  extractInstallCommands,
  isInstallScriptOneLiner,
  isUnsafeInstallCommand,
} from "./extract-install-commands";

describe("extractInstallCommands", () => {
  test("parses backtick fences", () => {
    const readme = ["## Install", "```", "brew install foo", "```"].join("\n");
    expect(extractInstallCommands(readme)).toEqual([
      { command: "brew install foo", platform: "macos" },
    ]);
  });

  test("parses tilde fences used by some READMEs", () => {
    const readme = [
      "**macOS**",
      "~~~ shell",
      "brew install ncurses automake autoconf gcc",
      "~~~",
      "",
      "**Debian/Ubuntu**",
      "~~~ shell",
      "sudo apt install libncursesw5-dev build-essential",
      "~~~",
    ].join("\n");
    expect(extractInstallCommands(readme)).toEqual([
      { command: "brew install ncurses automake autoconf gcc", platform: "macos" },
      { command: "sudo apt install libncursesw5-dev build-essential", platform: "linux" },
    ]);
  });

  test("keeps sudo/doas and matches dnf/zypper/pacman", () => {
    const readme = [
      "```",
      "sudo dnf install ncurses-devel",
      "sudo zypper install ncurses-devel",
      "sudo pacman -S --needed base-devel ncurses",
      "doas apt-get install foo",
      "```",
    ].join("\n");
    expect(extractInstallCommands(readme)).toEqual([
      { command: "sudo dnf install ncurses-devel", platform: "linux" },
      { command: "sudo zypper install ncurses-devel", platform: "linux" },
      { command: "sudo pacman -S --needed base-devel ncurses", platform: "linux" },
      { command: "doas apt-get install foo", platform: "linux" },
    ]);
  });

  test("ignores install lines outside fences", () => {
    expect(extractInstallCommands("Just run brew install foo")).toEqual([]);
  });

  test("allows install-script one-liners but rejects chained / substitution forms", () => {
    const readme = [
      "```",
      "curl https://example.com/install.sh | bash",
      "brew install foo && rm -rf /",
      "irm https://example.com/x.ps1 | iex",
      "iex (irm https://example.com/install.ps1)",
      "powershell -c \"irm https://example.com | iex\"",
      "brew install safe-tool",
      "```",
    ].join("\n");
    expect(extractInstallCommands(readme)).toEqual([
      { command: "curl https://example.com/install.sh | bash", platform: "universal" },
      { command: "irm https://example.com/x.ps1 | iex", platform: "windows" },
      { command: "iex (irm https://example.com/install.ps1)", platform: "windows" },
      {
        command: 'powershell -c "irm https://example.com | iex"',
        platform: "windows",
      },
      { command: "brew install safe-tool", platform: "macos" },
    ]);
  });

  test("rejects command substitution and backticks", () => {
    const readme = ["```", "curl $(echo evil) /x", "wget `id`", "npm install lodash", "```"].join(
      "\n",
    );
    expect(extractInstallCommands(readme)).toEqual([
      { command: "npm install lodash", platform: "universal" },
    ]);
  });

  test("when owner/repo given, drops unrelated dep installs if any relevant exist", () => {
    const readme = [
      "## Quick Install",
      "```bash",
      "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash",
      "```",
      "",
      "### Windows",
      "```powershell",
      "iex (irm https://hermes-agent.nousresearch.com/install.ps1)",
      "```",
      "",
      "#### Troubleshooting",
      "```powershell",
      "winget install --id GitHub.cli",
      'Invoke-WebRequest "https://github.com/astral-sh/uv/releases/download/$ver/uv.zip" -OutFile $zip',
      "```",
    ].join("\n");

    expect(
      extractInstallCommands(readme, { owner: "NousResearch", repo: "hermes-agent" }),
    ).toEqual([
      {
        command: "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash",
        platform: "universal",
      },
      {
        command: "iex (irm https://hermes-agent.nousresearch.com/install.ps1)",
        platform: "windows",
      },
    ]);
  });

  test("keeps all install methods for repos whose package names use different separators", () => {
    const readme = [
      "### Try it out",
      "```bash",
      "npx t3@latest",
      "```",
      "### Windows",
      "```bash",
      "winget install T3Tools.T3Code",
      "```",
      "### macOS",
      "```bash",
      "brew install --cask t3-code",
      "```",
      "### Arch Linux",
      "```bash",
      "yay -S t3code-bin",
      "yay -S t3code-nightly-bin",
      "```",
    ].join("\n");

    expect(
      extractInstallCommands(readme, { owner: "pingdotgg", repo: "t3code" }),
    ).toEqual([
      { command: "npx t3@latest", platform: "universal" },
      { command: "winget install T3Tools.T3Code", platform: "windows" },
      { command: "brew install --cask t3-code", platform: "macos" },
      { command: "yay -S t3code-bin", platform: "linux" },
      { command: "yay -S t3code-nightly-bin", platform: "linux" },
    ]);
  });

  test("without relevant matches, keeps all safe commands (package name ≠ repo)", () => {
    const readme = ["```", "brew install rg", "```"].join("\n");
    expect(
      extractInstallCommands(readme, { owner: "BurntSushi", repo: "ripgrep" }),
    ).toEqual([{ command: "brew install rg", platform: "macos" }]);
  });

  test("drops bare npm install build steps (even with # captions)", () => {
    const readme = [
      "## Build instructions",
      "```bash",
      "$ npm install                # Install dependencies",
      "$ npm start                  # Start Dopamine",
      "$ npm install -g cowsay",
      "```",
    ].join("\n");
    expect(
      extractInstallCommands(readme, { owner: "digimezzo", repo: "dopamine" }),
    ).toEqual([{ command: "npm install -g cowsay", platform: "universal" }]);
  });

  test("drops npm install with flags only and yarn/pnpm install", () => {
    const readme = [
      "```",
      "npm install --legacy-peer-deps",
      "yarn install",
      "pnpm install",
      "pip install -r requirements.txt",
      "pip install httpx",
      "```",
    ].join("\n");
    expect(extractInstallCommands(readme)).toEqual([
      { command: "pip install httpx", platform: "universal" },
    ]);
  });
});

describe("isInstallScriptOneLiner / isUnsafeInstallCommand", () => {
  test("classifies common install scripts as safe", () => {
    expect(isInstallScriptOneLiner("curl -fsSL https://x/install.sh | bash")).toBe(true);
    expect(isUnsafeInstallCommand("curl -fsSL https://x/install.sh | bash")).toBe(false);
    expect(isUnsafeInstallCommand("iex (irm https://x/install.ps1)")).toBe(false);
    expect(isUnsafeInstallCommand("brew install foo && rm -rf /")).toBe(true);
  });
});

describe("commandMentionsRepo", () => {
  test("matches full slug, package separators, and long primary token", () => {
    expect(
      commandMentionsRepo(
        "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash",
        "NousResearch",
        "hermes-agent",
      ),
    ).toBe(true);
    expect(commandMentionsRepo("brew install --cask t3-code", "pingdotgg", "t3code")).toBe(true);
    expect(commandMentionsRepo("npx t3@latest", "pingdotgg", "t3code")).toBe(true);
    expect(commandMentionsRepo("winget install --id GitHub.cli", "NousResearch", "hermes-agent")).toBe(
      false,
    );
  });
});
