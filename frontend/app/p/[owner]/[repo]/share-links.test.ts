import { test, expect } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { ShareLinks, embedButtonHtml } from "./share-links";

test("embed button HTML links the landing page and hosts the badge on yatko.app", () => {
  expect(embedButtonHtml("cli", "cli")).toBe(
    '<a href="https://yatko.app/cli/cli"><img alt="Get it on Yatko" src="https://yatko.app/badge.svg" height="54"></a>',
  );
});

test("share values scroll horizontally instead of truncating long URLs", () => {
  const html = renderToStaticMarkup(createElement(ShareLinks, { owner: "pingdotgg", repo: "t3code" }));
  const scrollContainers = html.match(/overflow-x-auto/g) ?? [];

  expect(scrollContainers).toHaveLength(6);
  expect(html).not.toContain("truncate");
});
