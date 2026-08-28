import { describe, expect, test } from "bun:test";
import { verifyChecksumCommand } from "./verify-checksum";

describe("verifyChecksumCommand", () => {
  test("uses the native macOS verifier", () => {
    expect(verifyChecksumCommand("abc123", "tool-darwin-arm64.tar.gz", "macos")).toBe(
      "printf '%s  %s\\n' 'abc123' 'tool-darwin-arm64.tar.gz' | shasum -a 256 -c -",
    );
  });

  test("uses sha256sum on Linux and PowerShell on Windows", () => {
    expect(verifyChecksumCommand("abc123", "tool-linux-amd64.tar.gz", "linux")).toBe(
      "printf '%s  %s\\n' 'abc123' 'tool-linux-amd64.tar.gz' | sha256sum -c -",
    );
    expect(verifyChecksumCommand("abc123", "tool's.zip", "windows")).toContain(
      "-LiteralPath 'tool''s.zip'",
    );
  });
});
