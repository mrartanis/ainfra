infractl — клиент HTTP API Infra
  nodes list [--brief|--view summary|--full] | nodes show ID [--brief|--full] | nodes put --file node.json --if-revision N
  attention | packages list NODE [--upgradable] | findings list [--node ID]
  docs search QUERY [--literal] | docs get ID [--revision N]
  docs put --file document.json [--if-revision N]
  docs append ID --file entry.json | docs entries ID
  docs approve|complete|cancel ID --if-revision N
  relations list | relations suggest [--node ID] | relations put --file relation.json | relations delete ID
  tokens list | tokens issue --file principal.json --output token-file
  tokens revoke ID | feeds status | events | me
  context NODE [--format markdown] | export --output NEW_DIRECTORY
  request METHOD API_PATH [--file JSON] [--if-revision N]
  --limit N --offset N: страница списка; --page-info: данные с метаданными страницы.
  --file -: читать JSON из stdin.
  --json: машинный JSON; --config: JSON с endpoint, token_file, ca_file.
  Узлы: list по умолчанию brief, show — summary; полный инвентарь только --full.
  Формат docs append: JSON {id, step, status, command?, result?, evidence?, rollback?, body?}.
  status: info|started|succeeded|failed; id сохраняйте для безопасного повтора.
  Формат docs put: JSON {id?, title, kind, body, nodes?, tags?}; без id создаёт документ.

Поиск узла: `infractl nodes list --page-info --json` (brief по умолчанию).
Для ревью внешних сервисов: `infractl nodes list --view summary --page-info --json`.
Выбранный узел: `infractl nodes show NODE --json`; полный инвентарь — только `--full`.
`packages list` возвращает объект PackageData с массивом `packages`, не массив верхнего уровня.
Счётчики относятся ко всему снимку до фильтрации/пагинации; заголовок X-Total-Count — к отфильтрованному списку.
`context` возвращает Markdown, а не JSON. Не пытайся декодировать его как JSON.

Пример дописывания журнала (ID фиксируется до первого запроса):
```sh
infractl docs append DOCUMENT_ID --file - <<'JSON'
{"id":"maintenance-20261022-step-01","step":"Проверка конфигурации","status":"succeeded","command":"nginx -t","result":"exit 0","evidence":["2026-10-22T13:00:00Z: configuration test successful"],"rollback":"Откат не требуется"}
JSON
infractl docs entries DOCUMENT_ID --page-info --json
```

Пагинация: `--page-info` выдаёт `{items, pagination}`. Если `next_offset >= 0`, запрашивай следующую страницу с `--offset`. Для пакетов `items` — объект PackageData.
