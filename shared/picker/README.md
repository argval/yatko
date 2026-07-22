# Asset Picker — shared contract

**Deep module:** `backend/picker` (Go).  
**Browser adapter:** `frontend/app/p/[owner]/[repo]/pick-asset.ts` (same ranking rules).  
**Test surface:** `fixtures.json` — both runtimes must agree on every case.

When changing ranking:

1. Change Go first and update/add fixtures.
2. Mirror the change in the TypeScript adapter.
3. Run `go test ./picker/` and `bun test` in `frontend/`.

Do not add browser-only ranking shortcuts (keyword fallbacks, softer arch filters) — they create silent `/dl` vs button drift.
