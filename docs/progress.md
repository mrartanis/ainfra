# Прогресс

Начало: пустой репозиторий, изучено ТЗ. Подготовлен план и ADR. Разработка идёт локально; исходные файлы владельца сохранены.

## Core / persistence

TDD: сначала failing tests ingestion, SQLite, documents/auth, relations/payment, Debian versions и feed reassessment, затем реализация. `go test ./internal/...` проходит. Есть stateful fake с rollback и SQLite adapter, FTS5, backup. DI: repository/clock/IDs через constructor; HTTP/SQL вне core. Синхронная оценка входит в ingestion/feed transaction.

Toolchain: Go 1.27.3, проверен SHA256 официального архива; modernc.org/sqlite v1.61.0 закреплён go.sum. UI будет четвёртым независимым binary согласно уточнению владельца.

## API, agent, UI и уточнения владельца

Локальный настоящий agent report Ubuntu 22.04: 2863 пакета, OS/network ok, WG error отражён отдельно. API проверен write-only/лимитами; UI browser smoke прошёл login, document create, escaping, 1440×900/390×844. Скриншоты в игнорируемой `.local/screenshots`.

По уточнению владельца spool удалён: 3 попытки и discard, только persisted sequence. Retention реализован: сроки/caps, latest + approved revisions, SQLite size limit; TDD на реальной SQLite. Отдельный UI не импортирует SQL и обращается через apiclient. CLI и export реализованы.

Live Debian/Ubuntu sync успешно выполнен, но первый замер ~2 GiB RSS. Исправление: streaming FetchTo + приватный disk staging + publication per source; повторный замер выполняется. USN upstream нормализован к CVE, USN-only не даёт уверенного verdict. Добавлены typed backup checks, reenrollment с отзывом старых токенов, более явное coverage/attention.

Поставляемый skill проверен quick_validate; CLI reference получен из фактического --help. Подготовлены units/configs/release scripts и эксплуатационные документы. Следующие обязательные работы: закончить полный audit ТЗ (validation, pagination, migration safeguards, coverage semantics), schemas/OpenAPI, CI/runtime matrix, release/dependency notices, race/security/load и финальная локальная приёмка. Нельзя объявлять весь релиз готовым на основании первых smoke-тестов.

Повторный live feed import: exit 0, 2:29.60 wall, peak RSS 338044 KiB вместо 2065284 KiB; результат в docs/feed-import-benchmark.md. `make test-e2e` проходит agent/API/finding/plan/admin approval/backup restore/search/auth. `make lint` и fmt-check проходят. Race suite запущен отдельно.

`make test-race` прошёл; govulncheck v1.9.0: No vulnerabilities found. Первая сборка release archives Linux amd64/arm64 и macOS arm64 CLI завершена; SHA256/static ELF проверяются release-check. Это промежуточные артефакты: финальная пересборка после оставшихся исправлений обязательна. Миграции теперь сериализованы BEGIN IMMEDIATE, future schema отвергается, существующие документы индексируются при upgrade. Добавлен local disk collector, исключающий сетевые/FUSE mounts; поведение проверено тестом.

Runtime collector matrix прошла на пяти pinned образах Debian 12/13, Ubuntu 22.04/24.04/26.04, non-root/offline/read-only; binary/source inventory совпал с независимым dpkg-query. Результат docs/runtime-matrix.md. Добавлен CI с pinned actions, отдельной race job и release archives. `/readyz` проверяет доступность storage, `/healthz` — процесс; TDD воспроизводит недоступную БД.

OpenAPI 3.1 и JSON Schemas созданы в api/, воспроизводимый generator tools/schema-gen. JSON Schema validator v6.0.2 используется тестами; generation не добавляет runtime binary. Проверки wire models/report schema проходят. Следующий приоритет: pagination и компактные indexed summaries для list/attention, нагрузочная проверка 100×5000/10000 docs; затем оставшиеся security/coverage contracts и конечная приёмка. docs/acceptance.md обновлён по фактическим доказательствам, без объявления завершения.

Performance milestone: компактные node-summary/finding-summary, schema 002 и node_id index; migration v1→v2 покрыта тестом. Тест 5000 packages доказывает компактность list и сохранность полного inventory. Suppression expiry обновляет attention без отчёта.

Нагрузка 100×5000 / 10000 docs на 2 CPU / 2 GiB прошла: p95 nodes 2.66 ms, attention 3.89 ms, search 24.12 ms, peak RSS ~124 MiB. Детали/ограничения docs/load-benchmark.md. Введены bounded API pages, headers, UI navigation, server-side filters и потоковый постраничный CLI export. Search выдаёт bounded snippets вместо полных документов/пакетных снимков. Полный /export JSON endpoint удалён до релиза: CLI обходит обычные versioned API resources, manifest отмечает per-record consistency.

Retention audit: найден и исправлен sliding expiry закрытых findings. TDD: heartbeat/fixed/source removed не продлевают resolved_at, reopen очищает его. Реальная SQLite проверена на cap свежих events и rollback oversized transaction при лимите 64 MiB; предыдущие данные сохранены. JSON schemas обновлены.

Correctness/UI audit: partial typed checks now apply valid per-job results without replacing the complete checks snapshot; mixed/unknown APT suite gives unknown CVE assessment. Both regressions were reproduced before fixing. Markdown preview now formats CommonMark with raw HTML/unsafe URLs disabled, including preview before saving; tests verify no API write and preserved revision. Browser smoke on the preceding pagination build passed again.

APT refresh: opt-in root unit now records running/success/error, last attempt and last successful completion atomically; collector reads status without invoking updates, UI shows it separately from unknown metadata age. Fake runner tests verify success→failure preserves last success, read-only collection and update-only argv. Actual privileged apt update on the workstation was not run.

Local command checks implemented with injected runner, operator-only absolute argv allowlist, per-command timeout/output bound, process-group cancellation and typed schema. Tests reproduced and fixed a promoted bytes.Buffer.ReadFrom bypass of the output limiter in command runners. Failure/timeout retains sibling check evidence. Evidence storage errors now abort draft/approval/read instead of hashing an incomplete set. Dependency license texts added to release archives; final paginated benchmark and govulncheck passed.

Release smoke passed on a clean pinned Debian 13 container, non-root/read-only/offline, with no Go/Python/Node/Docker inside: archive install, actual report, restart+sequence persistence, active backup, separate restored API, auth/doc/search/export, uninstall preserves state. Expanded Playwright smoke passed four node kinds, edit/reload, bidirectional relations, shared runbook, unsaved Markdown preview, search and mobile widths. Fresh local workstation report sequence 2: 2863 packages; OS/network/packages ok, WG error visible. Fuzz: Debian comparison 541202 executions and dpkg parser 435079 in 10 seconds each, passed. Runtime ingestion now rejects unknown OS/package fields, common secret field names, excessive nesting/string lengths and oversized total sections; regression tests first reproduced acceptance of unsafe data.
