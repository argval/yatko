import { describe, expect, test } from "bun:test";
import { normalizeSearchQuery, parseInput } from "./use-repo-search";

describe("parseInput", () => {
  test("extracts owner/repo from GitHub URLs", () => {
    expect(parseInput("https://github.com/astral-sh/uv")).toEqual({
      owner: "astral-sh",
      repo: "uv",
    });
    expect(parseInput("http://github.com/astral-sh/uv")).toEqual({
      owner: "astral-sh",
      repo: "uv",
    });
    expect(parseInput("https://www.github.com/astral-sh/uv")).toEqual({
      owner: "astral-sh",
      repo: "uv",
    });
    expect(parseInput("github.com/astral-sh/uv")).toEqual({
      owner: "astral-sh",
      repo: "uv",
    });
    expect(parseInput("https://github.com/astral-sh/uv/")).toEqual({
      owner: "astral-sh",
      repo: "uv",
    });
    expect(parseInput("https://github.com/astral-sh/uv.git")).toEqual({
      owner: "astral-sh",
      repo: "uv",
    });
    expect(parseInput("https://github.com/astral-sh/uv/releases/tag/0.4.0")).toEqual({
      owner: "astral-sh",
      repo: "uv",
    });
  });

  test("parses plain owner/repo slugs", () => {
    expect(parseInput("astral-sh/uv")).toEqual({ owner: "astral-sh", repo: "uv" });
    expect(parseInput("  astral-sh/uv  ")).toEqual({ owner: "astral-sh", repo: "uv" });
  });

  test("returns null for keyword searches", () => {
    expect(parseInput("uv")).toBeNull();
    expect(parseInput("github.com")).toBeNull();
    expect(parseInput("")).toBeNull();
  });
});

describe("normalizeSearchQuery", () => {
  test("collapses pasted GitHub URLs to owner/repo", () => {
    expect(normalizeSearchQuery("https://github.com/astral-sh/uv")).toBe("astral-sh/uv");
    expect(normalizeSearchQuery("HTTPS://GitHub.com/Astral-Sh/UV")).toBe("astral-sh/uv");
  });

  test("lowercases ordinary queries", () => {
    expect(normalizeSearchQuery("  Clip ")).toBe("clip");
    expect(normalizeSearchQuery("uv")).toBe("uv");
  });
});
