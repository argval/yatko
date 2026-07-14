export function CollapsibleCard({
  title,
  defaultOpen = true,
  children,
}: {
  title: string;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  return (
    <details open={defaultOpen} className="border border-border rounded-xl bg-surface/60 group">
      <summary className="px-6 sm:px-8 py-5 cursor-pointer font-semibold tracking-tight text-lg flex items-center justify-between select-none">
        {title}
        <svg
          width="16"
          height="16"
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="transition-transform group-open:rotate-180"
        >
          <path d="M4 6l4 4 4-4" />
        </svg>
      </summary>
      <div className="px-6 sm:px-8 pb-6 sm:pb-8">{children}</div>
    </details>
  );
}
