# Нагрузочная проверка локального API

2026-10-21. Контейнер Linux amd64 с Docker `--cpus 2 --memory 2g`, non-root UID, без сети/capabilities. Go 1.27.3, настоящая SQLite. База предварительно восстановлена из локального backup реальных Debian/Canonical feeds.

`tools/load-test` создаёт через use cases 100 узлов, 5000 пакетов на каждом и 10000 документов. Для трёх sources на узел (openssl/curl/bash) используются реальные vendor advisories; остальные package/source names синтетические и не маскируются под покрытые. Это тест масштаба инвентаря и скорости чтения, не модель 5000 уязвимых sources на каждом узле.

После 5 прогревочных запросов измерены 50 последовательных HTTP-запросов каждого типа к настоящему HTTP handler. p95 — 47-я упорядоченная выборка из 50; нагрузка не имитирует сотни параллельных пользователей.

| Запрос | p50, ms | p95, ms | Размер ответа |
|---|---:|---:|---:|
| nodes | 2.051 | 2.660 | 102127 B |
| attention | 2.585 | 3.891 | 36789 B |
| search?q=Backup | 19.409 | 24.115 | 23050 B |

Seed: 83.27 s. `/proc/self/status`: VmHWM 126588 KiB (~124 MiB), конечный VmRSS 75552 KiB. `--cpus` задаёт CFS quota, а не CPU affinity; Cpus_allowed_list может перечислять все CPU хоста. Raw report: `.local/load-results.json`, база `.local/load-test.db`.

Список и attention читают materialized summaries. Полные package snapshots остаются для отдельного detail/export/assessment. Индекс findings по node_id исключает пересканирование чужих узлов. Сводка учитывает срок suppression на чтении; истечение не требует heartbeat.

Этот замер сделан после введения summaries, до финальной проверки pagination/search projection. После завершения API требуется повторить чтения на том же наборе. Одновременный большой feed import пока не измерен в этом прогоне.

## Повтор после pagination/search projection

Тот же набор данных, `--reuse`, 2 CPU / 2 GiB, 50 чтений после 5 прогревочных. Seed не повторялся. p50/p95 ms: nodes **3.160/3.632**, attention **2.898/3.209**, search **47.641/56.797**. Ответы соответственно 102127 / 41689 / 47802 B. Peak RSS процесса только чтений 22112 KiB (~22 MiB); это не peak ingestion/import. Search теперь возвращает count и bounded snippets, поэтому запрос делает дополнительный count и имеет другую форму ответа. Все p95 остаются меньше 100 ms. Raw report `.local/load-results-final.json`. Прогон последовательный, конкурентный большой feed import ещё не измерен.

## Чтение во время публикации реального feed

2026-10-21, та же база/лимиты 2 CPU / 2 GiB. `--reuse --publish-feed ubuntu` повторно публикует 1 513 587 уже скачанных Ubuntu advisory записей через production `SyncSnapshot`, затем пересчитывает 100 узлов. Это измерение публикации и переоценки, без сети и распаковки архива. Публикация заняла **61,70 s**. Все 55 запросов каждого типа (5 прогревочных + 50 измеряемых) целиком прошли во время публикации; ошибок HTTP не было.

| Запрос | p50, ms | p95, ms |
|---|---:|---:|
| nodes | 3.855 | 4.894 |
| attention | 3.885 | 4.945 |
| search?q=Backup | 57.936 | 67.288 |

VmHWM всего процесса с публикацией: **536376 KiB (~524 MiB)**, конечный VmRSS 365852 KiB. Raw `.local/concurrent-load.json`. Вторая, read-only SQLite pool читает предыдущий committed snapshot, пока writer публикует новый. Это не стресс-тест сотен пользователей и не сетевой импорт.
