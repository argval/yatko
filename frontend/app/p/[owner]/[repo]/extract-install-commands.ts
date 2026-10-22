// Install Command module — types + README extraction.
// Presentation (InstallCommands) imports downward from here; do not reverse that seam.

import type { BlockContent, Code, Heading, Nodes, Parent, PhrasingContent, RootContent } from "mdast";
import { parseFragment, type DefaultTreeAdapterTypes } from "parse5";
import { fromMarkdown } from "mdast-util-from-markdown";
import { gfmFromMarkdown } from "mdast-util-gfm";
import { gfm } from "micromark-extension-gfm";
import type { Arch } from "./platform-utils";
import { commandStart, isInstallScriptOneLiner, lexPipeline } from "./install-shell-syntax";

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
  /** Prerequisite commands or prose stated in the same section. */
  prerequisites?: string[];
  /** The target of each prerequisite; use this when filtering for a visitor. */
  prerequisiteTargets?: InstallPrerequisite[];
  /** README section trail (headings, then lead-in text) the command came from. */
  evidence?: string;
};

export type InstallPrerequisite = Pick<InstallCommand, "command" | "platform" | "platforms" | "arches" | "evidence"> & {
  /** Prose is a stated requirement, never a runnable command. */
  kind?: "prose";
};

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
  platform: InstallOS | "universal" | "script" | "homebrew";
  /** Project-local step (`npm install`, `pip install -e .`, `cargo install --path .`). */
  local: boolean;
  /** Every package is a compiler / -dev header, i.e. a build prerequisite. */
  toolchain: boolean;
};

