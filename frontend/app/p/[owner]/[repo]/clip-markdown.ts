/** Soft cap so a megabyte README cannot dominate Fluid Active CPU (or the
 *  browser main thread). Backend fetch is still capped separately at 1 MB. */
export const MAX_MARKDOWN_CHARS = 100_000;

/** Truncate oversized markdown before the remark/rehype pipeline. */
export function clipMarkdown(source: string, maxChars = MAX_MARKDOWN_CHARS): string {
  if (source.length <= maxChars) return source;
  return `${source.slice(0, maxChars)}\n\n…\n`;
}
