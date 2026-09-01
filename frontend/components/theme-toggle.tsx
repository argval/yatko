import { GITHUB_REPO_URL, TWITTER_URL } from "@/lib/site";
import { chromeIconButton } from "./chrome";
import { ThemeButton } from "./theme-button";

/** Static links render without hydration; only the theme control is interactive. */
export function ThemeToggle() {
  return (
    <div className="fixed top-4 right-4 flex items-center gap-2 z-50">
      <a
        href={GITHUB_REPO_URL}
        target="_blank"
        rel="noopener noreferrer"
        className={chromeIconButton}
        aria-label="View source on GitHub"
        title="View source on GitHub"
      >
        <GitHubIcon />
      </a>
      <a
        href={TWITTER_URL}
        target="_blank"
        rel="noopener noreferrer"
        className={chromeIconButton}
        aria-label="Follow on X"
        title="Follow on X"
      >
        <XIcon />
      </a>
      <ThemeButton />
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

function XIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 1200 1227" fill="currentColor" aria-hidden>
      <path d="M714 519 1160 0h-106L667 450 361 0H0l468 681L0 1227h106l409-476 327 476h361L714 519Zm-145 169-48-69L136 69h164l310 444 48 69 404 578H898L569 688Z" />
    </svg>
  );
}
