import { ImageResponse } from "next/og";

export const size = { width: 1200, height: 630 };
export const contentType = "image/png";
export const alt = "Yatko download page preview";

const BACKEND_URL = process.env.BACKEND_URL || "http://localhost:8080";

type Props = { params: Promise<{ owner: string; repo: string }> };

type RepoMeta = { description?: string; avatar_url?: string };

async function fetchRepoMeta(owner: string, repo: string): Promise<RepoMeta> {
  try {
    const res = await fetch(`${BACKEND_URL}/api/release/${owner}/${repo}`, { next: { revalidate: 300 } });
    if (!res.ok) return {};
    return res.json();
  } catch {
    return {};
  }
}

// Fetches only the glyphs this image actually uses from Google Fonts, so the
// card matches the site's Outfit typeface instead of falling back to a
// generic sans-serif.
async function loadOutfitFont(text: string): Promise<ArrayBuffer | null> {
  try {
    const css = await (
      await fetch(`https://fonts.googleapis.com/css2?family=Outfit:wght@600&text=${encodeURIComponent(text)}`)
    ).text();
    const match = css.match(/src: url\((.+?)\) format\('(?:opentype|truetype)'\)/);
    if (!match) return null;
    return await (await fetch(match[1])).arrayBuffer();
  } catch {
    return null;
  }
}

export default async function Image({ params }: Props) {
  const { owner, repo } = await params;
  const { description, avatar_url: avatarUrl } = await fetchRepoMeta(owner, repo);
  const tagline =
    description && description.length > 140 ? description.slice(0, 140).trimEnd() + "…" : description || "Download the latest release";

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
    }
  );
}
