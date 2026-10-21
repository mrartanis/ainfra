# Приёмка

Состояние: разработка продолжается, весь DoD ещё не закрыт. Уточнения владельца имеют приоритет: отдельный UI/API, только proxy TLS, best-effort без spool, ограничение хранения на сервере.

| Область | Проверенная часть | Доказательство / оставшееся |
|---|---|---|
| Ingestion | scopes, duplicate/conflict, sequence, partial/complete, manual ownership | `internal/app/ingest_test.go`; добавлены nested secret/unknown OS-package fields, depth/string limits; clock skew tests входят в suite |
| Persistence | rollback, FTS5/literal, backup, future-schema rejection, upgrade indexing | настоящая SQLite в `internal/adapters/sqlite/*_test.go` |
| История/retention | age/count caps, latest+approved document revisions, сохранение open findings | `retention_test.go`: возраст, cap свежих events, сохранение approved, rollback при превышении 64 MiB; app test на неизменность resolved_at при heartbeat. Размер БД ограничен max_page_count, WAL/backup требуют запаса диска |
| Документы/планы | revision conflicts, editor denial, admin approval, edit invalidation | `documents_test.go`, e2e; проверены ошибки чтения evidence, refresh-only heartbeat, реальные version changes, completed/cancelled и права переходов |
| Узлы/связи/оплата | identity, runs_on cycle, calendar month clamp, предложения связей | app tests; предложения только по свежим данным и подтверждённым WG keys, без автоматической записи графа; Playwright: VM/host/router/backup, edit/reload, связи в обе стороны |
| Агент локально | настоящий Ubuntu 22.04 report, 2863 пакета | `.local` smoke; WG error не скрыт |
| Дистрибутивы | Debian 12/13, Ubuntu 22.04/24.04/26.04 collectors non-root/offline | [runtime matrix](runtime-matrix.md), pinned Docker images |
| RouterOS | HTTPS GET/proplist/auth protocol tests | live device not run — устройство/доступ не предоставлены |
| CVE feeds | оба настоящих источника; source comparator, ambiguity, outage retention | offline fixtures/tests и [live import benchmark](feed-import-benchmark.md); полный semantic audit продолжается |
| UI | отдельный процесс, API-only, login, create document, escaping, mobile widths | `scripts/browser-smoke.py`, `.local/screenshots`; расширенный smoke: четыре типа узлов, общая инструкция, связи, edit/reload/search и preview до сохранения |
| CLI/context/export | реальные API commands; чистый JSON | CLI tests, local smoke; role-scoped walkthrough reader/editor/admin пройден настоящими binaries в archive smoke на amd64 и arm64/QEMU |
| Backup restore | agent→API→finding→draft→approval→backup→restore→search/auth | `make test-e2e` passed |
| Static builds | Linux amd64/arm64 server/agent/UI/CLI; macOS arm64 CLI | первая `make release release-check` passed; archive install/restart/restore/uninstall smoke прошёл на amd64 и arm64/QEMU; последняя пересборка после WG/read-pool изменений ещё нужна |
| Race/dependencies | race suite passed; govulncheck v1.9.0 no vulnerabilities на проверенном наборе | повтор govulncheck после schema validator и goldmark: no vulnerabilities; полная race suite и затронутые пакеты после последующих изменений passed |
| Schemas | OpenAPI 3.1, generated wire models/report schema, schema validation tests | `api/`, `tools/schema-gen`, `internal/contracts`; transport constraints будут сверены с финальным API |
| CI | pinned actions, test/race/release jobs, collector matrix | workflow committed; GitHub checks success для 47b5c66, 2ec225e, 6c1ad72; также success для 8280a3b, 509c598, e6154b8; последний commit проверяется отдельно |
| Нагрузка | 100 nodes × 5000 packages / 10000 docs; 2 CPU / 2 GiB, p95 nodes 2.66 ms, attention 3.89 ms, search 24.12 ms | [benchmark](load-benchmark.md); повтор после финальной пагинации: p95 nodes 3.63 / attention 3.21 / search 56.80 ms; concurrent feed import ещё не выполнен |
| Live deployment | not run | владелец просил пока локальную приёмку; внешние адреса/credentials не предоставлены |

Команды, реально прошедшие к текущему этапу: `make test`, `make lint`, `make fmt-check`, `make test-e2e`, `make test-race`, `make release`, `make release-check`, `python3 scripts/collector-matrix.py`, browser smoke из локального venv. Это фиксация этапов; после последующих изменений затронутые проверки выполняются повторно.
