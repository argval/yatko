"use client";

import { useSyncExternalStore } from "react";
import { chromeIconButton } from "./chrome";

const themeListeners = new Set<() => void>();

function onStorage(e: StorageEvent) {
  if (e.key !== "theme") return;
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
  themeListeners.forEach((cb) => cb());
}

export function ThemeButton() {
  const dark = useSyncExternalStore(subscribeTheme, getIsDark, () => false);

  return (
    <button
      type="button"
      onClick={() => setTheme(!dark)}
      className={chromeIconButton}
      aria-label={dark ? "Switch to light mode" : "Switch to dark mode"}
      title={dark ? "Switch to light mode" : "Switch to dark mode"}
    >
      {dark ? <SunIcon /> : <MoonIcon />}
    </button>
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
