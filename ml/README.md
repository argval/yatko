# assetml training

Learns OS / CPU arch / libc from release asset filenames. Labels come from real file headers.
The Go side (`backend/assetml`) embeds the exported `model.json`.

1. Harvest labels (writes JSONL): `cd backend && go run ./cmd/harvest` (see its flags; output goes in `ml/data/`, git-ignored).
2. Train + export: `uv run --project ml python ml/train.py --data ml/data/harvest.jsonl --out backend/assetml/model.json`
3. Test: `uv run --project ml pytest ml`, `uv run --project ml ruff check ml`, `cd backend && go test ./assetml`
