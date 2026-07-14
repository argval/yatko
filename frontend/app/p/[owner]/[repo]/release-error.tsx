export function ReleaseError({ message }: { message: string }) {
  return (
    <main className="flex-1 flex flex-col items-center justify-center px-4 py-20 text-center">
      <p className="text-6xl mb-6">⚠️</p>
      <h1 className="text-2xl font-bold mb-2">Something went wrong</h1>
      <p className="text-foreground/50 mb-8 max-w-sm">{message}</p>
      <div className="flex gap-3">
        <a
          href=""
          className="px-4 py-2 rounded-lg bg-foreground text-background text-sm font-medium hover:opacity-80 transition-opacity"
        >
          Try again
        </a>
        <a
          href="/"
          className="px-4 py-2 rounded-lg border border-foreground/10 text-sm font-medium hover:bg-foreground/5 transition-colors"
        >
          Back to search
        </a>
      </div>
    </main>
  );
}
