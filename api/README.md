# API contracts

`openapi.json` describes the HTTP API; `models.schema.json` contains wire models and `report.schema.json` is the ingestion envelope with typed OS, package and check payloads. Generate all three with `go run ./tools/schema-gen` (use the pinned Go toolchain from Makefile).

OS/package observation fields may be omitted when unknown. Backup/payment checks have different payloads selected by `kind`; unavailable previous success/restore results are omitted or null, never fabricated. Network, WireGuard and disk observations preserve collector-specific structures; their runtime validation also rejects secret field names and excessive nesting/size. The server additionally enforces authorization, revision conflicts, clock skew, timestamp ordering, byte limits and unique check IDs. These stateful and byte-based rules are not entirely expressible by JSON Schema.

The settings response exposes the configured retention policy and last cleanup result. The database size limit covers SQLite pages; WAL, feed staging and backup files have separate storage considerations described in `docs/adr/003-retention.md` and `docs/operations.md`.

Production ingestion returns HTTP 200 with acknowledgement status `queued` after a durable server inbox commit. This does not indicate completion of inventory/CVE processing. `GET /api/v1/ingest/queue` exposes counts and bounded metadata for reader/editor/admin. Agent credentials cannot read queue status. Inbox persistence failure or capacity limits return HTTP 503; invalid/conflicting input retains its 4xx behavior. See `docs/adr/004-durable-inbox.md`.

`GET attention` accepts `provider` (root host's provider, inherited through `runs_on`; `UNKNOWN` when unset) and `host` (node ID, including the selected host and all descendants). These filters combine with `filter`/`sort` and run before pagination. The UI offers both filters on the node list; the map groups root hosts by provider/location.

`Node.labels.entry_point="true"` is an explicit manual designation of a site entry point, independent of `kind=router`. Multiple nodes in the same provider/site group may carry it. It is not inherited through `runs_on` and is not inferred from routes, VPN peers, or `labels.role`. The existing labels contract carries it through node detail, topology, context and export; no schema migration is needed.
