# ADR 001: границы компонентов

Go, стандартный HTTP, modernc SQLite (FTS5), явные SQL migrations. Core зависит от transaction repository, clock, ID generator; adapters внедряются composition roots. Stateful fake воспроизводит rollback. Синхронная атомарная оценка исключает недолговечную очередь jobs.

UI — отдельный Go binary с html/template и серверными формами; только API client, без SQL/core imports. API не обслуживает HTML и не терминирует TLS. Production cookie Secure, TLS на reverse proxy. Для loopback smoke отдельный явный development mode.

Секреты сборщиков — только файлы 0600, на сервере SHA-256 высокоэнтропийных токенов. Agent не получает команды. Bootstrap локальный, внешней регистрации нет.
