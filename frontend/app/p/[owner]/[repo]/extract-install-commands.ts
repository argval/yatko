// Install Command module — types + README extraction.
// Presentation (InstallCommands) imports downward from here; do not reverse that seam.

import type { Code, Heading, Nodes, Parent } from "mdast";
import { fromMarkdown } from "mdast-util-from-markdown";
import type { Arch } from "./pick-asset";
import { isInstallScriptOneLiner, lexPipeline } from "./install-shell-syntax";

export { isInstallScriptOneLiner, isUnsafeInstallCommand } from "./install-shell-syntax";

export type InstallOS = "macos" | "windows" | "linux";
/** `universal` = cross-platform package manager; `unknown` = the README doesn't say. */
export type InstallPlatform = InstallOS | "universal" | "unknown";
export type InstallArch = Exclude<Arch, "">;

export type InstallCommand = {
  /** Exact README text (continuation lines kept); never executed. */
  command: string;
  /** First stated OS; see `platforms` when the README names several. */
  platform: InstallPlatform;
  /** Every OS the README states for this command, only when more than one. */
  platforms?: InstallOS[];
  /** Architectures stated in the command or its README context. */
  arches?: InstallArch[];
  /** Prerequisite commands the README lists earlier in the same section. */
  prerequisites?: string[];
  /** The target of each prerequisite; use this when filtering for a visitor. */
  prerequisiteTargets?: InstallPrerequisite[];
  /** README section trail (headings, then lead-in text) the command came from. */
  evidence?: string;
};

export type InstallPrerequisite = Pick<InstallCommand, "command" | "platform" | "platforms" | "arches" | "evidence">;

