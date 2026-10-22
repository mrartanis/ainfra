infractl — клиент HTTP API Infra
  nodes list | nodes show ID | nodes put --file node.json --if-revision N
  attention | packages list NODE [--upgradable] | findings list [--node ID]
  docs search QUERY [--literal] | docs get ID [--revision N]
  docs put --file document.json [--if-revision N]
  docs approve|complete|cancel ID --if-revision N
  relations list | relations suggest [--node ID] | relations put --file relation.json | relations delete ID
  tokens list | tokens issue --file principal.json --output token-file
  tokens revoke ID | feeds status | events | me
  context NODE [--format markdown] | export --output NEW_DIRECTORY
  request METHOD API_PATH [--file JSON] [--if-revision N]
  --limit N --offset N: страница списка; --page-info: данные с метаданными страницы.
  --json: машинный JSON; --config: JSON с endpoint, token_file, ca_file.
  Формат docs put: JSON {id?, title, kind, body, nodes?, tags?}; без id создаёт документ.

Для приоритетного ревью внешних сервисов:
  infractl nodes list --json
  infractl context NODE --format markdown
  infractl request GET topology --json
  infractl packages list NODE --upgradable --json
  infractl findings list --node NODE --json

nodes list содержит ручные services с exposure/primary. Учитывайте страницы списка.
context включает внешние сервисы узла и вложенных гостей; topology возвращает полный компактный граф.
API_PATH задаётся без /api/v1/: для графа это topology. Карта в UI: /map.
