# Модель данных

SQLite `records(kind,id,body)` хранит типизированные JSON-агрегаты с composite primary key. Application Repository/Tx — port с атомарным commit/rollback; production adapter SQL и stateful fake используют одинаковые use cases. FTS5 индексирует узлы/документы триггерами. SQL migrations встроены в binary.

Node UUID постоянен. Ручные поля и observed sections разделены; agent не редактирует purpose, criticality, services, payments и relations. Sequence применяется в пределах явно назначенного instance. Повтор report_id с тем же hash идемпотентен, конфликтующий content отвергается. Старый sequence не изменяет snapshot.

Полный успешный collector заменяет только собственную секцию; partial/error не удаляют last-good data. Отдельно сохраняется latest collector status. Документы используют revision и If-Match, прежние версии хранятся отдельно. Approved plan связан с body hash и evidence hash; новый package content/связанные инструкции требуют revalidation, обычный heartbeat — нет.

Findings: assessment/remediation/workflow раздельны; inventory_at/feed_at независимы. Естественный ключ: node/source/advisory/channel. Несколько binary packages объединяются. Suppression требует admin, причину и срок. Изменения assessment отражаются в events.

Feeds группируются по source package, заменяются атомарно. Ошибочный импорт не заменяет активный snapshot. Дисковый staging приватен и удаляется после завершения. Сроки истории и размер БД ограничены политикой retention.
