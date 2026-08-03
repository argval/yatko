const HOW_IT_WORKS = [
  {
    title: "Direct download",
    path: "yatko.app/dl/owner/repo",
    body: "Detects the user's platform and redirects straight to the right binary.",
  },
  {
    title: "Landing page",
    path: "yatko.app/owner/repo",
    body: "Same shape as your GitHub URL — swap the domain and it just works.",
  },
  {
    title: "Link API",
    path: "yatko.app/api/link/owner/repo",
    body: "Returns JSON with the resolved download URL - for CI pipelines and scripts.",
  },
] as const;

export function HomeHowItWorks() {
  return (
    <div className="space-y-6 text-left">
      <div className="space-y-2 text-center">
        <h2 className="text-lg font-semibold tracking-tight">How it works</h2>
        <p className="text-sm text-muted leading-relaxed max-w-md mx-auto">
          Yatko serves the right GitHub release installation file for each visitor&apos;s OS and
          architecture — landing page, direct download, or JSON link API.
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {HOW_IT_WORKS.map((card) => (
          <div key={card.title} className="rounded-xl border border-border bg-surface/60 p-5 space-y-2">
            <p className="text-sm font-medium tracking-tight">{card.title}</p>
            <p className="text-xs text-muted/80 font-mono break-all">{card.path}</p>
            <p className="text-xs text-muted leading-relaxed">{card.body}</p>
          </div>
        ))}
      </div>
    </div>
  );
}
