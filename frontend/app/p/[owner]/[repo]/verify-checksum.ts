import type { Platform } from "./pick-asset";

function quotePosix(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function quotePowerShell(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

/** Build a shell-safe, platform-native SHA256 verification command. */
export function verifyChecksumCommand(hash: string, filename: string, platform: Platform): string {
  if (platform === "windows") {
    return `if ((Get-FileHash -LiteralPath ${quotePowerShell(filename)} -Algorithm SHA256).Hash -ieq ${quotePowerShell(hash)}) { "SHA256 verified" } else { throw "SHA256 mismatch" }`;
  }

  const command = platform === "macos" ? "shasum -a 256 -c -" : "sha256sum -c -";
  return `printf '%s  %s\\n' ${quotePosix(hash)} ${quotePosix(filename)} | ${command}`;
}
