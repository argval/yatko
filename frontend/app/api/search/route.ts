import { checkBotId } from "botid/server";
import { NextResponse } from "next/server";

const BACKEND_URL = process.env.BACKEND_URL || "http://localhost:8080";
const PROXY_TIMEOUT_MS = 8_000;

/**
 * Homepage autocomplete. Runs on the frontend service so we can verify BotID
 * (invisible challenge) before proxying to the Go backend / GitHub Search path.
 */
export async function GET(request: Request) {
  const verification = await checkBotId();
  if (verification.isBot) {
    return NextResponse.json({ error: "Access denied" }, { status: 403 });
  }

  const url = new URL(request.url);
  const q = url.searchParams.get("q") ?? "";
  const target = new URL("/api/search", BACKEND_URL);
  target.searchParams.set("q", q);

  try {
    const res = await fetch(target, {
      headers: { Accept: "application/json" },
      signal: AbortSignal.timeout(PROXY_TIMEOUT_MS),
      // Search responses are user-specific / short-lived; don't poison the
      // Next data cache with one visitor's autocomplete.
      cache: "no-store",
    });
    const body = await res.text();
    return new NextResponse(body, {
      status: res.status,
      headers: {
        "Content-Type": res.headers.get("Content-Type") ?? "application/json",
      },
    });
  } catch {
    return NextResponse.json(
      { error: "Couldn't reach the search service. Try again in a moment." },
      { status: 502 },
    );
  }
}
