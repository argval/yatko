# Release page monitoring

GitHub Actions starts a production build with a local fixture backend and checks the public `/<owner>/<repo>` route. A release with assets contains a `data-yatko-release` attribute in the response HTML. The smoke test also checks that an upstream error, a missing release, and a release with zero assets omit the attribute.

To gate production releases, make `release-smoke` a required GitHub check for pull requests into `prod`.

To run the same test locally, run `bun ci` and `bun run scripts/release-smoke.mjs` from `frontend/`. The test writes a fixture build to `frontend/.next`, so use a test checkout if you need to preserve your current build.

## UptimeRobot email alerts

The [UptimeRobot keyword monitors](https://uptimerobot.com/keyword-monitoring/) below check the public release pages every five minutes and retry failures before alerting. Each one currently looks for the repo's GitHub download URL in the page's embedded release data.

When the build with the attribute goes live, verify the new keyword in each live page's HTML and update the corresponding monitor. Keep the alert condition set to **Keyword not exists**.

| Monitor name | URL to monitor | Keyword after deployment |
| --- | --- | --- |
| Yatko release · cli/cli | `https://yatko.app/cli/cli` | `data-yatko-release="cli/cli"` |
| Yatko release · godotengine/godot | `https://yatko.app/godotengine/godot` | `data-yatko-release="godotengine/godot"` |
| Yatko release · zed-industries/zed | `https://yatko.app/zed-industries/zed` | `data-yatko-release="zed-industries/zed"` |

Each monitor uses the account email for both **Down** and **Up** alerts, with zero notification delay. Use **Test notification** on a monitor to confirm that test emails reach your inbox. [Notification settings](https://help.uptimerobot.com/en/articles/11360978-understanding-notification-channels-in-uptimerobot) · [Test notifications](https://help.uptimerobot.com/en/articles/11602913-how-to-test-notifications-in-uptimerobot-quick-guide)

If a monitor alerts, open the affected page and the corresponding GitHub releases page. Check whether GitHub still lists assets for the latest release. If it does, inspect the frontend and backend logs in Vercel for the time in the alert.
