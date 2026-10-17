# Full-file inspection worker

This is internal operation guidance. The worker reads application payloads from
public GitHub release assets and saves OS and CPU evidence. It never executes a
downloaded application or installer. It runs separately from the web server.

## Enable a shared evidence store

Set exactly one of these variables on both the web process and worker:

- `ARTIFACT_SCAN_DIR`: a persistent directory shared by those processes.
- `ARTIFACT_SCAN_REDIS_URL`: an explicitly selected Redis-protocol URL. Use
  `rediss://` when the Redis connection requires TLS.

The web process leaves full-file inspection disabled when both are unset. It
does not reuse `REDIS_URL`, `KV_URL`, or `UPSTASH_REDIS_URL`. Setting both scan
variables is a configuration error. Redis connections are lazy, have short
timeouts, and do not run a startup `PING`.

Use a dedicated persistent Redis instance or database with adequate capacity,
durable persistence, backups, and a `noeviction` policy. These are evidence
records and pending jobs, rather than disposable cache entries. Completed
records and queue keys have no TTL. The existing 24-hour response cache has no
effect on their lifetime. Redis eviction or storage loss can discard both
facts and pending work.

Local records use atomic file replacement, `fsync`, and file locks. Use a local
persistent filesystem that supports these operations. Share its directory
between worker and web containers on the same host. For separate hosts, use
the dedicated Redis option. Keep the data volume across container replacement.

Starting this worker does not deploy or change the existing web container.
Enable the web-side environment variable separately when the evidence store
is ready. No production or shared service is enabled automatically.

## Build and run the container

From the repository root:

```sh
docker build -f backend/Dockerfile.inspector -t yatko-inspector:local backend
docker volume create yatko-artifact-evidence
docker volume create yatko-artifact-tmp
docker run --rm --init \
  --read-only --cap-drop ALL --security-opt no-new-privileges=true \
  --memory 2g --cpus 2 --pids-limit 64 \
  --mount type=volume,source=yatko-artifact-evidence,target=/data \
  --mount type=volume,source=yatko-artifact-tmp,target=/tmp \
  -e ARTIFACT_SCAN_DIR=/data \
  yatko-inspector:local worker
```

The image uses UID and GID `65532`. New Docker volumes inherit the image's
directory permissions. An existing bind mount must allow that UID to write.
The root filesystem stays read-only. Only `/data` and `/tmp` need writes. Use a
separate temporary volume per worker container. Do not put credentials in a
Dockerfile, inventory file, or image layer.

For Redis, replace the evidence-volume mount and local-store environment flag
with `-e ARTIFACT_SCAN_REDIS_URL`. Supply the value through the process's
environment or your secret manager. Keep the temporary volume. Do not pass a
local-store variable at the same time. The worker needs outbound HTTPS access
to public GitHub release downloads and access to the chosen Redis server.
It does not need a GitHub token.

The image contains a static Go executable, the static `7zzs` binary installed
as `7zz`, and the `inno-inspect` Rust bridge. It builds the bridge from its
locked source and installs its license notices. The Rust builder is pinned by
digest. The official 7-Zip 26.04 Linux ARM64 and x64 downloads are checked
against fixed SHA-256 values before installation. Other image architectures
are rejected. The existing `backend/Dockerfile` is separate.

## Queue and inspect assets

Run commands from `backend/` when using Go directly. Set the same explicit
store variable used by the web process. Native execution also needs `7zz` and
the bridge. The bridge's build instructions are in
[`tools/inno-inspect/README.md`](../tools/inno-inspect/README.md).

```sh
go run ./cmd/artifactscan enqueue -cases /absolute/path/cases.json
go run ./cmd/artifactscan worker -once
go run ./cmd/artifactscan worker
```

`worker -once` drains jobs that are currently due and exits. A failed job can
have a future retry time, so this mode does not wait for all retries. The
continuous worker processes one asset at a time and waits for new or retryable
work. Multiple worker processes can share the store.

The inventory is a JSON array. Each item contains `repo`, `tag`, and GitHub
asset metadata. Preserve the real `id`, `updated_at`, and `digest` fields when
GitHub supplies them. For example:

