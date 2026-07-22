"use client";

import { useEffect, useRef, useState } from "react";

export function useCopy(): [boolean, (text: string) => void] {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  function copy(text: string) {
    navigator.clipboard.writeText(text).then(
      () => {
        setCopied(true);
        if (timerRef.current) clearTimeout(timerRef.current);
        timerRef.current = setTimeout(() => {
          timerRef.current = null;
          setCopied(false);
        }, 2000);
      },
      () => {}, // copy failed (insecure context / denied) — leave the button unchanged
    );
  }

  return [copied, copy];
}
