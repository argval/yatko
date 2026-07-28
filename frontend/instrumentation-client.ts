import { initBotId } from "botid/client/core";

// Protect browser-initiated fetches that hit expensive upstream work.
// Do NOT protect /p/* or /dl/* — those are direct-entry URL-swap paths and
// must work on the first request before any client challenge can run.
initBotId({
  protect: [
    {
      path: "/api/search",
      method: "GET",
    },
  ],
});