const SYSTEM_PM: [RegExp, InstallOS | "homebrew"][] = [
  [/^brew install$/, "homebrew"],
  [/^(?:apt|apt-get|dnf|yum) install$/, "linux"],
  [/^zypper (?:install|in)$/, "linux"],
  [/^(?:pacman|yay|paru) -S[yu]*$/, "linux"],
  [/^(?:winget|choco|scoop) install$/, "windows"],
  [/^snap install$/, "linux"],
  [/^flatpak install$/, "linux"],
  [/^apk add$/, "linux"],
  [/^port install$/, "macos"],
  [/^xbps-install(?: -S)?$/, "linux"],
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
  /^nix-env -iA$/,
  /^nix profile (?:install|add)$/,
  /^docker pull$/,
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
  const start = commandStart(words);

  for (let n = 4; n >= 1; n--) {
    // Lowercase only the tool name: `pacman -S` and `-s` differ.
    const verb = [words[start]?.toLowerCase(), ...words.slice(start + 1, start + n)].join(" ");
    const args = words.slice(start + n);
    const packages = args.flatMap((word) => word.startsWith("--id=") ? [word.slice(5)] : word.startsWith("-") ? [] : [word]);
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
  ["other", /\b(?:uninstall\w*|troubleshoot\w*|faq|known issues|examples?|usage|features?|sdk)\b/i],
  [
    "build",
    /\b(?:build(?:ing)?|from source|compil\w+|develop(?:ment|ing)?|contribut\w+|hacking|testing|run(?:ning)? (?:the )?tests)\b/i,
  ],
  ["prerequisite", /\b(?:prerequisites?|requirements?|(?:build |system |runtime )?dependenc(?:y|ies))\b/i],
  ["install", /\b(?:install(?:ation|ing)?|quick ?start|get(?:ting)? started|set ?up|downloads?)\b/i],
];

const OS_RULES: [InstallOS, RegExp][] = [
  ["macos", /\b(?:mac ?os(?: ?x)?|os ?x|darwin|macs?|apple silicon)\b/i],
  [
    "linux",
    /\b(?:linux|ubuntu|debian|fedora|arch linux|centos|rhel|red hat|opensuse|suse|alpine|nixos|gentoo|manjaro|mint|raspberry pi|wsl)\b/i,
  ],
  ["windows", /\b(?:windows|win32|win64|powershell)\b/i],
];

// Preserve unsupported targets as unknown instead of calling their package commands universal.
const OS_CONTEXT_RULES: [InstallOS | "unknown", RegExp][] = [
  ...OS_RULES,
  ["unknown", /\b(?:android|termux|freebsd|openbsd|netbsd|dragonfly|bsd|ios)\b/i],
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

function nodeText(node: Nodes, omitCode = false): string {
  if (node.type === "html") return ""; // Inline formatting tags are not visible text.
  if (omitCode && node.type === "inlineCode") return "";
  if ("value" in node && typeof node.value === "string") return node.value;
  if ("children" in node) return node.children.map((c) => nodeText(c as Nodes, omitCode)).join(node.type === "tableRow" ? " " : "");
  return "";
}

/** Split a fence into logical commands: strip prompts and `# captions`, join `\` continuations. */
function logicalLines(code: string, promptOnly: boolean): { command: string; caption: string }[] {
  const out: { command: string; caption: string }[] = [];
  let pending: string | null = null;
  let caption = "";
  const hasPrompts = code.split("\n").some((line) => PROMPT.test(line));
  for (const raw of code.split("\n")) {
    if (pending !== null) {
      pending += `\n${raw.trimEnd()}`;
      if (!pending.endsWith("\\")) {
        out.push({ command: pending, caption });
        pending = null;
      }
      continue;
    }
    if (/^\s*#/.test(raw)) {
      caption = raw.replace(/^\s*#\s*/, "");
      continue;
    }
    const prompt = raw.match(PROMPT);
    if (promptOnly && hasPrompts && !prompt) continue; // console output
    const line = stripComment(prompt ? raw.slice(prompt[0].length) : raw).trim();
    if (!line) continue;
    if (line.endsWith("\\")) pending = line;
    else out.push({ command: line, caption });
  }
  if (pending !== null) out.push({ command: pending, caption });
  return out;
}

/** Only an unquoted comment marker can remove the rest of a command. */
function stripComment(line: string): string {
  let quote = "";
  for (let i = 0; i < line.length; i++) {
    if (line[i] === "\\" && quote !== "'") { i++; continue; }
    if (quote) { if (line[i] === quote) quote = ""; continue; }
    if (line[i] === "'" || line[i] === '"') quote = line[i];
    else if (line[i] === "#" && (i === 0 || /\s/.test(line[i - 1]))) return line.slice(0, i);
  }
  return line;
}

type Found = {
  command: string;
  prose?: boolean;
  rec: Recognized;
  kind: Kind;
  os: InstallOS[];
  targetUnknown?: boolean;
  arches: InstallArch[];
  trail: Heading[];
  /** Headings a prerequisite applies under (cut above the heading that named it). */
  scope: Heading[];
  evidence: string;
  /** A requirement immediately after an installer also applies to that installer. */
  follows?: Found;
};

/** Normalize HTML sections to the same structural nodes as Markdown. */
function htmlSections(html: string, scopes: WeakMap<Nodes, string>): BlockContent[] {
  type HtmlNode = DefaultTreeAdapterTypes.ChildNode;
  const text = (node: HtmlNode): string => "value" in node ? node.value :
    "childNodes" in node ? node.childNodes.map(text).join("") : "";
  const inline = (node: HtmlNode): PhrasingContent[] => {
    if ("value" in node) return [{ type: "text", value: node.value }];
    if (!("tagName" in node) || ["script", "style", "template", "del", "s", "strike"].includes(node.tagName)) return [];
    if (node.tagName === "code") return [{ type: "inlineCode", value: text(node) }];
    if (node.tagName === "br") return [{ type: "text", value: "\n" }];
    return node.childNodes.flatMap(inline);
  };
  const convert = (node: HtmlNode): BlockContent[] => {
    if (!("tagName" in node)) return [];
    const tag = node.tagName;
    if (["script", "style", "template", "del", "s", "strike"].includes(tag)) return [];
    const value = text(node).trim();
    const children = [{ type: "text" as const, value }];
    const depths = { h1: 1, h2: 2, h3: 3, h4: 4, h5: 5, h6: 6 } as const;
    if (Object.hasOwn(depths, tag)) return [{ type: "heading", depth: depths[tag as keyof typeof depths], children }];
    if (tag === "pre") {
      const code = node.childNodes.find((child) => "tagName" in child && child.tagName === "code");
      const className = code && "attrs" in code ? code.attrs.find((attr) => attr.name === "class")?.value : "";
      return [{ type: "code", value: text(code ?? node).trim(), lang: className?.match(/(?:language|lang)-([\w-]+)/)?.[1] ?? "" }];
    }
    if (["p", "summary"].includes(tag)) return value ? [{ type: "paragraph", children: node.childNodes.flatMap(inline) }] : [];
    if (tag === "table") {
      const rows = (nodes: HtmlNode[]): HtmlNode[] => nodes.flatMap((child) => "tagName" in child && child.tagName === "tr" ? [child] : "childNodes" in child ? rows(child.childNodes) : []);
      return [{ type: "table", children: rows(node.childNodes).flatMap((row) => "childNodes" in row ? [{ type: "tableRow" as const, children: row.childNodes.flatMap((cell) => "tagName" in cell && ["td", "th"].includes(cell.tagName) ? [{ type: "tableCell" as const, children: cell.childNodes.flatMap(inline) }] : []) }] : []) }];
    }
    if (tag === "details") {
      const summary = node.childNodes.find((child) => "tagName" in child && child.tagName === "summary");
      const block: BlockContent = { type: "blockquote", children: node.childNodes.filter((child) => child !== summary).flatMap(convert) };
      scopes.set(block, summary ? text(summary) : "");
      return [block];
    }
    if (tag === "li") {
      const blocks = node.childNodes.flatMap(convert);
      return [{ type: "blockquote", children: blocks.length ? blocks : value ? [{ type: "paragraph", children: node.childNodes.flatMap(inline) }] : [] }];
    }
    return node.childNodes.flatMap(convert);
  };
  return parseFragment(html).childNodes.flatMap(convert);
}

type ReadmeBlock = { code?: Code; inline?: boolean; prose?: string; trail: Heading[]; leads: string[] };

/** Collect commands in document order, preserving enclosing details and row context. */
function collectBlocks(readme: string): ReadmeBlock[] {
  const out: ReadmeBlock[] = [];
  const htmlScopes = new WeakMap<Nodes, string>();
  const headings: Heading[] = [];
  const walk = (node: Nodes, ancestors: Parent[], scopes: string[] = []) => {
    if (node.type === "delete") return;
    const ownContext = htmlScopes.get(node);
    if (typeof ownContext === "string" && ownContext) scopes = [...scopes, ownContext];
    if (node.type === "heading") {
      while (headings.length && headings[headings.length - 1].depth >= node.depth) headings.pop();
      headings.push(node);
      return;
    }
    if (node.type === "code" || node.type === "paragraph" || node.type === "inlineCode") {
      const start = node.position?.start.offset ?? 0;
      if (node.type === "code" && node.position && !/^\s*(?:```|~~~)/.test(readme.slice(start, start + 16))) return;
      const leads: string[] = [];
      if (node.type === "inlineCode") {
        const container = ancestors.findLast((parent) => parent.type === "paragraph" || parent.type === "tableRow");
        if (container?.type === "paragraph") {
          const index = container.children.indexOf(node);
          let previous = index - 1;
          while (previous >= 0 && container.children[previous].type !== "inlineCode") previous--;
          leads.push(container.children.slice(previous + 1, index).map((child) => nodeText(child as Nodes)).join(""));
        } else if (container) leads.push(nodeText(container as Nodes, true).trim());
      }
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
      leads.push(...scopes.toReversed());
      if (node.type === "code") out.push({ code: node, trail: [...headings], leads });
      else if (node.type === "inlineCode") out.push({ code: { type: "code", value: node.value }, inline: true, trail: [...headings], leads });
      else out.push({ prose: nodeText(node).trim(), trail: [...headings], leads });
      if (node.type !== "paragraph") return;
    }
    if ("children" in node) {
      const parent: Parent = node;
      const children: RootContent[] = [];
      const contexts = new Map<RootContent, string[]>();
      const active = [...scopes];
      for (const child of parent.children) {
        if (child.type === "html") {
          const opens = /<details\b/i.test(child.value);
          const closes = /<\/details\s*>/i.test(child.value);
          const converted = htmlSections(child.value, htmlScopes);
          if (opens && !closes) {
            const block = converted.find((part) => htmlScopes.has(part));
            active.push(block ? htmlScopes.get(block) ?? "" : "");
          }
          for (const part of converted) { children.push(part); contexts.set(part, [...active]); }
          if (closes && !opens) active.pop();
        } else { children.push(child); contexts.set(child, [...active]); }
      }
      parent.children = children;
      for (const child of parent.children) walk(child as Nodes, [...ancestors, parent], contexts.get(child) ?? scopes);
    }
  };
  walk(fromMarkdown(readme, { extensions: [gfm()], mdastExtensions: [gfmFromMarkdown()] }), []);
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

/** The nearest explicit target wins, including targets outside the supported OS picker. */
function readmeOS(texts: string[]): Pick<Found, "os" | "targetUnknown"> {
  const matched = firstMatch(OS_CONTEXT_RULES, texts);
  let values = matched.values;
  if (matched.index >= 0 && /\b(?:wsl|windows subsystem for linux)\b/i.test(texts[matched.index])) values = ["linux"];
  return { os: values.filter((value) => value !== "unknown"), targetUnknown: values.includes("unknown") };
}

function compatible(a: Found, b: Found): boolean {
  const os = (f: Found) => (f.rec.platform === "script" || f.rec.platform === "universal" || f.rec.platform === "homebrew" ? f.os : [f.rec.platform]);
  const [x, y] = [os(a), os(b)];
  const osMatch=x.length === 0 || y.length === 0 || x.some((p) => y.includes(p));
  const archMatch=!a.arches.length || !b.arches.length || a.arches.some((arch)=>b.arches.includes(arch));
  return osMatch && archMatch;
}

/** Keep the same explicit OS/CPU context on installers and their prerequisites. */
function describeFound(found: Found): InstallPrerequisite {
  const contextual = found.rec.platform === "script" || found.rec.platform === "universal" || found.rec.platform === "homebrew";
  const os = contextual ? found.rec.platform === "homebrew" ? found.os.filter((target) => target === "macos" || target === "linux") : found.os : [];
  const unknown = found.targetUnknown || (found.rec.platform === "homebrew" && found.os.length > 0 && os.length === 0);
  const result: InstallPrerequisite = {
    command: found.command,
    platform: found.rec.platform === "script" ? (os[0] ?? "unknown") :
      found.rec.platform === "homebrew" ? (os[0] ?? (unknown ? "unknown" : "macos")) :
      found.rec.platform === "universal" ? (os[0] ?? (unknown ? "unknown" : "universal")) : unknown ? "unknown" : found.rec.platform,
  };
  if (os.length > 1) result.platforms = os;
  if (found.arches.length) result.arches = found.arches;
  if (found.evidence) result.evidence = found.evidence;
  if (found.prose) result.kind = "prose";
  return result;
}

/**
 * Extract copyable install commands from README shell fences and HTML pre/code. Uses
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

  const blocks = collectBlocks(readme.replace(/\r\n?/g, "\n"));
  for (const [blockIndex, { code, inline, prose, trail, leads }] of blocks.entries()) {
    const lang = (code?.lang ?? "").toLowerCase();
    if (!SHELL_LANGS.has(lang)) continue;
    const headingTexts = trail.map((h) => nodeText(h).trim()).reverse();
    const contexts = [...leads, ...headingTexts];
    // A nested method or prerequisite cannot turn build/repair instructions into installation.
    if (headingTexts.some((text) => firstMatch(KIND_RULES, [text]).values.some((kind) => kind === "build" || kind === "other"))) continue;
    const kindHit = firstMatch(KIND_RULES, contexts);
    let contextKind: Kind = kindHit.index < 0 ? "neutral" : kindHit.values[0];
    // Installing a published package may compile it; a compile lead-in alone is not a source-build section.
    if (contextKind === "build" && leads[0] && /\binstall\b/i.test(leads[0]) &&
        firstMatch(KIND_RULES, headingTexts).values[0] === "install" && !/\b(?:from source|build|development)\b/i.test(leads[0])) contextKind = "install";
    if (inline && contextKind !== "install" && contextKind !== "prerequisite") continue;
    const leadCount = leads.length;
    // A heading that names the context scopes it: prerequisites apply to its parent section.
    const scope = contextKind === "prerequisite" && kindHit.index >= leadCount ? trail.slice(0, trail.length - 1 - (kindHit.index - leadCount)) : trail;
    const evidence = [...trail.map((h) => nodeText(h).trim()), ...leads.slice(0, 1).map((l) => l.slice(0, 120))]
      .filter(Boolean)
      .join(" › ");

    if (prose !== undefined) {
      // Keep stated requirements verbatim, without inferring ordinary setup prose.
      if (contextKind !== "prerequisite" && !/^(?:requires?\b|you (?:will )?need\b|prerequisites?\s*:|requirements?\s*:)/i.test(prose)) continue;
      if (!prose || /:\s*$/.test(prose) || /\boptional\b/i.test(headingTexts[0] ?? "")) continue;
      const requirement = prose.split(/(?<=[.!?])\s+/).filter((sentence) => !/\boptional\b/i.test(sentence)).join(" ");
      if (!requirement) continue;
      const previous = found.at(-1);
      const follows = blocks[blockIndex - 1]?.code && previous?.kind !== "prerequisite" ? previous : undefined;
      found.push({ command: requirement, prose: true, rec: { platform: "universal", local: false, toolchain: false },
        kind: "prerequisite", ...readmeOS([prose, ...contexts]),
        arches: firstMatch(ARCH_RULES, [prose, ...contexts]).values, trail, scope, evidence, follows });
      continue;
    }
    if (!code) continue;
    for (const { command, caption } of logicalLines(code.value, SESSION_LANGS.has(lang))) {
      let rec = recognize(command);
      if (!rec || rec.local) continue;
      let kind = contextKind;
      if (rec.toolchain && kind !== "build" && kind !== "other") kind = "prerequisite";
      if (kind === "prerequisite" && mentions(command)) kind = "install";
      if (kind === "build" || kind === "other" || (kind === "prerequisite" && /\boptional\b/i.test(headingTexts[0] ?? ""))) continue;
      const texts = [command, ...(WINDOWS_LANGS.has(lang) ? ["windows"] : []), caption, ...contexts];
      const target = readmeOS(texts);
      if (/^(?:sudo\s+)?pacman\b/.test(command) && target.os.includes("windows")) rec = { ...rec, platform: "windows" };
      if (rec.platform === "homebrew" && target.os.length === 1 && target.os[0] === "linux" && /\b(?:too|also)\b/i.test(leads.join(" "))) target.os.unshift("macos");
      found.push({
        command,
        rec,
        kind,
        ...target,
        arches: firstMatch(ARCH_RULES, [command, caption, ...contexts]).values,
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
    const prerequisites = found.filter((prerequisite, prerequisiteIndex) =>
      (prerequisiteIndex < i || prerequisite.follows === f) &&
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
