import { ImageResponse } from "next/og";

export const size = { width: 1200, height: 630 };
export const contentType = "image/png";
export const alt = "Yatko (yatko.app) — clean downloads for GitHub releases";

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

export default async function Image() {
  const title = "Yatko";
  const tagline = "Clean download links for any public GitHub release";
  const font = await loadOutfitFont(`yatko.app${title}${tagline}`);

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
        <div style={{ display: "flex", fontSize: 28, color: "#a1a1aa", marginBottom: 28 }}>
          yatko.app
        </div>
        <div style={{ display: "flex", fontSize: 96, fontWeight: 600, letterSpacing: "-0.04em" }}>
          {title}
        </div>
        <div
          style={{
            display: "flex",
            fontSize: 28,
            color: "#a1a1aa",
            marginTop: 28,
            maxWidth: 900,
            textAlign: "center",
          }}
        >
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
