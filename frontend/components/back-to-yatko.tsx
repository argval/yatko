import Link from "next/link";
import { chromeTextButton } from "./chrome";

export function BackToYatko() {
  return (
    <Link
      href="/"
      className={`fixed top-4 left-4 z-50 ${chromeTextButton}`}
      aria-label="Back to Yatko homepage"
    >
      <BackIcon />
      Yatko
    </Link>
  );
}

function BackIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden>
      <path
        d="M10 3 5 8l5 5"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
