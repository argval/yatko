"use client";

export function PlatformFilterToggle({
  checked,
  onChange,
  disabled = false,
}: {
  checked: boolean;
  disabled?: boolean;
  onChange: (next: boolean) => void;
}) {
  return (
    <label className="flex items-center gap-2 min-h-11 text-xs text-muted cursor-pointer select-none hover:text-foreground transition-colors mb-4">
      <input
        type="checkbox"
        disabled={disabled}
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="rounded border-foreground/20 accent-foreground"
      />
      Selected target only
    </label>
  );
}
