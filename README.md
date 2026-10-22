# Infra

Личный инвентарь для человека и внешнего LLM-администратора. Работа не требует LLM, SaaS или CDN. Go, SQLite FTS5, HTTP API.

Infra собирает и выдаёт сведения об инфраструктуре; агенты не администрируют узлы и не получают удалённые команды. Внешний LLM-администратор запускается отдельно, а его SSH/API credentials для изменений хранятся на машине его запуска. Эти credentials не передаются в Infra. Доступы сборщика для чтения устройств и токены отправки observations — отдельные секреты, доступные только соответствующему сборщику; сервер инвентаря их не хранит. Подробности — в [границах безопасности](docs/security.md).

Четыре независимых бинарника:

- `infra-server`: HTTP API, SQLite, vendor feeds.
- `infra-agent`: однократный read-only сбор Debian/Ubuntu; `probe` для RouterOS 7 HTTPS REST и Keenetic/Netcraze HTTPS RCI.
- `infractl`: CLI к API, документы, поиск, context и экспорт.
- `infra-ui`: отдельный HTTP UI; работает только через API, без доступа к БД.

## Локальный запуск

Нужен Go 1.27.3 для сборки.

```sh
make build
mkdir -p .local
bin/infra-server bootstrap --db .local/infra.db --output .local/admin.token
bin/infra-server serve --db .local/infra.db --listen 127.0.0.1:18080
# В другом терминале:
bin/infra-ui --listen 127.0.0.1:18081 --api http://127.0.0.1:18080 --dev-loopback
```

Откройте http://127.0.0.1:18081 и введите токен из `.local/admin.token`. Секрет не передаётся в argv. `--dev-loopback` разрешён только для локального HTTP. В production используются Secure cookies и reverse proxy.

По умолчанию server загружает официальные Debian/Canonical feeds, затем обновляет их раз в час. Для offline-тестирования можно указать `--feeds=false`: отсутствие покрытия видно в UI. Синхронизация Canonical обрабатывает большой распакованный поток; импорт не нужно считать мгновенным.

## Подключение агента

Создайте узел в UI или локальной командой `infra-server create-node --name host`. UUID узла не меняется при смене имени/IP. Выдайте agent token для этого UUID и нового UUID экземпляра через UI или `infra-server bootstrap --role agent --node UUID --instance INSTANCE_UUID --output /private/path/agent.token`.

Заполните [пример конфигурации](packaging/config/agent.example.json), укажите абсолютные пути и права 0600 на token file:

```sh
bin/infra-agent check-config --config /absolute/path/agent.json
bin/infra-agent collect --config /absolute/path/agent.json --json
bin/infra-agent run --config /absolute/path/agent.json
```

Для 32-битного Raspberry Pi (`uname -m` → `armv7l`) используйте `make build-agent-armv7`: бинарник `bin/infra-agent-linux-armv7`. ARM64 — другая архитектура; для неё нужен GOARCH=arm64.

Один запуск, три попытки доставки. Не доставленный отчёт отбрасывается; следующий запуск собирает новый. Агент хранит счётчик порядка, а не очередь. Сервер отвечает 200 после сохранения отчёта в отдельный SQLite inbox; инвентарь и CVE обновляет фоновый worker. Состояние: `infractl request GET ingest/queue --json`. Агент не устанавливает пакеты и не принимает команды.

## Проверки и состояние готовности

```sh
make test lint test-integration test-e2e test-security
make test-race test-fuzz
make build release release-check
```

[Локальная приёмка](docs/acceptance.md) содержит проверенные сценарии и ограничения: живой RouterOS, внешний deployment и привилегированные helper units не запускались. Готовые архивы создаются в `dist/`, установка и восстановление описаны в [эксплуатации](docs/operations.md).

[Эксплуатация](docs/operations.md), [безопасность](docs/security.md), [модель](docs/data-model.md), [контракт providers](docs/provider-contract.md), [ход работ](docs/progress.md).

Подключение роутеров: [инвентарь Keenetic с pi](docs/keenetic-inventory.md), [skill добавления Keenetic](skills/infra-add-keenetic/SKILL.md). [Входные точки на карте и площадки](docs/site-gateway.md) описывают текущие отметки и проект отдельной модели Site.

## CLI для внешнего администратора

`infractl` можно собрать локально на macOS/Linux: `make build-cli`, бинарник `bin/infractl`. Если исходников нет — `git clone https://github.com/mrartanis/ainfra.git`. Настройка токена и endpoint: [подготовка CLI](skills/infra-review/references/setup.md).

`nodes list` по умолчанию выдаёт краткий список, `nodes show ID` — summary без массивов пакетов и runtime. Для services/coverage списка используй `nodes list --view summary`; полный инвентарь запрашивается явно через `--full`. Прямой API использует `view=brief|summary|full` с теми же defaults.

Журнал обслуживания дописывается отдельными записями: `docs append ID --file -`, чтение — `docs entries ID --page-info --json`. У записи обязательны стабильный уникальный `id`, `step`, `status` (`info|started|succeeded|failed`); остальные поля: `command`, `result`, `evidence`, `rollback`, `body`. Повтор с тем же ID и содержимым безопасен. Документ и его одобрение не меняются. Сведения о хостах хранятся в сервисе Infra, `.local` предназначен для локальной разработки сервиса.
