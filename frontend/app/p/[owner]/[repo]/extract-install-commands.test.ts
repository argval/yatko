import { describe, expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { InstallPrerequisites } from "./install-commands";
import { commandMentionsRepo, extractInstallCommands } from "./extract-install-commands";

const md = (...lines: string[]) => lines.join("\n");
const commands = (readme: string, opts?: { owner: string; repo: string }) =>
  extractInstallCommands(readme, opts).map((c) => c.command);

describe("extractInstallCommands", () => {
  test("parses backtick fences with section evidence", () => {
    expect(extractInstallCommands(md("## Install", "```", "brew install foo", "```"))).toEqual([
      { command: "brew install foo", platform: "macos", evidence: "Install" },
    ]);
  });

  test("parses tilde fences used by some READMEs", () => {
    const readme = md("**macOS**", "~~~ shell", "brew install htop", "~~~");
    expect(extractInstallCommands(readme)).toMatchObject([
      { command: "brew install htop", platform: "macos" },
    ]);
  });

  test("drops build-from-source prerequisites (htop-style)", () => {
    const readme = md(
      "## Build instructions",
      "### Prerequisite",
      "**Debian/Ubuntu**",
      "~~~ shell",
      "sudo apt install libncursesw5-dev autotools-dev build-essential",
      "~~~",
      "**macOS**",
      "~~~ shell",
      "brew install ncurses automake autoconf gcc",
      "~~~",
      "```bash",
      "pip install numpy",
      "```",
    );
    expect(extractInstallCommands(readme)).toEqual([]);
  });

  test("keeps sudo/doas and matches dnf/zypper/pacman", () => {
    const readme = md(
      "```",
      "sudo dnf install foo",
      "sudo zypper install foo",
      "sudo pacman -S --needed foo",
      "doas apt-get install foo",
      "pacman -Ss foo",
      "```",
    );
    expect(extractInstallCommands(readme)).toEqual([
      { command: "sudo dnf install foo", platform: "linux" },
      { command: "sudo zypper install foo", platform: "linux" },
      { command: "sudo pacman -S --needed foo", platform: "linux" },
      { command: "doas apt-get install foo", platform: "linux" },
    ]);
  });

  test("ignores install lines outside fences and in non-shell fences", () => {
    expect(extractInstallCommands("Just run brew install foo")).toEqual([]);
    expect(extractInstallCommands("    brew install foo")).toEqual([]);
    expect(extractInstallCommands(md("```python", "pip install foo", "```"))).toEqual([]);
  });

  test("console fences only take prompt lines", () => {
    const readme = md("```console", "$ cargo install foo", "cargo install bar", "```");
    expect(commands(readme)).toEqual(["cargo install foo"]);
  });

  test("allows install-script one-liners but rejects chained / substitution forms", () => {
    const readme = md(
      "```",
      "curl https://example.com/install.sh | bash",
      "brew install foo && rm -rf /",
      "irm https://example.com/x.ps1 | iex",
      "iex (irm https://example.com/install.ps1)",
      'powershell -ExecutionPolicy ByPass -c "irm https://example.com | iex"',
      'powershell -c "irm https://example.com | iex; evil"',
      "curl $(echo evil) /x",
      "brew install safe-tool",
      "```",
    );
    expect(extractInstallCommands(readme)).toEqual([
      { command: "curl https://example.com/install.sh | bash", platform: "unknown" },
      { command: "irm https://example.com/x.ps1 | iex", platform: "windows" },
      { command: "iex (irm https://example.com/install.ps1)", platform: "windows" },
      {
        command: 'powershell -ExecutionPolicy ByPass -c "irm https://example.com | iex"',
        platform: "windows",
      },
      { command: "brew install safe-tool", platform: "macos" },
    ]);
  });

  test("infers OS and arch for scripts only from stated context", () => {
    const readme = md(
      "## Installation",
      "### Linux x86_64",
      "```sh",
      "curl -fsSL https://example.com/install.sh | sh",
      "```",
      "On macOS and Linux:",
      "```sh",
      "curl -LsSf https://example.com/uv.sh | sh",
      "```",
      "- Raspberry Pi (aarch64):",
      "",
      "  ```sh",
      "  wget -qO- https://example.com/pi.sh | sudo -E bash -",
      "  ```",
    );
    expect(extractInstallCommands(readme)).toMatchObject([
      { command: "curl -fsSL https://example.com/install.sh | sh", platform: "linux", arches: ["amd64"] },
      {
        command: "curl -LsSf https://example.com/uv.sh | sh",
        platform: "macos",
        platforms: ["macos", "linux"],
      },
      { command: "wget -qO- https://example.com/pi.sh | sudo -E bash -", platform: "linux", arches: ["arm64"] },
    ]);
    // A package manager's portability does not override the project's stated OS.
    expect(extractInstallCommands(md("### Windows", "```", "pipx install foo", "```"))).toMatchObject([
      { platform: "windows" },
    ]);
  });

  test("keeps backslash-continued commands whole", () => {
    const readme = md(
      "```bash",
      "curl --proto '=https' --tlsv1.2 -sSf \\",
      "  https://example.com/install.sh | sh -s -- -y",
      "brew install foo",
      "```",
    );
    expect(commands(readme)).toEqual([
      "curl --proto '=https' --tlsv1.2 -sSf \\\n  https://example.com/install.sh | sh -s -- -y",
      "brew install foo",
    ]);
  });

  test("preserves identical installers under different target sections",()=>{
    const command="curl https://example.com/install.sh | sh";
    const readme=md("## Installation","### macOS","```sh",command,"```","### Linux","```sh",command,"```");
    expect(extractInstallCommands(readme).map(({platform})=>platform)).toEqual(["macos","linux"]);
  });

  test("does not attach a prerequisite for another CPU",()=>{
    const readme=md("## Installation","### Prerequisites for Apple Silicon","```sh","brew install helper","```","### Intel Mac","```sh","brew install tool","```");
    expect(extractInstallCommands(readme)[0].prerequisites).toBeUndefined();
  });

  test("attaches stated prerequisites from the same install section", () => {
    const readme = md(
      "## Installation",
      "### Prerequisites",
      "```",
      "sudo apt install libssl-dev",
      "```",
      "### Cargo",
      "```",
      "cargo install foo",
      "```",
      "## Other",
      "```",
      "cargo install bar",
      "```",
    );
    expect(extractInstallCommands(readme)).toMatchObject([
      { command: "cargo install foo", platform: "universal", prerequisites: ["sudo apt install libssl-dev"] },
      { command: "cargo install bar" },
    ]);
    expect(extractInstallCommands(readme)[1].prerequisites).toBeUndefined();
  });

  test("recognizes modern Python and Bun global installers", () => {
    const readme = md(
      "```",
      "uv tool install foo",
      "pipx install foo",
      "pip3 install foo",
      "python -m pip install foo",
      "python3 -m pip install 'foo[cli]'",
      "bun add -g foo",
      "bun install -g foo",
      "npm i -g @scope/foo",
      "```",
    );
    expect(commands(readme)).toEqual([
      "uv tool install foo",
      "pipx install foo",
      "pip3 install foo",
      "python -m pip install foo",
      "python3 -m pip install 'foo[cli]'",
      "bun add -g foo",
      "bun install -g foo",
      "npm i -g @scope/foo",
    ]);
  });

  test("drops project-local installs", () => {
    const readme = md(
      "```",
      "npm install --legacy-peer-deps",
      "yarn install",
      "pnpm install",
      "bun install",
      "pip install -r requirements.txt",
      "pip install -e .",
      "cargo install --path .",
      "go install ./...",
      "pip install httpx",
      "```",
    );
    expect(commands(readme)).toEqual(["pip install httpx"]);
  });

  test("drops bare npm install build steps (even with # captions)", () => {
    const readme = md(
      "## Commands",
      "```bash",
      "$ npm install                # Install dependencies",
      "$ npm start                  # Start Dopamine",
      "$ npm install -g cowsay",
      "```",
    );
    expect(commands(readme, { owner: "digimezzo", repo: "dopamine" })).toEqual([
      "npm install -g cowsay",
    ]);
  });

  test("drops troubleshooting/uninstall sections; keeps install sections", () => {
    const readme = md(
      "## Quick Install",
      "```bash",
      "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash",
      "```",
      "### Windows",
      "```powershell",
      "iex (irm https://hermes-agent.nousresearch.com/install.ps1)",
      "```",
      "#### Troubleshooting",
      "```powershell",
      "winget install --id GitHub.cli",
      'Invoke-WebRequest "https://github.com/astral-sh/uv/releases/download/$ver/uv.zip" -OutFile $zip',
      "```",
      "## Uninstall",
      "```",
      "brew install hermes-cleanup",
      "```",
    );
    expect(extractInstallCommands(readme, { owner: "NousResearch", repo: "hermes-agent" })).toEqual([
      {
        command: "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash",
        platform: "unknown",
        evidence: "Quick Install",
      },
      {
        command: "iex (irm https://hermes-agent.nousresearch.com/install.ps1)",
        platform: "windows",
        evidence: "Quick Install › Windows",
      },
    ]);
  });

  test("install sections keep official aliases even when another command names the repo", () => {
    const readme = md(
      "## Installation",
      "### macOS",
      "```",
      "brew install gh",
      "```",
      "### Windows",
      "```",
      "winget install --id GitHub.cli",
      "```",
      "### Linux",
      "```",
      "sudo apt install gh",
      "```",
    );
    expect(commands(readme, { owner: "cli", repo: "cli" })).toEqual([
      "brew install gh",
      "winget install --id GitHub.cli",
      "sudo apt install gh",
    ]);
  });

  test("keeps all install methods for repos whose package names use different separators", () => {
    const readme = md(
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
    );
    expect(extractInstallCommands(readme, { owner: "pingdotgg", repo: "t3code" })).toMatchObject([
      { command: "npx t3@latest", platform: "universal" },
      { command: "winget install T3Tools.T3Code", platform: "windows" },
      { command: "brew install --cask t3-code", platform: "macos" },
      { command: "yay -S t3code-bin", platform: "linux" },
      { command: "yay -S t3code-nightly-bin", platform: "linux" },
    ]);
  });

  test("context-free commands: drops unrelated deps only when some command names the repo", () => {
    expect(commands(md("```", "brew install rg", "```"), { owner: "BurntSushi", repo: "ripgrep" })).toEqual([
      "brew install rg",
    ]);
    const mixed = md("```", "brew install ripgrep", "brew install jq", "```");
    expect(commands(mixed, { owner: "BurntSushi", repo: "ripgrep" })).toEqual(["brew install ripgrep"]);
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


test("universal installs retain prerequisite targets and display only the selected OS and CPU", () => {
  const readme = md(
    "## Installation",
    "### Prerequisites for Linux ARM64",
    "```sh", "sudo apt install libssl-dev", "```",
    "### Prerequisites for ARM64",
    "```sh", "npm install -g helper", "```",
    "### Install",
    "```sh", "npm install -g example-tool", "```",
  );
  const [command] = extractInstallCommands(readme);
  expect(command.platform).toBe("universal");
  expect(command.prerequisiteTargets).toMatchObject([
    { command: "sudo apt install libssl-dev", platform: "linux", arches: ["arm64"] },
    { command: "npm install -g helper", platform: "universal", arches: ["arm64"] },
  ]);
  const render = (platform: "linux" | "windows", arch: "amd64" | "arm64") =>
    renderToStaticMarkup(createElement(InstallPrerequisites, { command, target: { platform, arch } }));
  expect(render("linux", "arm64")).toContain("sudo apt install libssl-dev");
  expect(render("linux", "arm64")).toContain("npm install -g helper");
  expect(render("windows", "arm64")).not.toContain("sudo apt");
  expect(render("windows", "arm64")).toContain("npm install -g helper");
  expect(render("windows", "amd64")).toBe("");
  expect(render("linux", "amd64")).toBe("");
});

test("legacy prerequisite strings still display when no target metadata is supplied", () => {
  expect(renderToStaticMarkup(createElement(InstallPrerequisites, {
    command: { command: "npm install -g example-tool", platform: "universal", prerequisites: ["npm install -g helper"] },
    target: { platform: "windows", arch: "amd64" },
  }))).toContain("npm install -g helper");
});