/** True when the command text looks tied to this owner/repo (not a dep install). */
export function commandMentionsRepo(command: string, owner: string, repo: string): boolean {
  const lower = command.toLowerCase();
  const o = owner.toLowerCase();
  const r = repo.toLowerCase();
  if (!o || !r) return false;
  if (lower.includes(`${o}/${r}`)) return true;
  if (lower.includes(`@${o}/${r}`)) return true;
  if (lower.includes(r)) return true;
  // Package managers commonly use a different separator (`t3-code` vs `t3code`).
  const compactRepo = compactIdentifier(r);
  if (compactRepo && compactIdentifier(lower).includes(compactRepo)) return true;
  // `npx t3@latest` is the published CLI name for the `t3code` repository.
  const npxPackage = lower.match(/\bnpx\s+(?:-[^\s]+\s+)*([@a-z0-9][a-z0-9./_-]*)/i)?.[1];
  if (npxPackage) {
    const compactPackage = compactIdentifier(npxPackage);
    if (compactPackage.length >= 2 && compactRepo.startsWith(compactPackage)) return true;
  }
  // Primary token of multi-segment names (hermes-agent → hermes) when long enough
  // to avoid matching short noise like "go" / "ai".
  const primary = r.split(/[-_]/)[0];
  if (primary && primary.length >= 4) {
    const re = new RegExp(`(?:^|[^a-z0-9])${escapeRegExp(primary)}(?:[^a-z0-9]|$)`, "i");
    if (re.test(lower)) return true;
  }
  return false;
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function compactIdentifier(value: string): string {
  return value.replace(/[^a-z0-9]/gi, "");
}

// ---- Command recognition ------------------------------------------------------

type Recognized = {
  /** OS fixed by the tool itself, `universal`, or `script` (needs README context). */
  platform: InstallOS | "universal" | "script";
  /** Project-local step (`npm install`, `pip install -e .`, `cargo install --path .`). */
  local: boolean;
  /** Every package is a compiler / -dev header, i.e. a build prerequisite. */
  toolchain: boolean;
};

const SYSTEM_PM: [RegExp, InstallOS][] = [
  [/^brew install$/, "macos"],
  [/^(?:apt|apt-get|dnf|yum) install$/, "linux"],
  [/^zypper (?:install|in)$/, "linux"],
  [/^(?:pacman|yay|paru) -S[yu]*$/, "linux"],
  [/^(?:winget|choco|scoop) install$/, "windows"],
];

// Verb prefixes of cross-platform package managers, longest first.
const UNIVERSAL_PM = [
  /^(?:python3?|py) -m pip install$/,
  /^uv tool install$/,
  /^uv pip install$/,
  /^yarn global add$/,
  /^(?:pip3?|pipx|gem|cargo|go) install$/,
  /^npm (?:install|i|add)$/,
  /^(?:yarn|pnpm|bun) add$/,
  /^(?:pnpm|bun) (?:install|i)$/,
  /^npx$/,
];

const TOOLCHAIN_PKG =
  /^(?:build-essential|base-devel|gcc|g\+\+|clang|llvm|make|cmake|automake|autoconf|libtool|pkg-?config|pkgconf|ninja(?:-build)?|meson)$|-(?:dev|devel)$/i;
const LOCAL_FLAG = /^(?:-r|-e|--requirement|--editable|--path)$/;
const PATH_ARG = /^(?:\.{1,2}|\.{1,2}\/.*|\/.*)$/;

/** Classify one logical command line, or null when it isn't a promotable install. */
function recognize(command: string): Recognized | null {
  if (isInstallScriptOneLiner(command)) {
    const ps = !/^(?:curl|wget)\b/.test(command);
    return { platform: ps ? "windows" : "script", local: false, toolchain: false };
  }
  const stages = lexPipeline(command);
  if (stages?.length !== 1) return null;
  const words = stages[0];
  let start = 0;
  if (words[0] === "sudo" || words[0] === "doas") for (start = 1; words[start]?.startsWith("-"); start++);

  for (let n = 4; n >= 1; n--) {
    // Lowercase only the tool name: `pacman -S` and `-s` differ.
    const verb = [words[start]?.toLowerCase(), ...words.slice(start + 1, start + n)].join(" ");
    const args = words.slice(start + n);
    const packages = args.filter((w) => !w.startsWith("-"));
    const system = SYSTEM_PM.find(([re]) => re.test(verb));
    if (system) {
      if (packages.length === 0) return null;
      return { platform: system[1], local: false, toolchain: packages.every((p) => TOOLCHAIN_PKG.test(p)) };
    }
    if (UNIVERSAL_PM.some((re) => re.test(verb))) {
      const local =
        packages.length === 0 ||
        args.some((w) => LOCAL_FLAG.test(w)) ||
        packages.some((p) => PATH_ARG.test(p));
      return { platform: "universal", local, toolchain: false };
    }
  }
  return null;
}

// ---- README context -----------------------------------------------------------

type Kind = "install" | "prerequisite" | "build" | "other" | "neutral";

// Checked in this order against each context text, innermost first.
const KIND_RULES: [Kind, RegExp][] = [
  ["other", /\b(?:uninstall\w*|troubleshoot\w*|faq|known issues)\b/i],
  [
    "build",
    /\b(?:build(?:ing)?|from source|compil\w+|develop(?:ment|ing)?|contribut\w+|hacking|testing|run(?:ning)? (?:the )?tests)\b/i,
  ],
  ["prerequisite", /\b(?:prerequisites?|requirements?|(?:build |system |runtime )?dependenc(?:y|ies))\b/i],
  ["install", /\b(?:install(?:ation|ing)?|quick ?start|get(?:ting)? started|set ?up|downloads?)\b/i],
];

const OS_RULES: [InstallOS, RegExp][] = [
  ["macos", /\b(?:mac ?os(?: ?x)?|os ?x|darwin|mac|apple silicon)\b/i],
  [
    "linux",
    /\b(?:linux|ubuntu|debian|fedora|arch linux|centos|rhel|red hat|opensuse|suse|alpine|nixos|gentoo|manjaro|mint|raspberry pi|wsl)\b/i,
  ],
  ["windows", /\b(?:windows|win32|win64|powershell)\b/i],
];

const ARCH_RULES: [InstallArch, RegExp][] = [
  ["amd64", /\b(?:x86[_-]64|amd64|x64|intel)\b/i],
  ["arm64", /\b(?:arm64|aarch64|apple silicon|m[1-5])\b/i],
  ["arm", /\b(?:armv[67]\w*|armhf|armel)\b/i],
  ["386", /\b(?:i[36]86|x86(?![_-]64)|32-bit)\b/i],
];

const SHELL_LANGS = new Set([
  "", "sh", "bash", "zsh", "fish", "ksh", "shell", "shellscript", "console", "terminal",
  "shell-session", "sh-session", "shellsession", "bash-session", "powershell", "ps", "ps1",
  "pwsh", "posh", "cmd", "bat", "batch", "nu", "nushell", "text", "txt", "plain", "plaintext", "none",
]);
const SESSION_LANGS = new Set(["console", "shell-session", "sh-session", "shellsession", "bash-session"]);
const WINDOWS_LANGS = new Set(["powershell", "ps", "ps1", "pwsh", "posh", "cmd", "bat", "batch"]);

const PROMPT = /^\s*(?:\$|>|PS(?: [^>\n]*)?>)\s+/;

function nodeText(node: Nodes): string {
  if ("value" in node && typeof node.value === "string") return node.value;
  if ("children" in node) return node.children.map((c) => nodeText(c as Nodes)).join("");
  return "";
}

/** Split a fence into logical commands: strip prompts and `# captions`, join `\` continuations. */
function logicalLines(code: string, promptOnly: boolean): string[] {
  const out: string[] = [];
  let pending: string | null = null;
  for (const raw of code.split("\n")) {
    if (pending !== null) {
      pending += `\n${raw.trimEnd()}`;
      if (!pending.endsWith("\\")) {
        out.push(pending);
        pending = null;
      }
      continue;
    }
    const prompt = raw.match(PROMPT);
    if (promptOnly && !prompt) continue; // console output
    const line = (prompt ? raw.slice(prompt[0].length) : raw).replace(/\s+#.*$/, "").trim();
    if (!line) continue;
    if (line.endsWith("\\")) pending = line;
    else out.push(line);
  }
  if (pending !== null) out.push(pending);
  return out;
}

type Found = {
  command: string;
  rec: Recognized;
  kind: Kind;
  os: InstallOS[];
  arches: InstallArch[];
  trail: Heading[];
  /** Headings a prerequisite applies under (cut above the heading that named it). */
  scope: Heading[];
  evidence: string;
};

/** Code blocks in document order with their heading trail and lead-in paragraphs (innermost first). */
function collectFences(readme: string): { code: Code; trail: Heading[]; leads: string[] }[] {
  const out: { code: Code; trail: Heading[]; leads: string[] }[] = [];
  const headings: Heading[] = [];
  const walk = (node: Nodes, ancestors: Parent[]) => {
    if (node.type === "heading") {
      while (headings.length && headings[headings.length - 1].depth >= node.depth) headings.pop();
      headings.push(node);
      return;
    }
    if (node.type === "code") {
      // Indented code blocks are usually output/examples; only fences count.
      const start = node.position?.start.offset ?? 0;
      if (!/^\s*(?:```|~~~)/.test(readme.slice(start, start + 16))) return;
      const leads: string[] = [];
      let child: Nodes = node;
      for (let i = ancestors.length - 1; i >= 0; i--) {
        const siblings = ancestors[i].children as Nodes[];
        for (let j = siblings.indexOf(child) - 1; j >= 0; j--) {
          if (siblings[j].type === "code") continue;
          if (siblings[j].type === "paragraph") leads.push(nodeText(siblings[j]).trim());
          break;
        }
        child = ancestors[i] as Nodes;
      }
      out.push({ code: node, trail: [...headings], leads });
      return;
    }
    if ("children" in node) for (const c of node.children) walk(c as Nodes, [...ancestors, node as Parent]);
  };
  walk(fromMarkdown(readme), []);
  return out;
}

function matchAll<T>(rules: [T, RegExp][], text: string): T[] {
  return rules.filter(([, re]) => re.test(text)).map(([v]) => v);
}

/** First context text (innermost first) that yields any match. */
function firstMatch<T>(rules: [T, RegExp][], texts: string[]): { values: T[]; index: number } {
  for (let i = 0; i < texts.length; i++) {
    const values = matchAll(rules, texts[i]);
    if (values.length) return { values, index: i };
  }
  return { values: [], index: -1 };
}

function compatible(a: Found, b: Found): boolean {
  const os = (f: Found) => (f.rec.platform === "script" || f.rec.platform === "universal" ? f.os : [f.rec.platform]);
  const [x, y] = [os(a), os(b)];
  const osMatch=x.length === 0 || y.length === 0 || x.some((p) => y.includes(p));
  const archMatch=!a.arches.length || !b.arches.length || a.arches.some((arch)=>b.arches.includes(arch));
  return osMatch && archMatch;
}

/** Keep the same explicit OS/CPU context on installers and their prerequisites. */
function describeFound(found: Found): InstallPrerequisite {
  const contextual = found.rec.platform === "script" || found.rec.platform === "universal";
  const os = contextual ? found.os : [];
  const result: InstallPrerequisite = {
    command: found.command,
    platform: found.rec.platform === "script" ? (os[0] ?? "unknown") :
      found.rec.platform === "universal" ? (os[0] ?? "universal") : found.rec.platform,
  };
  if (os.length > 1) result.platforms = os;
  if (found.arches.length) result.arches = found.arches;
  if (found.evidence) result.evidence = found.evidence;
  return result;
}

/**
 * Extract copyable install commands from README shell fences. Uses Markdown
 * structure (headings, lead-in paragraphs, list items, fence language) to drop
 * build/dependency/troubleshooting steps, attach stated prerequisites, and infer
 * OS/arch only when stated — scripts with no stated OS are `unknown`.
 */
export function extractInstallCommands(
  readme: string,
  opts?: { owner?: string; repo?: string },
): InstallCommand[] {
  const found: Found[] = [];
  const owner = opts?.owner?.trim();
  const repo = opts?.repo?.trim();
  const mentions = (c: string) => !!owner && !!repo && commandMentionsRepo(c, owner, repo);

  for (const { code, trail, leads } of collectFences(readme.replace(/\r\n?/g, "\n"))) {
    const lang = (code.lang ?? "").toLowerCase();
    if (!SHELL_LANGS.has(lang)) continue;
    const headingTexts = trail.map((h) => nodeText(h).trim()).reverse();
    const contexts = [...leads, ...headingTexts];
    const kindHit = firstMatch(KIND_RULES, contexts);
    const contextKind: Kind = kindHit.index < 0 ? "neutral" : kindHit.values[0];
    const leadCount = leads.length;
    // A heading that names the context scopes it: prerequisites apply to its parent section.
    const scope = kindHit.index >= leadCount ? trail.slice(0, trail.length - 1 - (kindHit.index - leadCount)) : trail;
    const evidence = [...trail.map((h) => nodeText(h).trim()), ...leads.slice(0, 1).map((l) => l.slice(0, 120))]
      .filter(Boolean)
      .join(" › ");

    for (const command of logicalLines(code.value, SESSION_LANGS.has(lang))) {
      const rec = recognize(command);
      if (!rec || rec.local) continue;
      let kind = contextKind;
      if (rec.toolchain && kind !== "build" && kind !== "other") kind = "prerequisite";
      if (kind === "prerequisite" && mentions(command)) kind = "install";
      if (kind === "build" || kind === "other") continue;
      const texts = [command, ...(WINDOWS_LANGS.has(lang) ? ["windows"] : []), ...contexts];
      found.push({
        command,
        rec,
        kind,
        os: firstMatch(OS_RULES, texts).values,
        arches: firstMatch(ARCH_RULES, [command, ...contexts]).values,
        trail,
        scope,
        evidence,
      });
    }
  }

  // Explicit install sections keep every method (official aliases like `gh` for cli/cli);
  // context-free commands drop unrelated dep installs when some command names this repo.
  const anyRelevant = found.some((f) => f.kind !== "prerequisite" && mentions(f.command));
  const seen = new Set<string>();
  const results: InstallCommand[] = [];
  found.forEach((f, i) => {
    if (f.kind === "prerequisite") return;
    if (f.kind === "neutral" && anyRelevant && !mentions(f.command)) return;
    const result: InstallCommand = describeFound(f);
    const prerequisites = found.slice(0, i).filter((prerequisite) =>
      prerequisite.kind === "prerequisite" &&
      prerequisite.scope.every((heading, index) => f.trail[index] === heading) &&
      compatible(prerequisite, f));
    if (prerequisites.length) {
      result.prerequisites = [...new Set(prerequisites.map((prerequisite) => prerequisite.command))];
      result.prerequisiteTargets = prerequisites.map(describeFound);
    }
    const key=JSON.stringify([result.command,result.platform,result.platforms,result.arches,result.prerequisiteTargets]);
    if(seen.has(key)) return;
    seen.add(key);
    results.push(result);
  });
  return results;
}
