import { describe, expect, test } from "bun:test";
import { isAllowedMarkdownImageSrc, resolveRepoContentUrl } from "./markdown";

describe("isAllowedMarkdownImageSrc", () => {
  test("allows GitHub-controlled https hosts", () => {
    expect(isAllowedMarkdownImageSrc("https://raw.githubusercontent.com/o/r/HEAD/img.png")).toBe(
      true,
    );
    expect(
      isAllowedMarkdownImageSrc("https://user-images.githubusercontent.com/1/abc.png"),
    ).toBe(true);
    expect(isAllowedMarkdownImageSrc("https://camo.githubusercontent.com/abc")).toBe(true);
    expect(isAllowedMarkdownImageSrc("https://avatars.githubusercontent.com/u/1")).toBe(true);
    expect(isAllowedMarkdownImageSrc("https://github.com/user-attachments/assets/abc")).toBe(
      true,
    );
    expect(isAllowedMarkdownImageSrc("https://www.github.com/o/r/raw/main/x.png")).toBe(true);
  });

  test("rejects attacker and non-https hosts", () => {
    expect(isAllowedMarkdownImageSrc("https://evil.tld/px.gif")).toBe(false);
    expect(isAllowedMarkdownImageSrc("http://raw.githubusercontent.com/o/r/HEAD/x.png")).toBe(
      false,
    );
    expect(isAllowedMarkdownImageSrc("//evil.tld/px.gif")).toBe(false);
    expect(isAllowedMarkdownImageSrc("https://evilgithubusercontent.com/x.png")).toBe(false);
    expect(isAllowedMarkdownImageSrc("https://notgithub.com/x.png")).toBe(false);
    expect(isAllowedMarkdownImageSrc("data:image/png;base64,abc")).toBe(false);
    expect(isAllowedMarkdownImageSrc("/relative/path.png")).toBe(false);
  });
});

describe("resolveRepoContentUrl", () => {
  test("rewrites relative paths onto raw.githubusercontent.com", () => {
    expect(resolveRepoContentUrl("docs/logo.png", "acme", "app", "v1")).toBe(
      "https://raw.githubusercontent.com/acme/app/v1/docs/logo.png",
    );
  });
});
