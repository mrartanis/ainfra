# Приёмка

Состояние: разработка продолжается, весь DoD ещё не закрыт. Уточнения владельца имеют приоритет: отдельный UI/API, только proxy TLS, best-effort без spool, ограничение хранения на сервере.

| Область | Проверенная часть | Доказательство / оставшееся |
|---|---|---|
| Ingestion | scopes, duplicate/conflict, sequence, partial/complete, manual ownership | `internal/app/ingest_test.go`; расширенные payload/clock/secret cases ещё проверяются |
| Persistence | rollback, FTS5/literal, backup, future-schema rejection, upgrade indexing | настоящая SQLite в `internal/adapters/sqlite/*_test.go` |
| История/retention | age/count caps, latest+approved document revisions, сохранение open findings | `retention_test.go`: возраст, cap свежих events, сохранение approved, rollback при превышении 64 MiB; app test на неизменность resolved_at при heartbeat. Размер БД ограничен max_page_count, WAL/backup требуют запаса диска |
| Документы/планы | revision conflicts, editor denial, admin approval, edit invalidation | `documents_test.go`, e2e; дополнительные evidence revalidation cases в аудите |
| Узлы/связи/оплата | identity, runs_on cycle, calendar month clamp | app tests; полная ручная UX-приёмка ещё не завершена |
| Агент локально | настоящий Ubuntu 22.04 report, 2863 пакета | `.local` smoke; WG error не скрыт |
| Дистрибутивы | Debian 12/13, Ubuntu 22.04/24.04/26.04 collectors non-root/offline | [runtime matrix](runtime-matrix.md), pinned Docker images |
| RouterOS | HTTPS GET/proplist/auth protocol tests | live device not run — устройство/доступ не предоставлены |
| CVE feeds | оба настоящих источника; source comparator, ambiguity, outage retention | offline fixtures/tests и [live import benchmark](feed-import-benchmark.md); полный semantic audit продолжается |
| UI | отдельный процесс, API-only, login, create document, escaping, mobile widths | `scripts/browser-smoke.py`, `.local/screenshots`; остальные формы ещё проверяются |
| CLI/context/export | реальные API commands; чистый JSON | CLI tests, local smoke; полный walkthrough skill ещё не закончен |
| Backup restore | agent→API→finding→draft→approval→backup→restore→search/auth | `make test-e2e` passed |
| Static builds | Linux amd64/arm64 server/agent/UI/CLI; macOS arm64 CLI | первая `make release release-check` passed; финальная пересборка после всех правок требуется |
| Race/dependencies | race suite passed; govulncheck v1.9.0 no vulnerabilities на проверенном наборе | повтор security check после добавления schema validator запланирован |
| Schemas | OpenAPI 3.1, generated wire models/report schema, schema validation tests | `api/`, `tools/schema-gen`, `internal/contracts`; transport constraints будут сверены с финальным API |
| CI | pinned actions, test/race/release jobs, collector matrix | workflow committed; результат GitHub job нужно проверить отдельно |
| Нагрузка | 100 nodes × 5000 packages / 10000 docs; 2 CPU / 2 GiB, p95 nodes 2.66 ms, attention 3.89 ms, search 24.12 ms | [benchmark](load-benchmark.md); повтор после финальной пагинации и concurrent feed import ещё не выполнен |
| Live deployment | not run | владелец просил пока локальную приёмку; внешние адреса/credentials не предоставлены |

Команды, реально прошедшие к текущему этапу: `make test`, `make lint`, `make fmt-check`, `make test-e2e`, `make test-race`, `make release`, `make release-check`, `python3 scripts/collector-matrix.py`, browser smoke из локального venv. Это фиксация этапов; после последующих изменений затронутые проверки выполняются повторно.
