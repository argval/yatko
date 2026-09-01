import { ImageResponse } from "next/og";
import { EXAMPLE_REPOS } from "@/lib/example-repos";
import { outfitFontOption } from "@/lib/og-font";
import { getRelease, getRepoMeta } from "./backend";

export const size = { width: 1200, height: 630 };
export const contentType = "image/png";
export const alt = "Yatko download page preview";
/** Match release-page ISR (literal required for Next segment config). */
export const revalidate = 3600;

/** Same seed as the release page — enables on-demand ISR for other repos. */
export function generateStaticParams() {
  return EXAMPLE_REPOS.map(({ owner, repo }) => ({ owner, repo }));
}

type Props = { params: Promise<{ owner: string; repo: string }> };

export default async function Image({ params }: Props) {
  const { owner, repo } = await params;
  const result = await getRelease(owner, repo);
  const repoMeta = result.ok ? await getRepoMeta(owner, repo) : null;
  const description = repoMeta?.description;
  const avatarUrl = repoMeta?.avatar_url;
  const tagline =
    description && description.length > 140
      ? description.slice(0, 140).trimEnd() + "…"
      : description || "Download the latest release";

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
          fontFamily: "Outfit",
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
      fonts: [outfitFontOption],
    },
  );
}
