# Измерение импорта feeds, 2026-10-21

Команда: `/usr/bin/time -v bin/infra-server sync-feeds --db .local/staging-smoke.db`.
Настоящие Debian Security Tracker и Canonical OSV, новая SQLite база, без узлов.

Результат streaming + disk staging: exit 0, wall 2:29.60, user CPU 99.23 s, system CPU 16.81 s, maximum RSS 338044 KiB (~330 MiB). Первый вариант с полным normalized snapshot в памяти: 2065284 KiB (~2 GiB); этот вариант заменён в production composition root.

Замер на локальной Ubuntu 22.04 amd64 машине владельца (64 GiB RAM); это не нагрузочный тест 2-vCPU/2-GiB и не измерение latency API под нагрузкой. Пересчёт 100 узлов в этот результат не входит. Внешний feed может измениться, поэтому время не SLA. Raw time output остаётся в `.local/staging-sync-metrics.log`.

Повтор после исправления OSV intervals: fresh DB `.local/ranges-smoke.db`, оба реальных источника, exit 0, wall 2:18.73, peak RSS 338308 KiB. Полный результат `.local/ranges-feed.log`. Размер итоговой БД ~528 MiB. Повтор не включает нагрузку inventory reassessment; она проверяется отдельно workload-тестами.