```json
[
  {
    "repo": "owner/repo",
    "tag": "v1.0.0",
    "asset": {
      "id": 123,
      "name": "Application.dmg",
      "size": 123456,
      "state": "uploaded",
      "updated_at": "2026-10-17T12:00:00Z",
      "browser_download_url": "https://github.com/owner/repo/releases/download/v1.0.0/Application.dmg"
    }
  }
]
```

For a standalone inspection report:

```sh
go run ./cmd/artifactscan inspect \
  -cases /absolute/path/cases.json \
  -out /absolute/path/report.json \
  -workers 1
```

Add `-files /absolute/path/downloads` to inspect existing local files without
network downloads. This offline directory uses filenames equal to the SHA-256
of `browser_download_url`, followed by `.asset`. Files must match the inventory's
declared size and its GitHub digest when provided. The report is updated after
each result. Standalone `inspect` produces a report; it does not populate the
durable queue or evidence store. Use `enqueue` and `worker` for shared evidence.

The worker environment also accepts:

- `ARTIFACT_SCAN_7ZIP`: path to `7zz`. The container sets this path.
- `ARTIFACT_SCAN_INNO`: path to `inno-inspect`. The container sets this path.
- `ARTIFACT_SCAN_DOWNLOAD_DIR`: optional retained-download directory. Without
  it, complete source downloads use temporary storage and are removed after
  inspection. Retained downloads have no automatic eviction. Put them on a
  separate volume with an explicit disk quota and retention process.

## Request behavior and record lifetime

All download redirects, link API responses, and release-page responses use the
same enrichment code. When enabled, the web process batches stored-evidence
lookups and queues missing work within a 250 ms storage budget. It does not run
extractors or full downloads. A first request keeps the current fast fallback
while the worker inspects an unseen asset.

Completed deep payload facts take priority over old shallow installer-template
or launcher CPU observations. Publisher manifest contradictions remain explicit
and block automatic selection. A completed unknown or unsupported result keeps
its concrete reason without erasing useful fast facts. Proven malformed files
are blocked. Storage and network failures do not prove that a binary is bad.

Identity includes the asset URL, ID, name, size, `updated_at`, digest, and the
explicit inspector version. Replacing an asset or changing that version creates
a new job. Download counters and display labels do not invalidate evidence.
Bump `github.DeepInspectorVersion` when parser or extractor behavior changes.
Old version records remain available on disk or Redis; remove them only through
an explicit retention decision after the new version is verified.

A claimed job has a 15-minute lease. Its callback has a 10-minute timeout, so a
healthy attempt should finish before its lease expires. A replacement worker can
claim an expired lease after a crash. The previous worker's token cannot replace
the new result. Transport and other operational failures retry after one minute
and two minutes, with three total attempts. Exhausted failures use state `error`
without artifact metadata. Repeated enqueue does not reset exhausted or terminal
jobs. A changed identity or inspector version creates fresh work.

## Resource limits and disk space

The current inspector bounds each asset as follows:

| Resource | Limit |
| --- | --- |
| Downloaded asset | 2 GiB |
| Total staged decoded files | 4 GiB |
| Entries in one archive listing | 100,000 |
| Nested containers | 6 levels |
| Main executable prefix | 1 MiB |
| Deep payload phase | 8 minutes |
| Worker attempt, including download | 10 minutes |
| Each extractor subprocess | 120 CPU seconds, about 1.5 GiB address/data space |

The subprocess output and command counts are also bounded. Hitting a format or
resource limit retains a specific unknown or unsupported result.

Plan at least 8 GiB of writable temporary disk per serial worker. A full asset
and its staged payloads can coexist. The application limits each attempt, but
does not impose a volume-wide disk quota. Configure host or volume limits and
monitor free space. Interrupted containers can leave temporary directories;
remove those only after their worker has stopped. Evidence records have no
automatic retention cleanup, and an optional retained-download cache can grow
across assets.

The container example limits memory to 2 GiB, CPU to two cores, and processes
to 64. Keep standalone batch inspection at `-workers 1` under those limits.
The CLI's default batch parallelism is two; use a larger memory and disk budget
when selecting multiple batch workers. A memory-limited temporary filesystem
must not be sized as if it were ordinary disk. Use a disk-backed temporary
volume for large payloads.
