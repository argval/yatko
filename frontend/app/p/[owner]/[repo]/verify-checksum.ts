/** Build a portable checksum verify one-liner for README / CLI paste. */
export function verifyChecksumCommand(hash: string, filename: string): string {
  // Two spaces is the text-mode format accepted by both sha256sum and shasum -c.
  return `echo "${hash}  ${filename}" | shasum -a 256 -c -`;
}
