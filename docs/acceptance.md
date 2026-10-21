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
| CVE feeds | оба настоящих источника; source comparator, ambiguity, outage retention | offline fixtures/tests и [live import benchmark](feed-import-benchmark.md); проверены ranges, release coverage и обновление старой нормализации; последний live import 2026-10-21 19:38 UTC |
| UI | отдельный процесс, API-only, login, create document, escaping, mobile widths | `scripts/browser-smoke.py`, `.local/screenshots`; расширенный smoke: четыре типа узлов, общая инструкция, связи, edit/reload/search и preview до сохранения |
| CLI/context/export | реальные API commands; чистый JSON | CLI tests, local smoke; role-scoped walkthrough reader/editor/admin пройден настоящими binaries в archive smoke на amd64 и arm64/QEMU |
| Backup restore | agent→API→finding→draft→approval→backup→restore→search/auth | `make test-e2e` passed |
| Static builds | Linux amd64/arm64 server/agent/UI/CLI; macOS arm64 CLI | `make release release-check` повторно passed на 7e76fd9; archive install/restart/restore/uninstall и role-scoped skill smoke повторно passed на amd64 и arm64/QEMU |
| Race/dependencies | race suite passed; govulncheck v1.9.0 no vulnerabilities на проверенном наборе | повтор govulncheck после schema validator и goldmark: no vulnerabilities; полная race suite и затронутые пакеты после последующих изменений passed |
| Schemas | OpenAPI 3.1, generated wire models/report schema, schema validation tests | `api/`, `tools/schema-gen`, `internal/contracts`; типизированы OS/package/check payloads, reenroll/suppression/token/settings и query parameters; stateful/byte-level ограничения описаны отдельно |
| CI | pinned actions, test/race/release jobs, collector matrix | workflow committed; GitHub checks success для 47b5c66, 2ec225e, 6c1ad72; также success для 8280a3b, 509c598, e6154b8; CI success также для 58d5374, f82fd05, 99d4903 и 7e76fd9 |
| Нагрузка | 100 nodes × 5000 packages / 10000 docs; 2 CPU / 2 GiB, p95 nodes 2.66 ms, attention 3.89 ms, search 24.12 ms | [benchmark](load-benchmark.md); повтор после финальной пагинации: p95 nodes 3.63 / attention 3.21 / search 56.80 ms; конкурентная публикация cached Ubuntu feed: p95 4.89/4.95/67.29 ms, peak RSS 524 MiB; сеть/распаковка в этот замер не входят |
| Live deployment | not run | владелец просил пока локальную приёмку; внешние адреса/credentials не предоставлены |

Команды, реально прошедшие к текущему этапу: `make test`, `make lint`, `make fmt-check`, `make test-e2e`, `make test-race`, `make release`, `make release-check`, `python3 scripts/collector-matrix.py`, browser smoke из локального venv. Это фиксация этапов; после последующих изменений затронутые проверки выполняются повторно.

## Последний проверенный запуск

2026-10-21, код 7e76fd9. Полная race suite, govulncheck v1.9.0 (No vulnerabilities found), release checks, archive smoke amd64 + arm64/QEMU, collector matrix и расширенный Playwright smoke прошли повторно. Сервер и отдельный UI перезапущены на 127.0.0.1:18080/18081. Агент: sequence 4, 2863 пакета, OS/packages/network ok; WireGuard error отражён честно. Debian: 271108 advisory, Ubuntu: 1513587; нормализация версии 1 и фактические releases видны в feed-status. Политика retention через API: 90 дней / 100000 events и receipts / 20 ревизий / 1024 MiB. На старте очищается история; открытые findings и актуальный инвентарь сохраняются.

Полный аудит ТЗ ещё не закрыт: отдельно сверить timestamps/idempotency ручных связей, tags документов, представление ручных labels/links и наблюдаемой оплаты; это оставшиеся пункты модели/пользовательских сценариев, а не объявленные будущие функции. Внешняя установка, живой RouterOS и привилегированные helper units не запускались; пользователь просил пока локальную приёмку.
