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
  Формат docs put: JSON {id?, title, kind, body, nodes?}; без id создаёт документ.
