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
