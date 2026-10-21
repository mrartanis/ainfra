# Эксплуатация

## Установка

Распакуйте архив целевой платформы, проверьте SHA256SUMS. Скопируйте нужные бинарники из `bin` в `/usr/local/bin`. Создайте системного пользователя `infra` без shell и `/var/lib/infra` с владельцем infra и правами 0700. Server не должен работать от root.

Установите `packaging/systemd/infra-server.service` и `infra-ui.service`, перечитайте units. Перед запуском создайте человеческий admin token локальной командой под пользователем infra; output храните вне общедоступных каталогов. Сервис UI отдельный, слушает loopback. Подключите оба процесса к существующему TLS proxy по примеру `packaging/config/nginx.example.conf`. Приложения не используют forwarded headers для доверия/авторизации.

Для агента создайте пользователя infra-agent, каталог `/etc/infra` и token file, читаемый только infra-agent. Конфигурация с абсолютными путями; state directory предоставляет systemd. Установите agent service/timer и включите timer. Проверка: `systemctl list-timers infra-agent.timer`, `journalctl -u infra-agent.service`. Зависимости агента: dpkg-query, apt-cache, apt-mark; network routes используют ip. WireGuard требует wg и соответствующих прав; общий root агенту не нужен.

APT refresh timer — opt-in и не включается установкой агента. Он запускает только `apt-get update`, не upgrades, не удаляет locks. Пока возраст metadata не подтверждён надёжным источником, в инвентаре остаётся unknown.

## CLI, fresh data, CVE

Создайте config по `packaging/config/cli.example.json`, назначьте `INFRA_CONFIG` либо `--config`. Для LLM выдавайте reader, при необходимости editor.

```sh
infractl attention --json
infractl feeds status --json
infractl nodes show NODE_UUID --json
infractl packages list NODE_UUID --upgradable --json
infractl findings list --node NODE_UUID --json
infractl context NODE_UUID --format markdown
```

Проверяйте `observed_at` полного снимка, а не только `received_at`. Успешная отправка не гарантирует успех каждого collector. APT candidate, наличие исправления и возможность обновления — разные сведения.

## Хранение

Флаги server: `--retention-days 90`, `--max-history 100000`, `--document-revisions 20`, `--max-db-mib 1024`. Очистка при старте и раз в час, вручную `infra-server prune --db PATH` с той же политикой. Через API `/api/v1/settings` видны политика и последний результат. Подробности и ограничения WAL: [ADR 003](adr/003-retention.md).

После очистки файл SQLite может не уменьшиться: освобождённые страницы переиспользуются. Лимит относится к файлу БД, не к суммарному месту для WAL, временного feed staging и backups. Оставьте запас диска. Не удаляйте вручную WAL/SHM у работающей базы.

## Backup и restore

```sh
infra-server backup --db /var/lib/infra/infra.db --output /private/backups/infra-2026-10-21.db
```

Используется SQLite `VACUUM INTO`, а не cp открытой WAL-базы. Output должен отсутствовать, права 0600. Отдельно сохраните конфигурации и настройки proxy. Копия включает token digests и должна считаться секретной.

Для проверки восстановления запустите отдельный сервер с `--db /private/backups/infra-2026-10-21.db --listen 127.0.0.1:18082 --feeds=false`. Проверьте документы/ревизии, relations, поиск, доступ токенов и context. Для production восстановления остановите основной процесс, сохраните старую базу и замените её согласованной копией. Если backup мог быть раскрыт, отзовите/перевыдайте токены; возврат backup также возвращает состояние отзыва на момент копии.

## Обновление и удаление

Сначала backup, затем остановка соответствующего service, атомарная замена бинарника, запуск и проверка health/API/agent. Старый binary не гарантирует обратимость SQL migration; для отката может понадобиться согласованная копия БД.

Для удаления отключите timers/services и удалите units/бинарники. State и backups удаляются отдельно только по решению владельца, не автоматически uninstall-скриптом.

## RouterOS

Probe target задаётся только в локальном agent config: node_id, https url, credential_file, ca_file, token_file. Credential file 0600: JSON с username/password выделенного read-only пользователя. Режим `infra-agent probe --config ...` использует GET и ограниченные .proplist. HTTP, insecure TLS и redirects запрещены. Живая проверка устройства ещё не выполнена; см. acceptance.

### Результат opt-in APT refresh

`infra-apt-refresh.service` запускает отдельную команду `infra-agent refresh-apt`, без agent config или токена. Она выполняет только `apt-get update`, с `APT::Update::Error-Mode=any`, ожиданием APT lock и общим timeout 14 минут. Собственный flock исключает одновременные запуски; lock-файлы APT не удаляются.

Результат атомарно записывается в `/var/lib/infra-apt-refresh/status.json` (0644, содержит только время и статус). `running` после аварийного завершения не считается успехом. Ошибка обновляет last_attempt/status, сохраняя last_success предыдущего успешного запуска. Обычный агент только читает результат, API/UI показывают его отдельно от неизвестного возраста всей совокупности APT metadata. Произвольный `mtime` не выдаётся за успешный refresh. Unit/timer по умолчанию не включены.

### Локальные проверки командами

Предпочтительны атомарно записываемые JSON `check_files`. Альтернатива в локальном agent.json:

```json
"check_commands": [
  {"argv": ["/usr/local/libexec/infra-backup-status", "daily"], "timeout_seconds": 10, "output_limit": 65536}
]
```

Каждая команда выдаёт один typed check JSON той же схемы, что check file. Абсолютный executable и все аргументы задаёт оператор; API и документы не могут добавлять команды. Это доверенный локальный скрипт, не sandbox. Не указывайте секреты в argv. Максимум 64 file/command checks, timeout 1–60 секунд, stdout до 1 MiB на команду, суммарный принятый JSON до 8 MiB. Stderr и ошибочный stdout не отправляются. Timeout убивает process group; успешные соседние checks остаются в partial report. Дубликат check_id даёт partial вместо повторного применения. Probe не запускает локальные checks для RouterOS.

### Закрытие плана

`infractl docs complete ID --if-revision N` отмечает одобренную, актуальную ревизию выполненной; `infractl docs cancel ID --if-revision N` отменяет план. Обе операции доступны только admin. Это ручной статус документа, не подтверждение устранения findings и не запуск команд. Изменившиеся основания требуют подготовки/одобрения новой ревизии. UI предоставляет те же действия.

### Примеры backup/payment checks

`packaging/examples/restic-backup-hook.sh OUTPUT_JSON CHECK_ID /path/restic backup ...` оборачивает **настоящий** backup job. Вызов `snapshots` отвергается. Только exit 0 записывает last_success; ошибка/exit 3 записывают failed attempt без выдуманного успеха. Restore test wrapper не выставляет. Передавайте restic credentials обычным защищённым механизмом restic, не через check JSON.

Добавьте OUTPUT_JSON в локальный `check_files` агента. Каталог должен существовать; например, владелец backup job, группа infra-agent, mode 2750; файл создаётся атомарно с 0640. Согласуйте права с фактическими пользователями. Без доступного результата агент покажет partial; успешная копия на сервере не стирается ошибкой следующей попытки. Для уже существующего typed JSON достаточно добавить его абсолютный путь в `check_files`, wrapper не нужен.

`packaging/examples/payment-check.example.json` — явно маркированный образец результата локальной invoice-проверки. Замените дату наблюдения и значения данными своего локального учёта; никакой provider integration или автоматической оплаты нет. Наблюдение не переписывает вручную заданную дату оплаты.
