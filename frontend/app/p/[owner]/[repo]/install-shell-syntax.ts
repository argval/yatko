// Static shell syntax checks for README install commands. Nothing here runs a
// command: we only lex text so Yatko promotes plain installs and the official
// fetch-and-run one-liners (curl | sh, irm | iex), never chains or substitution.

/** Pipeline stages, each a list of words with quotes removed. */
export type Pipeline = string[][];

/**
 * Lex a command into pipeline stages. Returns null for syntax Yatko never promotes:
 * `$(…)`, backticks, `;`, `&`/`&&`, `||`, redirects, subshells, bare newlines, or
 * unterminated quotes. `powershell` keeps `(`/`)` as words and `\` as a literal.
 */
export function lexPipeline(command: string, powershell = false): Pipeline | null {
  const stages: string[][] = [[]];
  let word = "";
  let inWord = false;
  const endWord = () => {
    if (inWord) stages[stages.length - 1].push(word);
    word = "";
    inWord = false;
  };

  for (let i = 0; i < command.length; i++) {
    const ch = command[i];
    const next = command[i + 1];
    if (ch === "`" || (ch === "$" && next === "(")) return null;
    if (ch === "\\" && !powershell) {
      if (next === undefined) return null;
      if (next === "\n") endWord();
      else {
        word += next;
        inWord = true;
      }
      i++;
      continue;
    }
    if (ch === "'") {
      const close = command.indexOf("'", i + 1);
      if (close < 0) return null;
      word += command.slice(i + 1, close);
      inWord = true;
      i = close;
      continue;
    }
    if (ch === '"') {
      let j = i + 1;
      for (; j < command.length && command[j] !== '"'; j++) {
        const c = command[j];
        if (c === "`" || (c === "$" && command[j + 1] === "(")) return null;
        if (c === "\\" && !powershell && '$`"\\\n'.includes(command[j + 1] ?? "")) j++;
        word += command[j];
      }
      if (j >= command.length) return null;
      inWord = true;
      i = j;
      continue;
    }
    if (ch === " " || ch === "\t") {
      endWord();
      continue;
    }
    if (ch === "#" && !inWord) break; // trailing comment
    if (ch === "|") {
      if (next === "|") return null;
      endWord();
      if (stages[stages.length - 1].length === 0) return null;
      stages.push([]);
      continue;
    }
    if (powershell && (ch === "(" || ch === ")")) {
      endWord();
      stages[stages.length - 1].push(ch);
      continue;
    }
    if (";&<>()\n\r".includes(ch)) return null;
    word += ch;
    inWord = true;
  }
  endWord();
  return stages.every((s) => s.length > 0) ? stages : null;
}

const POSIX_SHELL = /^(?:ba|z|da|k)?sh$/;
const PS_FETCH = /^(?:irm|iwr|invoke-restmethod|invoke-webrequest)$/i;
const PS_EVAL = /^(?:iex|invoke-expression)$/i;
const PS_HOST = /^(?:powershell|pwsh)(?:\.exe)?$/i;

/** `curl … | sh` / `wget … | sudo bash -s -- …`: one fetch piped into one shell. */
function isPosixScriptPipe(command: string): boolean {
  const stages = lexPipeline(command);
  if (!stages || stages.length !== 2) return false;
  const [fetch, run] = stages;
  if (!/^(?:curl|wget)$/.test(fetch[0])) return false;
  let k = 0;
  if (run[0] === "sudo") for (k = 1; run[k]?.startsWith("-"); k++);
  if (!POSIX_SHELL.test(run[k] ?? "")) return false;
  return !run.slice(k + 1).some((arg) => arg === "--command" || arg.startsWith("--command=") || /^-[^-]*c/.test(arg));
}

/** `irm … | iex`, `iex (irm …)`, or `powershell -c "<one of those>"`. */
export function isPowerShellOneLiner(command: string): boolean {
  const stages = lexPipeline(command, true);
  if (!stages) return false;
  const first = stages[0];
  const hasParens = (words: string[]) => words.some((w) => w === "(" || w === ")");
  if (stages.length === 2) {
    return (
      PS_FETCH.test(first[0]) &&
      !hasParens(first) &&
      stages[1].length === 1 &&
      PS_EVAL.test(stages[1][0])
    );
  }
  if (stages.length !== 1) return false;
  if (PS_EVAL.test(first[0]) && first[1] === "(" && first[first.length - 1] === ")") {
    const inner = first.slice(2, -1);
    return PS_FETCH.test(inner[0] ?? "") && !hasParens(inner);
  }
  const c = first.findIndex((w) => /^-(?:c|command)$/i.test(w));
  if (PS_HOST.test(first[0]) && c > 0 && c === first.length - 2 && !hasParens(first)) {
    const inner = first[c + 1];
    return !PS_HOST.test(inner.trim().split(/\s+/)[0]) && isPowerShellOneLiner(inner);
  }
  return false;
}

/** Official fetch-and-run install one-liners (POSIX or PowerShell). */
export function isInstallScriptOneLiner(command: string): boolean {
  return isPosixScriptPipe(command) || isPowerShellOneLiner(command);
}

/**
 * True when Yatko must not promote the command: anything except one simple
 * command or an official install one-liner. Allowed one-liners still reject
 * substitution and chains.
 */
export function isUnsafeInstallCommand(command: string): boolean {
  if (isInstallScriptOneLiner(command)) return false;
  return lexPipeline(command)?.length !== 1;
}
