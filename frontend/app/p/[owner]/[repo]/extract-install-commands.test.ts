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

  test("without relevant matches, keeps all safe commands (package name ≠ repo)", () => {
    const readme = ["```", "brew install rg", "```"].join("\n");
    expect(
      extractInstallCommands(readme, { owner: "BurntSushi", repo: "ripgrep" }),
    ).toEqual([{ command: "brew install rg", platform: "macos" }]);
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
  test("matches full slug, repo name, and long primary token", () => {
    expect(
      commandMentionsRepo(
        "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash",
        "NousResearch",
        "hermes-agent",
      ),
    ).toBe(true);
    expect(commandMentionsRepo("winget install --id GitHub.cli", "NousResearch", "hermes-agent")).toBe(
      false,
    );
  });
});
