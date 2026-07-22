"use client";

export function PlatformFilterToggle({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
}) {
  return (
    <label className="flex items-center gap-2 text-xs text-muted cursor-pointer select-none hover:text-foreground transition-colors mb-4">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="rounded border-foreground/20 accent-foreground"
      />
      My platform only
    </label>
  );
}
