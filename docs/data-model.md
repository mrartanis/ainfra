# Модель данных

SQLite `records(kind,id,body)` хранит типизированные JSON-агрегаты с composite primary key. Application Repository/Tx — port с атомарным commit/rollback; production adapter SQL и stateful fake используют одинаковые use cases. FTS5 индексирует узлы/документы триггерами. SQL migrations встроены в binary.

Node UUID постоянен. Ручные поля и observed sections разделены; agent не редактирует purpose, criticality, services, payments и relations. Sequence применяется в пределах явно назначенного instance. Повтор report_id с тем же hash идемпотентен, конфликтующий content отвергается. Старый sequence не изменяет snapshot.

Полный успешный collector заменяет только собственную секцию; partial/error не удаляют last-good data. Отдельно сохраняется latest collector status. Документы используют revision и If-Match, прежние версии хранятся отдельно. Approved plan связан с body hash и evidence hash; новый package content/связанные инструкции требуют revalidation, обычный heartbeat — нет.

Findings: assessment/remediation/workflow раздельны; inventory_at/feed_at независимы. Естественный ключ: node/source/advisory/channel. Несколько binary packages объединяются. Suppression требует admin, причину и срок. Изменения assessment отражаются в events.

Feeds группируются по source package, заменяются атомарно. Ошибочный импорт не заменяет активный snapshot. Дисковый staging приватен и удаляется после завершения. Сроки истории и размер БД ограничены политикой retention.

Ручные связи содержат `created_at`/`updated_at`; повтор той же связи (включая обратную пару tunnel_peer) возвращает существующую запись без нового события. У старых записей без дат остаётся неизвестное время — дата не выдумывается при чтении.

`payment_checks` — отдельная карта наблюдений по check_id, не ручной график `payment`. Она хранит latest observed_at/status и last_success_at/next_due/amount/currency/evidence последнего успеха. Ошибка сохраняет предыдущий успех; устаревший результат не заменяет новый. Agent не может выставить LastPaid. SaveNode сохраняет наблюдения независимо от устаревшей формы. Полный успешный checks snapshot удаляет отсутствующие ID; partial сохраняет их, максимум 1000 ID. Summary исключает текст evidence. Время вложенной проверки ограничено тем же 5-минутным допуском, что отчёт и секция.

Каждая полученная секция получает серверный SHA-256 `content_hash` канонического JSON data (ключи объектов упорядочиваются, целые числа сохраняют точность). Hash не включает observed_at/status/envelope и не принимается на доверии от агента. Секции обновляются независимо; неизменный package snapshot имеет прежний hash при новом времени наблюдения. Хранится только текущий last-good snapshot, история одинаковых списков пакетов не накапливается. В компактном nodes list hash относится к исходному полному снимку, а data может быть проекцией; полный payload доступен в node detail. Старые записи получают hash при следующем отчёте.
