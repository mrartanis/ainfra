# Приёмка

Состояние: реализация и локальная приёмка завершены; результаты и границы перечислены ниже. Внешнее развёртывание не входит в текущую приёмку. Уточнения владельца имеют приоритет: отдельный UI/API, только proxy TLS, best-effort без spool, ограничение хранения на сервере.

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

Указанные ранее пробелы закрыты: timestamps/idempotency ручных связей, tags документов в форме/preview, labels/links и отдельные наблюдения оплаты. Общий контракт fake/SQLite и шесть коротких fuzz-прогонов проходят; полная матрица пяти дистрибутивов выполнена на amd64 и arm64/QEMU, нативный ARM64 CI job также прошёл. Эксплуатационные требования structured logs, content hashes секций и help/check агента реализованы и покрыты регрессионными тестами; итоговое соответствие этапам приведено ниже. Внешняя установка, живой RouterOS и привилегированные helper units не запускались; пользователь просил пока локальную приёмку.


Дополнение 2026-10-21 20:05 UTC: локальный агент отправил sequence 5, OS/packages/network/disks ok, 9 наблюдений локальных дисков; WG error сохраняется без подмены успехом. Playwright подтвердил labels/links, tags, ручную оплату с calendar clamp и редактирование даты без потери LastPaid/note/timezone. Архивы с backend 2516772 прошли amd64/arm64 smoke с пустым `/usr/share/zoneinfo`, включая Europe/Berlin и restore; исправление aria-label textarea проверено последующим локальным browser smoke и должно войти в окончательную пересборку архивов.


## Итоговый аудит этапов ТЗ (локальный контур)

Проверен код ed2a0ff. Уточнения владельца заменяют исходные пункты про embedded UI, TLS на сервере и spool: UI — отдельный процесс, сервер — HTTP за proxy, доставка — три попытки и discard. Счётчик sequence на диске предотвращает откат наблюдений после перезапуска, он не является гарантией доставки.

| Этап ТЗ | Результат и подтверждение |
|---|---|
| 0: основа/дистрибуция | Четыре бинарника, ручной DI в cmd, static Linux amd64/arm64, macOS arm64 CLI; release-check проверяет ELF и комплектность. Threat model в security.md. |
| 1: домен/persistence/auth | Общая contract suite fake + SQLite; тесты scopes/rotation/revoke, order/replay, partial snapshots, transaction rollback, reopen и migrations. |
| 2: инвентарь/UI | Реальный browser smoke: четыре типа узлов, двусторонние связи, shared document/tags, labels/links, service/backup CRUD, payment/calendar/edit, Markdown preview, поиск и mobile. |
| 3: Linux/APT | Реальный локальный Ubuntu host и десять distro/architecture комбинаций; parser fixtures для hold/pinning/multiarch/source/broken/unknown. Retry/discard проверен отдельным тестом. |
| 4: checks/WG/RouterOS | Typed checks, allowlist/process timeout, last-good, restore evidence и предложения связей проверены. RouterOS — HTTPS protocol tests; живого устройства нет. WG на локальном host сообщает ошибку прав/доступности, успех не выдумывается. |
| 5: CVE | Оба настоящих feed импортированы; offline matching/ranges/release/Pro/unknown tests, comparator против dpkg, reassessment/lifecycle/suppression. Одна фактическая находка дополнительно сверена с Canonical и host dpkg; приватное доказательство в .local/vendor-verification.json. |
| 6: CLI/skill/plans | API-only CLI, JSON/schema/export/context; reader/editor/admin walkthrough из release archive, draft/approval/evidence invalidation/complete/cancel. Модель и её credentials не нужны. |
| 7: эксплуатация | Retention age/count/revisions/size, rollback при заполнении, активный backup/restore, schema upgrade, race/fuzz/security, нагрузка на 2 CPU/2 GiB и install/uninstall. Structured JSON logs и correlation ID проверены без утечки содержимого запросов. |
| 8: приёмка владельца | Выполнена локальная часть: настоящий агент, API/UI/CLI, vendor comparison, документы/планы, backup/restore. Внешняя установка, reboot/timer и privileged helpers намеренно не запускались. |

Секции инвентаря имеют независимые content hashes и last-good timestamps. SQLite хранит текущий JSON-агрегат узла, а не отдельную историческую таблицу package snapshots: неизменные полные списки не накапливаются. Пересчёт findings синхронный в атомарной транзакции; отдельная очередь pending jobs не нужна. Эти решения сохраняют проверяемость состояния при рестарте и ограничивают хранение.

Комплект передачи: README, operations/security/data-model/provider-contract/acceptance/progress, ADR, OpenAPI/JSON Schemas, fixtures с provenance, skills/infra-review, systemd/config/examples, install/uninstall и dist archives с SHA256SUMS/licenses. Исходный план сохранён без переписывания его требований задним числом.

Не проверено live: RouterOS; установка systemd/timer и reboot; opt-in root APT refresh и WireGuard helper; runtime macOS CLI; внешний TLS proxy/сервер владельца. QEMU не выдаётся за native ARM64: локальная полная матрица эмулирована, отдельный native ARM64 CI job также прошёл.

## Доступ к локальному экземпляру

UI: http://127.0.0.1:18081; API: http://127.0.0.1:18080. Admin token хранится в .local/admin.token (0600), вводится в login, не публикуется. Реальный узел: local-ubuntu, UUID c41259fb-7128-4e51-bdba-a4d805687858; browser-* — явно помеченные тестовые записи. Server запущен с --feeds=false для воспроизводимой локальной проверки и использует ранее импортированный кэш; автоматический refresh включается обычным запуском без этого флага.

Последний реальный отчёт: sequence 6, OS/packages/network/disks ok; 2863 пакета. Content hashes четырёх успешных секций имеют 64 hex символа, JSON request logs проверены на работающем API. Retention API подтвердил 90 дней / 100000 / 20 / 1024 MiB, файл БД 676331520 байт на момент проверки. Browser smoke повторно прошёл на этом коде.

GitHub CI для ed2a0ff завершён успешно: [run 39669318162](https://github.com/mrartanis/ainfra/actions/runs/39669318162), test (включая govulncheck/fuzz), race, release-build и native ARM64 collectors.

Финальная проверка ed2a0ff: `make test lint fmt-check`, полная `make test-race`, `make release release-check` прошли. Свежие архивы прошли оба `scripts/release-smoke.py --arch amd64/arm64`: non-root/offline install, настоящий агент, restart, активный backup/restore, auth, role-scoped skill walkthrough, поиск/export и uninstall с сохранением state. Linux ARM64 archive smoke локально использует QEMU. Архивы находятся в dist/, SHA256SUMS проверены.
