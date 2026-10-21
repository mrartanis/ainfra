# Прогресс

Начало: пустой репозиторий, изучено ТЗ. Подготовлен план и ADR. Разработка идёт локально; исходные файлы владельца сохранены.

## Core / persistence

TDD: сначала failing tests ingestion, SQLite, documents/auth, relations/payment, Debian versions и feed reassessment, затем реализация. `go test ./internal/...` проходит. Есть stateful fake с rollback и SQLite adapter, FTS5, backup. DI: repository/clock/IDs через constructor; HTTP/SQL вне core. Синхронная оценка входит в ingestion/feed transaction.

Toolchain: Go 1.27.3, проверен SHA256 официального архива; modernc.org/sqlite v1.61.0 закреплён go.sum. UI будет четвёртым независимым binary согласно уточнению владельца.
