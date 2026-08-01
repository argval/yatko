import { describe, expect, test } from "bun:test";
import { matchesQuery, normalizeSearchQuery, parseInput, type SearchItem } from "./use-repo-search";

describe("parseInput", () => {
  test("extracts owner/repo from GitHub URLs", () => {
    expect(parseInput("https://github.com/astral-sh/uv")).toEqual({
      owner: "astral-sh",
      repo: "uv",
    });
    expect(parseInput("https://github.com/facebook/react-native")).toEqual({
      owner: "facebook",
      repo: "react-native",
    });
    expect(parseInput("github.com/astral-sh/uv")).toEqual({
      owner: "astral-sh",
      repo: "uv",
    });
  });

  test("parses plain owner/repo slugs", () => {
    expect(parseInput("astral-sh/uv")).toEqual({ owner: "astral-sh", repo: "uv" });
    expect(parseInput("facebook/react-native")).toEqual({
      owner: "facebook",
      repo: "react-native",
    });
  });

  test("returns null for keyword searches", () => {
    expect(parseInput("uv")).toBeNull();
    expect(parseInput("react-native")).toBeNull();
    expect(parseInput("github.com/oven-sh")).toBeNull();
    expect(parseInput("")).toBeNull();
  });
});

describe("normalizeSearchQuery", () => {
  test("collapses pasted repo URLs to owner/repo", () => {
    expect(normalizeSearchQuery("https://github.com/astral-sh/uv")).toBe("astral-sh/uv");
    expect(normalizeSearchQuery("https://github.com/facebook/react-native")).toBe(
      "facebook/react-native",
    );
  });

  test("collapses owner-only URLs to bare login", () => {
    expect(normalizeSearchQuery("https://github.com/oven-sh")).toBe("oven-sh");
    expect(normalizeSearchQuery("https://github.com/oven-sh/")).toBe("oven-sh");
    expect(normalizeSearchQuery("github.com/cli")).toBe("cli");
  });

  test("keeps bare dashed tokens as-is", () => {
    expect(normalizeSearchQuery("react-native")).toBe("react-native");
    expect(normalizeSearchQuery("oven-sh")).toBe("oven-sh");
    expect(normalizeSearchQuery("setup-uv")).toBe("setup-uv");
  });

  test("strips legacy owner: sentinel", () => {
    expect(normalizeSearchQuery("owner:oven-sh")).toBe("oven-sh");
  });
});

describe("matchesQuery", () => {
  const uv: SearchItem = {
    owner: "astral-sh",
    repo: "uv",
    description: "",
    stars: 1,
    avatar_url: "",
  };
  const setup: SearchItem = {
    owner: "astral-sh",
    repo: "setup-uv",
    description: "",
    stars: 1,
    avatar_url: "",
  };
  const rn: SearchItem = {
    owner: "facebook",
    repo: "react-native",
    description: "",
    stars: 1,
    avatar_url: "",
  };

  test("slug queries use owner match + repo prefix", () => {
    expect(matchesQuery(uv, "astral-sh/uv")).toBe(true);
    expect(matchesQuery(setup, "astral-sh/uv")).toBe(false);
    expect(matchesQuery(rn, "facebook/react-native")).toBe(true);
    expect(matchesQuery(rn, "facebook/react")).toBe(true);
  });

  test("bare tokens match owner or repo substring", () => {
    expect(matchesQuery(rn, "react-native")).toBe(true);
    expect(matchesQuery(setup, "setup-uv")).toBe(true);
    expect(matchesQuery(uv, "astral-sh")).toBe(true);
    expect(matchesQuery(rn, "astral-sh")).toBe(false);
  });
});
