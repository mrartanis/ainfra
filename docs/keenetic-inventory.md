# Инвентарь Keenetic через probe

Адаптер реализован; конкретные установки и результаты обслуживания хранятся в Infra. Адаптер Keenetic собирает `os`, `network`, `wireguard` через HTTPS RCI. Это сбор инвентаря; удалённое управление не входит в runtime сборщика. Учётки на роутерах готовит оператор отдельным helper, а не агент.

## Размещение и доступ

Обход Keenetic выполняет сервер `pi`. Для каждого роутера оператор задаёт HTTPS endpoint: внешний адрес при наличии прямого доступа либо KeenDNS через облако. LAN/VPN не является обязательным транспортом. Сервер Infra принимает observations и сам к роутерам не подключается.

Endpoint и способ авторизации — разные параметры: имя KeenDNS само по себе не определяет используемую схему входа. Проверка с pi должна предшествовать подключению каждого нового варианта доступа. Реализован web `/auth` с challenge-response и cookies; HTTP Proxy Digest пока не поддержан. Не переключаться автоматически на HTTP, другой origin или другой адрес и не пересылать credentials по redirect. Для прямого IP сертификат также должен проходить проверку; при необходимости используется DNS-имя или доверенный CA.

Рабочая схема:

```text
pi → HTTPS /auth + /rci/show/... → Keenetic (внешний адрес / облако)
pi → HTTPS /api/v1/ingest/reports → Infra → UI / CLI / внешний LLM-администратор
```

## Секреты

Постоянному сборщику нужна отдельная учётка с минимальными доступными правами чтения. Рабочая копия её секрета хранится на pi, например `/etc/infra/secrets/keenetic-<node-id>.json`, и читается сервисным пользователем probe; файл 0600, каталог 0700. Это доступ сборщика, не credentials для администрирования. Для каждого target используется отдельный scoped bearer token Infra. Пароли, cookies и заголовки авторизации не входят в отчёт или логи.

Credentials внешнего LLM-администратора хранятся на машине его запуска, за пределами Infra. Переданный для разового теста admin-пароль не сохраняется как постоянный доступ pi. Выдача новой учётки и изменение прав роутера не выполняются сборщиком.

