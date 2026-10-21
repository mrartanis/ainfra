# Infra

Личный инвентарь для человека и внешнего LLM-администратора. Работа не требует LLM, SaaS или CDN. Go, SQLite FTS5, HTTP API.

Четыре независимых бинарника:

- `infra-server`: HTTP API, SQLite, vendor feeds. TLS всегда завершается внешним proxy.
- `infra-agent`: однократный read-only сбор Debian/Ubuntu; `probe` для RouterOS 7 HTTPS REST.
- `infractl`: CLI к API, документы, поиск, context и экспорт.
- `infra-ui`: отдельный HTTP UI; работает только через API, без доступа к БД.

## Локальный запуск

Нужен Go 1.27.3 для сборки. На машине запуска готовых бинарников Go/Python/Node/Docker не нужны.

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

Один запуск, три попытки доставки. Неудавшийся отчёт отбрасывается; следующий запуск собирает новый. На диске сохраняется счётчик порядка, а не очередь отчётов. Агент не устанавливает пакеты и не принимает команды.

## Проверки и состояние готовности

```sh
make test lint test-integration test-e2e test-security
make test-race
make build release release-check
```

[Приёмка](docs/acceptance.md) отличает выполненные проверки от оставшихся. До закрытия приёмки репозиторий является разрабатываемым продуктом, а не завершённым релизом.

[Эксплуатация](docs/operations.md), [безопасность](docs/security.md), [модель](docs/data-model.md), [контракт providers](docs/provider-contract.md), [ход работ](docs/progress.md).
