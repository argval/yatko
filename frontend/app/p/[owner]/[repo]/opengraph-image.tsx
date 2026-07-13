import { ImageResponse } from "next/og";

export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

type Props = { params: Promise<{ owner: string; repo: string }> };

export default async function Image({ params }: Props) {
  const { owner, repo } = await params;

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
        }}
      >
        <div style={{ display: "flex", fontSize: 28, color: "#a1a1aa", marginBottom: 28 }}>
          yoink.dev
        </div>
        <div style={{ display: "flex", fontSize: 68, fontWeight: 600 }}>
          <span style={{ color: "#a1a1aa" }}>{owner}/</span>
          <span>{repo}</span>
        </div>
        <div style={{ display: "flex", fontSize: 26, color: "#a1a1aa", marginTop: 28 }}>
          Download the latest release
        </div>
      </div>
    ),
    { ...size }
  );
}
