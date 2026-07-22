"use client";

import { useSyncExternalStore } from "react";
import { chromeIconButton } from "./chrome";

// The <html> `.dark` class (set pre-paint by the inline script in layout.tsx) is the
// single source of truth for theme. Read it via useSyncExternalStore so ThemeToggle
// has no init effect and stays React-Compiler-eligible.
const themeListeners = new Set<() => void>();
function onStorage(e: StorageEvent) {
  if (e.key !== "theme") return;
  // A storage event only means localStorage changed in another tab - this
  // tab's own <html> class is untouched, so apply it here before notifying,
  // or getIsDark() would just re-read the same (stale) local class.
  document.documentElement.classList.toggle("dark", e.newValue === "dark");
  themeListeners.forEach((cb) => cb());
}
function subscribeTheme(cb: () => void) {
  themeListeners.add(cb);
  window.addEventListener("storage", onStorage);
  return () => {
    themeListeners.delete(cb);
    window.removeEventListener("storage", onStorage);
  };
}
function getIsDark() {
  return document.documentElement.classList.contains("dark");
}
function setTheme(next: boolean) {
  document.documentElement.classList.toggle("dark", next);
  localStorage.setItem("theme", next ? "dark" : "light");
  themeListeners.forEach((cb) => cb()); // same-tab notify (storage event won't)
}

export function ThemeToggle() {
  const dark = useSyncExternalStore(subscribeTheme, getIsDark, () => false);

  function toggle() {
    setTheme(!dark);
  }

  return (
    <div className="fixed top-4 right-4 flex items-center gap-2 z-50">
      <a
        href="https://github.com/argval/yatko"
        target="_blank"
        rel="noopener noreferrer"
        className={chromeIconButton}
        aria-label="View source on GitHub"
        title="View source on GitHub"
      >
        <GitHubIcon />
      </a>
      <button
        type="button"
        onClick={toggle}
        className={chromeIconButton}
        aria-label={dark ? "Switch to light mode" : "Switch to dark mode"}
        title={dark ? "Switch to light mode" : "Switch to dark mode"}
      >
        {dark ? <SunIcon /> : <MoonIcon />}
      </button>
    </div>
  );
}

function GitHubIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.09 3.29 9.4 7.86 10.93.57.1.78-.25.78-.55v-1.94c-3.2.7-3.88-1.54-3.88-1.54-.52-1.33-1.28-1.68-1.28-1.68-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.56-.29-5.26-1.28-5.26-5.7 0-1.26.45-2.29 1.19-3.09-.12-.29-.52-1.47.11-3.06 0 0 .97-.31 3.18 1.18a11.1 11.1 0 0 1 5.79 0c2.2-1.49 3.18-1.18 3.18-1.18.63 1.59.23 2.77.11 3.06.74.8 1.18 1.83 1.18 3.09 0 4.43-2.7 5.4-5.28 5.69.42.36.78 1.07.78 2.16v3.2c0 .3.21.66.79.55A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z" />
    </svg>
  );
}

function SunIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="5" />
      <path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  );
}
