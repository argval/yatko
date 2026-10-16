import { describe, expect, test } from "bun:test";
import { isInstallScriptOneLiner, isUnsafeInstallCommand, lexPipeline } from "./install-shell-syntax";

describe("lexPipeline", () => {
  test("splits pipes outside quotes and removes quotes", () => {
    expect(lexPipeline(`curl -fsSL 'https://x/a b' | sh -s -- "--dir=$HOME"`)).toEqual([
      ["curl", "-fsSL", "https://x/a b"],
      ["sh", "-s", "--", "--dir=$HOME"],
    ]);
    expect(lexPipeline("brew install foo \\\n  bar")).toEqual([["brew", "install", "foo", "bar"]]);
  });

  test("rejects substitution, chains, redirects, and unterminated quotes", () => {
    for (const bad of [
      "curl x | sh || true",
      "curl x | sh &",
      "curl x > /tmp/i.sh",
      "bash <(curl x)",
      'sh -c "$(curl x)"',
      "echo `id`",
      "a; b",
      "a && b",
      "curl 'x",
      "a\nb",
    ]) {
      expect(lexPipeline(bad)).toBeNull();
    }
  });
});

describe("isInstallScriptOneLiner / isUnsafeInstallCommand", () => {
  test("allows official fetch-and-run one-liners", () => {
    for (const ok of [
      "curl -fsSL https://x/install.sh | bash",
      "curl -sSf https://sh.rustup.rs | sh -s -- -y",
      "wget -qO- https://x | sudo -E bash -",
      "irm https://x/install.ps1 | iex",
      "iwr -useb https://x | iex",
      "iex (irm https://x/install.ps1)",
      'powershell -ExecutionPolicy ByPass -c "irm https://x/install.ps1 | iex"',
    ]) {
      expect(isInstallScriptOneLiner(ok)).toBe(true);
      expect(isUnsafeInstallCommand(ok)).toBe(false);
    }
  });

  test("rejects extra pipes, non-shell sinks, and chained PowerShell", () => {
    for (const bad of [
      "curl https://x | sh | tee log",
      "curl https://x | python",
	  "curl https://x | bash -c 'echo unexpected'",
	  "curl https://x | sh -lc 'echo unexpected'",
      "cat x | sh",
      "irm https://x | iex; Remove-Item y",
      "iex ((New-Object Net.WebClient).DownloadString('https://x'))",
      'powershell -c "irm https://x | iex" ; evil',
      'iex "& { $(irm https://x) }"',
      "brew install foo && rm -rf /",
    ]) {
      expect(isInstallScriptOneLiner(bad)).toBe(false);
      expect(isUnsafeInstallCommand(bad)).toBe(true);
    }
    expect(isUnsafeInstallCommand("brew install foo")).toBe(false);
  });
});


test("literal forbidden syntax stays forbidden inside quoted arguments", () => {
  for (const value of ["sample$(evil)", "sample`evil`", "sample&&evil"]) {
    expect(lexPipeline(`pip install '${value}'`)).toBeNull();
  }
});

test("script pipes require one HTTPS fetch to stdout with supported options", () => {
  for (const command of [
    'curl --help | sh', 'curl https://x/install --output saved | sh',
    'curl http://x/install | sh', 'curl https://user:secret@x/install | sh',
    'curl https://x/a https://x/b | sh', 'wget https://x/install | sh',
    'irm -OutFile saved https://x/install | iex',
  ]) expect(isInstallScriptOneLiner(command)).toBe(false);
  for (const command of [
    'curl --proto =https --tlsv1.2 -fsSL https://vendor.example/install | sudo -u root bash',
    'wget --output-document=- https://vendor.example/install | sh',
    'irm -Uri https://vendor.example/install | iex',
  ]) expect(isInstallScriptOneLiner(command)).toBe(true);
});
