"use client";

import { useState } from "react";

export function useCopy(): [boolean, (text: string) => void] {
  const [copied, setCopied] = useState(false);

  function copy(text: string) {
    navigator.clipboard.writeText(text).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      },
      () => {}, // copy failed (insecure context / denied) — leave the button unchanged
    );
  }

  return [copied, copy];
}
