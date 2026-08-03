import { describe, expect, test } from "bun:test";
import { verifyChecksumCommand } from "./verify-checksum";

describe("verifyChecksumCommand", () => {
  test("emits sha256sum/shasum -c compatible two-space format", () => {
    expect(verifyChecksumCommand("abc123", "tool-darwin-arm64.tar.gz")).toBe(
      'echo "abc123  tool-darwin-arm64.tar.gz" | shasum -a 256 -c -',
    );
  });
});
