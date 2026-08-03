import { ImageResponse } from "next/og";

export const size = { width: 32, height: 32 };
export const contentType = "image/png";

/** Simple branded mark for tabs / install prompts. */
export default function Icon() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "#09090b",
          color: "#fafafa",
          fontSize: 20,
          fontWeight: 700,
          letterSpacing: "-0.06em",
        }}
      >
        Y
      </div>
    ),
    { ...size },
  );
}
