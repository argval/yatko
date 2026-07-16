#!/usr/bin/env bash
# Runs the Yatko backend (Go) and frontend (Next.js) together for local dev.
# Ctrl+C stops both.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CLEANED_UP=0
cleanup() {
  [ "$CLEANED_UP" -eq 1 ] && return
  CLEANED_UP=1
  echo "Stopping backend and frontend..."
  kill "${BACKEND_PID:-}" "${FRONTEND_PID:-}" 2>/dev/null || true
  wait "${BACKEND_PID:-}" "${FRONTEND_PID:-}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

(
  cd "$ROOT_DIR/backend"
  go run . 2>&1 | sed -u 's/^/[backend] /'
) &
BACKEND_PID=$!

(
  cd "$ROOT_DIR/frontend"
  bun run dev 2>&1 | sed -u 's/^/[frontend] /'
) &
FRONTEND_PID=$!

wait "$BACKEND_PID" "$FRONTEND_PID"
