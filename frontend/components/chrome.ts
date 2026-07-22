/** Shared glass chrome for fixed corner controls (theme, GitHub, back). */
export const chromeControl =
  "rounded-lg border border-border bg-surface/80 backdrop-blur-sm hover:bg-foreground/5 active:scale-[0.98] transition-[background-color,transform,color] duration-150";

export const chromeIconButton = `p-2 ${chromeControl}`;

export const chromeTextButton =
  `inline-flex items-center gap-1.5 px-3 py-2 text-sm text-muted hover:text-foreground ${chromeControl}`;
