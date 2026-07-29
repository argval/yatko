import { ImageResponse } from "next/og";
import { getRelease } from "./backend";
import { BACKEND_FETCH_REVALIDATE_SECONDS } from "@/lib/backend-env";

export const size = { width: 1200, height: 630 };
export const contentType = "image/png";
export const alt = "Yatko download page preview";
/** Match release-page ISR so social cards aren't regenerated on every share unfurl. */
export const revalidate = BACKEND_FETCH_REVALIDATE_SECONDS;

type Props = { params: Promise<{ owner: string; repo: string }> };

// Fetches only the glyphs this image actually uses from Google Fonts, so the
// card matches the site's Outfit typeface instead of falling back to a
// generic sans-serif.
async function loadOutfitFont(text: string): Promise<ArrayBuffer | null> {
  try {
    const cssRes = await fetch(
      `https://fonts.googleapis.com/css2?family=Outfit:wght@600&text=${encodeURIComponent(text)}`,
    );
    if (!cssRes.ok) return null;
    const css = await cssRes.text();
    const match = css.match(/src: url\((.+?)\) format\('(?:opentype|truetype)'\)/);
    if (!match?.[1]) return null;
    const fontRes = await fetch(match[1]);
    if (!fontRes.ok) return null;
    return await fontRes.arrayBuffer();
  } catch {
    return null;
  }
}

export default async function Image({ params }: Props) {
  const { owner, repo } = await params;
  const result = await getRelease(owner, repo);
  const description = result.ok ? result.data.description : undefined;
  const avatarUrl = result.ok ? result.data.avatar_url : undefined;
  const tagline =
    description && description.length > 140
      ? description.slice(0, 140).trimEnd() + "…"
      : description || "Download the latest release";

  const font = await loadOutfitFont(`yatko.app${owner}/${repo}${tagline}`);

  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          background: "#09090b",
          color: "#fafafa",
          fontFamily: font ? "Outfit" : undefined,
          padding: "0 80px",
        }}
      >
        <div style={{ display: "flex", fontSize: 28, color: "#a1a1aa", marginBottom: 28 }}>yatko.app</div>
        <div style={{ display: "flex", alignItems: "center", gap: 24 }}>
          {avatarUrl && (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={avatarUrl} width={80} height={80} style={{ borderRadius: 18 }} />
          )}
          <div style={{ display: "flex", fontSize: 64, fontWeight: 600 }}>
            <span style={{ color: "#a1a1aa" }}>{owner}/</span>
            <span>{repo}</span>
          </div>
        </div>
        <div style={{ display: "flex", fontSize: 26, color: "#a1a1aa", marginTop: 28, maxWidth: 900, textAlign: "center" }}>
          {tagline}
        </div>
      </div>
    ),
    {
      ...size,
      fonts: font ? [{ name: "Outfit", data: font, style: "normal", weight: 600 }] : [],
    },
  );
}
