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

// Generated snippets exercise section structure without depending on changing remote READMEs.
describe("README HTML and stated runtime requirements", () => {
  test("HTML headings preserve installation scope for unfamiliar method headings and aliases", () => {
    const readme = md(
      '<h2 id="install">Installation</h2>', '',
      '### The community package', '```sh', 'brew install gh', '```',
      '<h3>Windows ARM64</h3>', '',
      '```powershell', 'winget install GitHub.cli', '```',
    );
    expect(extractInstallCommands(readme, { owner: 'cli', repo: 'cli' })).toMatchObject([
      { command: 'brew install gh', platform: 'macos', evidence: 'Installation › The community package' },
      { command: 'winget install GitHub.cli', platform: 'windows', arches: ['arm64'] },
    ]);
  });

  test("HTML pre/code uses the same language and safety validation as Markdown fences", () => {
    const readme = '<section><h2>Install</h2><h3>Linux x64</h3>' +
      '<pre><code class="language-sh">curl "https://example.com/install?a=1&amp;b=2" | sh\n' +
      'brew install foo &amp;&amp; evil</code></pre>' +
      '<pre><code class="language-python">pip install example</code></pre>' +
      '<script>brew install hidden</script></section>';
    expect(extractInstallCommands(readme)).toMatchObject([
      { command: 'curl "https://example.com/install?a=1&b=2" | sh', platform: 'linux', arches: ['amd64'] },
    ]);
    expect(extractInstallCommands(readme)).toHaveLength(1);
  });

  test("HTML requirements keep exact rendered wording and explicit target scope", () => {
    const readme = '<div><h2>Installation</h2><h3>Requirements for Linux ARM64</h3>' +
      '<ul><li>Node.js 20+</li><li>Python 3.10 or later</li></ul>' +
      '<h3>Package</h3><pre><code>npm install -g example-tool</code></pre></div>';
    const [command] = extractInstallCommands(readme);
    expect(command.prerequisiteTargets).toMatchObject([
      { command: 'Node.js 20+', kind: 'prose', platform: 'linux', arches: ['arm64'] },
      { command: 'Python 3.10 or later', kind: 'prose', platform: 'linux', arches: ['arm64'] },
    ]);
    const render = (arch: 'arm64' | 'amd64') => renderToStaticMarkup(createElement(InstallPrerequisites, {
      command, target: { platform: 'linux', arch },
    }));
    expect(render('arm64')).toContain('Node.js 20+; Python 3.10 or later');
    expect(render('arm64')).not.toContain('<button');
    expect(render('amd64')).toBe('');
  });

  test("explicit prerequisite prose attaches to the following installer without becoming a command", () => {
    const readme = md('## Installation', 'Requires Node.js 20+, Python 3.10 or later.', '',
      '```sh', 'npm install -g example-tool', '```', '## Usage',
      '```sh', 'npm install -g different-tool', '```');
    const result = extractInstallCommands(readme);
    expect(result[0].prerequisites).toEqual(['Requires Node.js 20+, Python 3.10 or later.']);
    expect(result[0].prerequisiteTargets?.[0].kind).toBe('prose');
    expect(result).toHaveLength(1);
  });

  test("prose prerequisites do not cross stated CPU targets", () => {
    const readme = md('## Installation', '### Requirements for Apple Silicon',
      'Node.js 20+', '', '### Intel Mac', '```sh', 'brew install tool', '```');
    expect(extractInstallCommands(readme)[0].prerequisites).toBeUndefined();
  });

  test("nested install headings cannot promote build and troubleshooting commands", () => {
    for (const heading of ['Build from source', 'Troubleshooting']) {
      const readme = md(`<h2>${heading}</h2>`, '', '### Install',
        'Requires Python 3.10 or later.', '', '```sh', 'pip install example-tool', '```');
      expect(extractInstallCommands(readme)).toEqual([]);
    }
  });
});

test("mixed HTML lead-ins and inline formatting retain targets and visible wording", () => {
  const readme = md('## Installation', '<p>On Linux ARM64:</p>', '',
    '```sh', 'curl https://example.com/install | sh', '```',
    '### Requirements', 'Requires <strong>Node.js 20+</strong>.', '',
    '### Package', '```sh', 'npm install -g tool', '```');
  const result = extractInstallCommands(readme);
  expect(result[0]).toMatchObject({ platform: 'linux', arches: ['arm64'] });
  expect(result[1].prerequisites).toEqual(['Requires Node.js 20+.']);
});

