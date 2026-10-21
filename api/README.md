# API contracts

`openapi.json` describes the HTTP API; `models.schema.json` contains wire models and `report.schema.json` is the ingestion envelope with typed OS, package and check payloads. Generate all three with `go run ./tools/schema-gen` (use the pinned Go toolchain from Makefile).

OS/package observation fields may be omitted when unknown. Backup/payment checks have different payloads selected by `kind`; unavailable previous success/restore results are omitted or null, never fabricated. Network, WireGuard and disk observations preserve collector-specific structures; their runtime validation also rejects secret field names and excessive nesting/size. The server additionally enforces authorization, revision conflicts, clock skew, timestamp ordering, byte limits and unique check IDs. These stateful and byte-based rules are not entirely expressible by JSON Schema.

The settings response exposes the configured retention policy and last cleanup result. The database size limit covers SQLite pages; WAL, feed staging and backup files have separate storage considerations described in `docs/adr/003-retention.md` and `docs/operations.md`.