Перед выбором readonly-учётки нужно проверить версию: [бюллетень Keenetic KEN-PSA-2026-WP01](https://keenetic.com/en/security?lang=en) описывает повышение прав из readonly до администратора в версиях до 5.0.4. Успешный опрос с admin не доказывает работоспособность будущей учётки с меньшими правами.

## Проверка протокола

Перед подключением проверь HTTPS, `/auth` и доступ через новую readonly-сессию. Сохраняй результат, адреса, UUID и проверенные публичные ключи только в Infra; секреты — в защищённой конфигурации сборщика. В репозиторий не добавляй отчёты с реальных устройств.

- `GET /auth` возвращает challenge и realm; `POST /auth` с challenge-response создаёт сессию. Cookies хранятся в памяти процесса.
- `/rci/show/version` определяет модель, архитектуру и версию. Версия нужна для проверки исправления KEN-PSA-2026-WP01.
- `/rci/show/system` содержит uptime, CPU load и память. Строковые числа нормализуются; KiB переводятся в байты. [Официальный пример единиц памяти](https://destek.keenetic.com.tr/extra-dsl/kn-2111/tr/22350-using-the-zram-swap-file.html).
- `/rci/show/interface` возвращает объект; для идентификации используется `row.id`, поскольку ключи верхнего уровня могут быть числовыми.
- `/rci/show/interface/wireguard?name=WireguardN` содержит публичные ключи, peers и handshake. Служебное значение `2147483647` нельзя принимать за достоверный возраст handshake.
- IPv4 routes приходят массивом; IPv6 могут приходить массивом, `{}` либо `{"route6": [...]}`. Неизвестная таблица остаётся `unknown`.

HTTP 200 не заменяет проверку структуры и RCI status. Наблюдаемый peer не назначает gateway площадки; для связи нужны проверенные публичные ключи обеих сторон. Состояние интерфейсов, конкретные связи и доступность проверяются заново для каждого узла.

## Реализация

`ProbeTarget.type` выбирает `routeros` / `keenetic`; отсутствие поля сохраняет прежнее поведение RouterOS. `internal/adapters/keenetic` использует отдельные client/cookie jar, 12-секундный timeout запроса, 90 секунд на сбор, до 2 MiB на ответ. POST используется только для `/auth`, GET — для фиксированных `show` endpoints. Proxy окружения и redirects отключены. Ошибки авторизации не повторяются для каждой секции одного сбора.

Поля отбираются типизированными структурами; сырой ответ не отправляется. Формат WG нормализован к публичному формату Linux-коллектора, поэтому текущий сервер строит topology без обновления server binary. Неизвестные значения остаются отсутствующими; ошибка/partial не заменяет last-good snapshot. Ошибка одного target не прерывает обход остальных.

Проверены auth/cookies/re-auth, RCI errors при HTTP 200, лимиты, TLS/redirects, исключение секретов, unknown metrics, формы routes и независимость targets. CLI поддерживает сборку для ARMv7. Автоматическое CVE-покрытие Keenetic не возникает из Debian/Ubuntu feeds.

## Подключение следующего устройства

1. Проверь SSH-доступ к probe-host, sudo и endpoint с проверкой TLS, права API Infra и отсутствие узла-дубликата. Уточни login и путь к разовому admin-паролю; не читай пароль в вывод. Для readonly нужна версия с исправлением KEN-PSA-2026-WP01 (проверенный нижний предел 5.0.4).
2. На pi должен существовать системный пользователь `infra-probe`. Каталоги `/etc/infra-probe` и `targets` — root:infra-probe 0750. Скопируй операторский helper [provision-user.py](../skills/infra-add-keenetic/scripts/provision-user.py) на pi. Запусти его, передав admin-пароль через SSH stdin:

   ```sh
   scp skills/infra-add-keenetic/scripts/provision-user.py PROBE_HOST:/tmp/infra-provision-user.py
   ssh PROBE_HOST 'sudo -n python3 /tmp/infra-provision-user.py --url https://router.example --credential-output /etc/infra-probe/targets/NAME.credentials.json --owner infra-probe' < /absolute/path/admin-password
   ```

   Новый пользователь имеет имя `infra_pi_<random>` и отдельный случайный пароль. Helper не меняет `admin`, устанавливает пароль до выдачи доступа, проверяет ровно теги `http`/`readonly` и чтение через новую сессию, затем сохраняет конфигурацию. Если есть несохранённые изменения или версия не подтверждена, он останавливается до мутаций. Ошибка оставляет секрет для диагностики/повторного запуска; не создавай новые учётки и не повторяй неуспешные входы циклом. Отдельная проверка без admin-пароля и без изменений: те же аргументы плюс `--verify-only`.

   Проверенный запрос установки пароля на 5.1.6:

   ```json
   {"user":{"name":"infra_pi_EXAMPLE","password":{"md5":{"hash":"MD5(username:realm:password)"}}}}
   ```

   Это POST `/rci/`. Realm берётся из `/auth`, не задаётся константой. Плоское `password` и `password.md5` со строкой не являются рабочей заменой: возможны пустой ответ/RCI error внутри HTTP 200. В имени пользователя нельзя использовать дефис. Пароль и его MD5 — секреты; helper их не печатает.
3. Через admin API/CLI создай `Node(kind=router)` с именем, hostname, меткой площадки `labels.provider`, URL и проверенными public keys всех WG-интерфейсов. IP облачного proxy не записывай как адрес роутера. Не создавай `runs_on` от NAS/pi к маршрутизатору.
4. Подготовь principal `{"role":"probe","nodes":["NODE_UUID"],"instance":"PROBE_INSTANCE_UUID"}`. Для установленного pi instance возьми из `/etc/infra-probe/agent.json`, не меняй его при добавлении target. `infractl tokens issue --file principal.json --output NEW_PRIVATE_TOKEN_FILE` сохраняет секрет в файл, а не stdout. Передай его по SSH на pi в `/etc/infra-probe/targets/NAME.token`, infra-probe:infra-probe 0600, удали промежуточную копию. Admin token Infra на pi не копируется.
5. Добавь target в существующий конфиг, сохранив прежние targets и state. Пример одного target:

   ```json
   {"type":"keenetic","node_id":"NODE_UUID","url":"https://router.example","credential_file":"/etc/infra-probe/targets/NAME.credentials.json","token_file":"/etc/infra-probe/targets/NAME.token","ca_file":""}
   ```

   Общий config остаётся root:infra-probe 0640, `state_dir=/var/lib/infra-probe`. API endpoint задаётся конфигурацией, например `https://infra.example.org`. Top-level `token_file` может ссылаться на первый target; отправка использует отдельный token каждого target.
6. Сборка: `make build-agent-armv7`. Устанавливай бинарник в `/usr/local/bin/infra-probe`, root:root 0755. Используются [service](../packaging/systemd/infra-probe.service) и [timer](../packaging/systemd/infra-probe.timer); Linux-agent pi не заменяется.

   ```sh
   sudo -u infra-probe /usr/local/bin/infra-probe check-config --config /etc/infra-probe/agent.json
   sudo systemctl start infra-probe.service
   systemctl show infra-probe.service -p Result -p ExecMainStatus
   ```

7. Проверь `nodes show NODE_UUID`, last_report и timestamps секций, `status=ok`, `complete=true`, применение отчёта и очередь `infractl request GET ingest/queue --json`. Ack `queued` не заменяет эту проверку. Затем включи `infra-probe.timer` и проверь следующий срок, повторный sequence, карточку/context и topology. Для устройства без WG ожидается успешный пустой снимок.
8. Удали исходные временные admin-password files в согласованный момент после успешной настройки. На pi остаются только отдельные credentials чтения и scoped tokens. Записывай в результат UUID, статусы и расписание, без секретов.

Опрос — **раз в час**, случайный сдвиг до 5 минут (по выбору владельца). Первоначальный вариант раз в 5 минут заменён. Частота настраивается локально на pi; успешно доставленный heartbeat не освежает неуспешную секцию.

## Учёт установленного оборудования

Узлы, UUID, endpoints, публичные ключи, результаты проверок и расписания сохраняются в Infra. Проверяй владельцев credentials/config, минимальные права роутерных учёток, timestamps всех секций, состояние inbox и таймер. Эти сведения не дублируются в Git.

Основа протокола: [официальный справочник RCI](https://storage.googleapis.com/docs.help.keenetic.com/cli/3.1/en/cli_manual_kn-1310_tr.pdf), [HTTP Proxy API и Digest](https://support.keenetic.com/skipper-dsl/kn-2112/en/55035-using-api-methods-through-the-http-proxy-service.html). Поддержка конкретной модели и версии подтверждается отдельной проверкой, а не одним наличием метода в справочнике.