test("HTML requirement list items do not inherit a sibling's OS or CPU", () => {
  const [command] = extractInstallCommands('<h2>Installation</h2><h3>Requirements</h3>' +
    '<ul><li>Node.js 20+ on Linux ARM64</li><li>Python 3.10 or later</li></ul>' +
    '<h3>Package</h3><pre><code>npm install -g tool</code></pre>');
  expect(command.prerequisiteTargets).toMatchObject([
    { command: 'Node.js 20+ on Linux ARM64', platform: 'linux', arches: ['arm64'] },
    { command: 'Python 3.10 or later', platform: 'universal' },
  ]);
  expect(command.prerequisiteTargets?.[1].arches).toBeUndefined();
});

describe("live README coverage regressions", () => {
  test("inline installation lists retain known package methods and reject unsafe chains", () => {
    const readme = md('## Installation', '### Linux', '- Arch: `pacman -S sample`',
      '- Ubuntu: `apt install sample`', '### macOS', '- Homebrew: `brew install sample`',
      '- Custom: `brew install sample && evil`', '## Usage',
      'An unrelated example is `pip install example-dependency`.');
    expect(commands(readme)).toEqual(['pacman -S sample', 'apt install sample', 'brew install sample']);
  });

  test("GFM installation tables retain each row target without promoting unrelated tables", () => {
    const readme = md('## Installation', '| Platform | Instructions |', '| --- | --- |',
      '| Linux ARM64 | `cargo install sample --locked` |',
      '| Windows x64 | `cargo install sample --locked` |',
      '| macOS | `brew install sample` |',
      '## Examples', '| Example | Command |', '| --- | --- |',
      '| Dependency | `pip install unrelated` |');
    expect(extractInstallCommands(readme)).toMatchObject([
      { command: 'cargo install sample --locked', platform: 'linux', arches: ['arm64'] },
      { command: 'cargo install sample --locked', platform: 'windows', arches: ['amd64'] },
      { command: 'brew install sample', platform: 'macos' },
    ]);
    expect(extractInstallCommands(readme)).toHaveLength(3);
  });

  test("Homebrew respects stated Linux or shared OS support and keeps its default", () => {
    const readme = md('## Installation', '```sh', '# macOS or Linux', 'brew install sample', '```',
      '### Linux ARM64', '```sh', 'brew install other', '```');
    expect(extractInstallCommands(readme)).toMatchObject([
      { command: 'brew install sample', platform: 'macos', platforms: ['macos', 'linux'] },
      { command: 'brew install other', platform: 'linux', arches: ['arm64'] },
    ]);
    expect(extractInstallCommands('```sh\nbrew install sample\n```')[0].platform).toBe('macos');
  });

  test("a package install that compiles is kept while source-build sections stay excluded", () => {
    const readme = md('## How do I install it?', 'You can also install (compile) with cargo:',
      '```sh', 'cargo install --locked sample', '```', '### Build from source',
      'You can also install (compile) with cargo:', '```sh', 'cargo install other', '```');
    expect(commands(readme)).toEqual(['cargo install --locked sample']);
  });

  test("runtime prose and inline version numbers remain requirements, not commands", () => {
    const readme = md('## Installation', 'Requires Node.js `20+` and Python `3.10 or later`.', '',
      '`npm install -g sample`', '', '## Usage', '`npm install -g unrelated`');
    const [command] = extractInstallCommands(readme);
    expect(command.command).toBe('npm install -g sample');
    expect(command.prerequisites).toEqual(['Requires Node.js 20+ and Python 3.10 or later.']);
    expect(extractInstallCommands(readme)).toHaveLength(1);
  });
});

test("explicit unsupported systems remain unknown rather than universal", () => {
  const readme = md('## Installation', '<details><summary>BSD</summary>', '',
    '| Source | Instructions |', '| --- | --- |', '| crates.io | `cargo install sample` |',
    '</details>');
  expect(extractInstallCommands(readme)[0].platform).toBe('unknown');
});

