import Link from "next/link";
import { EXAMPLE_REPOS } from "@/lib/example-repos";

export function HomeExamples() {
  return (
    <div className="space-y-2.5 text-left">
      <p className="text-xs text-muted font-medium">Try one</p>
      <div className="flex flex-wrap gap-2">
        {EXAMPLE_REPOS.map(({ owner, repo }) => {
          const slug = `${owner}/${repo}`;
          return (
            <Link
              key={slug}
              href={`/${owner}/${repo}`}
              prefetch={false}
              className="px-3 py-1.5 rounded-lg text-sm bg-foreground/[0.04] hover:bg-foreground/[0.08] active:scale-[0.98] transition-[background-color,transform] duration-150 font-mono"
            >
              {slug}
            </Link>
          );
        })}
      </div>
    </div>
  );
}
