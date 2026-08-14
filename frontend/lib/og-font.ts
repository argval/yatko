import { readFile } from "node:fs/promises";
import { join } from "node:path";

/**
 * Outfit SemiBold for ImageResponse / Satori.
 * Loaded once at module scope from a local TTF — Google Fonts CSS without a
 * browser UA returns woff2, which Satori cannot parse and can crash prerender
 * with opaque errors like "Cannot read properties of undefined (reading 'split')".
 */
export const outfitSemiBold = await readFile(
  join(process.cwd(), "assets/fonts/Outfit-SemiBold.ttf"),
);

export const outfitFontOption = {
  name: "Outfit",
  data: outfitSemiBold,
  style: "normal" as const,
  weight: 600 as const,
};