describe("README structure and command regressions", () => {
  const fence = (command: string, lang = 'sh') => `\`\`\`${lang}\n${command}\n\`\`\``;
  const extract = (body: string) => extractInstallCommands(`## Installation\n${body}`);

  test("explicit Windows MSYS2 and WSL contexts remain distinct", () => {
    expect(extract('### Windows MSYS2\n`pacman -S $MINGW_PACKAGE_PREFIX-sample`')[0].platform).toBe('windows');
    expect(extract('### Windows (WSL)\n' + fence('curl https://vendor.example/install | sh'))[0].platform).toBe('linux');
    expect(extract('### Termux\n' + fence('apt install sample'))[0].platform).toBe('unknown');
  });

  test("inline clauses keep their own OS and Intel Macs constrain both OS and CPU", () => {
    const results = extract('On macOS run `brew install sample`, and on Windows run `choco install sample`.');
    expect(results.map(({ platform, platforms }) => ({ platform, platforms }))).toEqual([
      { platform: 'macos', platforms: undefined }, { platform: 'windows', platforms: undefined },
    ]);
    expect(extract('### Intel Macs\n' + fence('cargo install sample'))[0]).toMatchObject({ platform: 'macos', arches: ['amd64'] });
  });

  test("HTML tables preserve inline commands and row targets", () => {
    expect(extract('<table><tr><td>Linux ARM64</td><td><code>cargo install sample</code></td></tr></table>'))
      .toMatchObject([{ command: 'cargo install sample', platform: 'linux', arches: ['arm64'] }]);
  });

  test("details summary targets survive an intervening lead paragraph", () => {
    expect(extract('<details><summary>Linux ARM64</summary>\n\nInstall the latest version:\n\n' +
      fence('curl https://vendor.example/install | sh') + '\n</details>')[0])
      .toMatchObject({ platform: 'linux', arches: ['arm64'] });
  });

  test("promptless console commands, valued sudo options, and quoted hashes survive", () => {
    expect(extract(fence('pipx install sample', 'console'))).toHaveLength(1);
    expect(extract(fence('sudo -u root apt install sample'))[0].command).toBe('sudo -u root apt install sample');
    const command = "pip install sample --config-settings='message=hello # world'";
    expect(extract(fence(command))[0].command).toBe(command);
  });

  test("additional package managers and equals-form Winget IDs are recognized", () => {
    const commands = ['nix-env -iA nixpkgs.sample', 'nix profile install nixpkgs#sample',
      'flatpak install flathub org.example.Sample', 'snap install sample',
      'docker pull vendor/sample', 'apk add sample', 'port install sample',
      'xbps-install -S sample', 'winget install --id=Vendor.Sample'];
    expect(extract(fence(commands.join('\n'))).map(({ command }) => command)).toEqual(commands);
  });

  test("struck instructions and usage or SDK examples are excluded", () => {
    expect(extract('~~`apt install sample`~~')).toEqual([]);
    expect(extract('### SDK\n' + fence('pip install sample'))).toEqual([]);
    expect(extract('### Usage examples\n' + fence('uv tool install unrelated'))).toEqual([]);
  });

  test("adjacent following requirements attach without leaking across sections or prose", () => {
    expect(extract(fence('pipx install sample') + '\nRequires Python 3.10.')[0].prerequisites).toEqual(['Requires Python 3.10.']);
    expect(extract(fence('pipx install sample') + '\n## Other\nRequires Python 3.10.')[0].prerequisites).toBeUndefined();
    expect(extract(fence('pipx install sample') + '\n\nSee the guide.\n\nRequires Python 3.10.')[0].prerequisites).toBeUndefined();
  });

  test("optional dependencies do not become required installation steps", () => {
    expect(extract('### Optional dependencies\n' + fence('apt install make') + '\n### Package\n' + fence('pipx install sample'))[0].prerequisites).toBeUndefined();
    expect(extract('Requires optional Git support.\n\n' + fence('pipx install sample'))[0].prerequisites).toBeUndefined();
    expect(extract('Requires Python 3.10. Git is optional.\n\n' + fence('pipx install sample'))[0].prerequisites).toEqual(['Requires Python 3.10.']);
  });
});
