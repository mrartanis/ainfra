# План реализации

Источник требований: ../infra-codex-plan.md. Уточнения владельца: сервер только HTTP за proxy; UI отдельный компонент через API; локальная приёмка прежде внешнего развёртывания; TDD, DI, commit/push.

1. Toolchain, SQLite/FTS/static spike, контракт ingestion и stateful fake.
2. Атомарное хранение, авторизация, ручной инвентарь, документы/ревизии/поиск.
3. Linux collector, APT, spool/retry, checks, безопасный WG, RouterOS probe.
4. Vendor feeds, Debian comparator, оценка/свежесть/coverage.
5. API/CLI/context/export и отдельный UI, workflow планов.
6. Backup/restore, hardening, systemd/release, локальный end-to-end.

Каждый этап: failing behavioral tests → implementation → проверки → commit/push. Живой RouterOS и машины владельца не подменяются fixtures.
