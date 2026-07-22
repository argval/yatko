export function ReleaseLoading() {
  return (
    <main className="flex-1 flex flex-col items-center justify-center px-4 py-20 text-center">
      <svg
        className="animate-spin text-foreground/40"
        width="32"
        height="32"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        aria-hidden
      >
        <path d="M21 12a9 9 0 1 1-9-9" />
      </svg>
      <p className="text-sm text-foreground/40 mt-4" role="status" aria-live="polite">
        Loading release…
      </p>
    </main>
  );
}
