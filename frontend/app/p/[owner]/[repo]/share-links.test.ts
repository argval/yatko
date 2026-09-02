import { test, expect } from "bun:test";
import { embedButtonHtml } from "./share-links";

test("embed button HTML links the landing page and hosts the badge on yatko.app", () => {
  expect(embedButtonHtml("cli", "cli")).toBe(
    '<a href="https://yatko.app/cli/cli"><img alt="Get it on Yatko" src="https://yatko.app/badge.svg" height="54"></a>',
  );
});
