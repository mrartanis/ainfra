# Инвентарь Keenetic с pi

Статус: реализовано и установлено на pi, 2026-10-22. Адаптер Keenetic собирает `os`, `network`, `wireguard` через HTTPS RCI. Это сбор инвентаря; удалённое управление не входит в runtime сборщика. Учётки на роутерах готовит оператор отдельным helper, а не агент.

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

## Проверка с pi

2026-10-22 сначала проверен `https://artanis-bg.keenetic.pro` из пользовательской SSH-сессии `pi`. HTTPS с проверкой сертификата, без proxy из окружения и без следования redirects. Разовый admin-пароль и cookies на диск pi не записывались. Исходный `/tmp/delme.pwd` удалён после первого теста. При последующем подключении обоих роутеров созданы отдельные readonly-учётки; их секреты сохранены только на pi. Выданные повторно локальные `/tmp/1` и `/tmp/2` удалены после настройки и применения отчётов.

Подтверждено:

- `GET /auth` возвращает 401 с challenge и realm `Keenetic Viva`.
- `POST /auth` с challenge-response возвращает 200 и устанавливает сессию в cookie jar, хранящемся только в памяти процесса.
- `GET /rci/show/version`: Viva (KN-1910), `title=5.1.6`, `release=5.01.C.6.0-1`, архитектура `mips`.
- `GET /rci/show/system`: hostname, CPU load, память и uptime присутствуют в ответе. Uptime передан строкой с числом секунд; CPU load — числом. Память `memtotal`/`memfree` передана в KiB, в observations пересчитывается в байты. [Официальный пример единиц памяти](https://destek.keenetic.com.tr/extra-dsl/kn-2111/tr/22350-using-the-zram-swap-file.html).
- `GET /rci/show/interface`: объект с интерфейсами, включая физические порты, Wi-Fi, bridges и шесть WireGuard-интерфейсов. Для идентификации нужен `row.id`: ключи верхнего объекта не всегда совпадают с ID, у портов встречаются числовые ключи.
- `GET /rci/show/interface?name=WireguardN` и `GET /rci/show/interface/wireguard?name=WireguardN` возвращают публичный ключ интерфейса, peers, состояние и возраст handshake. Отдельный endpoint WireGuard даёт более узкий ответ.
- `GET /rci/show/interface/stat?name=WireguardN` возвращает счётчики и скорости интерфейса.

На снимке пять WireGuard-интерфейсов имели `state=up`, один — `down`. Встречается `last-handshake=2147483647`: его нельзя без проверки трактовать как реальный возраст handshake. Значения `connected=yes/no`, `state=up/down` и числовые строки требуют явной нормализации. HTTP 200 также требует проверки структуры и статуса команды, а не только HTTP-кода.

В loopback metrics wireproxy на pi peer имеет публичный ключ `7iwsOZvCf6LsaxEQyHxKwt88uBO8Qd+io6bcNFNpGS8=`. Он совпадает с ключом `Wireguard2` белградского Keenetic; список peers этого интерфейса содержит публичный ключ pi из каталога. Публичные ключи всех шести интерфейсов внесены в узел роутера для topology. Эта связь не назначает белградский роутер gateway площадки pi: площадки pi и belgrade различны.

Оба заданных имени работают с pi через `/auth`, в том числе с созданными readonly-учётками. Режим самого KeenDNS (прямой/облачный) не считывался из конфигурации. Отдельный endpoint на прямом публичном IP, Digest и длительный live-тест истечения сессии не выполнялись; повтор авторизации проверен protocol tests. IPv4 routes приходят массивом; IPv6 — массивом, `{}` либо `{"route6": [...]}`. Неизвестная routing table остаётся `unknown`.

## Реализация

`ProbeTarget.type` выбирает `routeros` / `keenetic`; отсутствие поля сохраняет прежнее поведение RouterOS. `internal/adapters/keenetic` использует отдельные client/cookie jar, 12-секундный timeout запроса, 90 секунд на сбор, до 2 MiB на ответ. POST используется только для `/auth`, GET — для фиксированных `show` endpoints. Proxy окружения и redirects отключены. Ошибки авторизации не повторяются для каждой секции одного сбора.

Поля отбираются типизированными структурами; сырой ответ не отправляется. Формат WG нормализован к публичному формату Linux-коллектора, поэтому текущий сервер строит topology без обновления server binary. Неизвестные значения остаются отсутствующими; ошибка/partial не заменяет last-good snapshot. Ошибка одного target не прерывает обход остальных.

Проверены auth/cookies/re-auth, RCI errors при HTTP 200, лимиты, TLS/redirects, исключение секретов, unknown metrics, формы routes и независимость targets. CLI-тесты выполнены на самом ARMv7 pi. Автоматическое CVE-покрытие Keenetic не возникает из Debian/Ubuntu feeds.

## Подключение следующего устройства

1. Проверь `ssh pi`, sudo, endpoint с проверкой TLS, права API Infra и отсутствие узла-дубликата. Уточни login и путь к разовому admin-паролю; не читай пароль в вывод. Для readonly нужна версия с исправлением KEN-PSA-2026-WP01 (проверенный нижний предел 5.0.4).
2. На pi должен существовать системный пользователь `infra-probe`. Каталоги `/etc/infra-probe` и `targets` — root:infra-probe 0750. Скопируй операторский helper [provision-user.py](../skills/infra-add-keenetic/scripts/provision-user.py) на pi. Запусти его, передав admin-пароль через SSH stdin:

   ```sh
   scp skills/infra-add-keenetic/scripts/provision-user.py pi:/tmp/infra-provision-user.py
   ssh pi 'sudo -n python3 /tmp/infra-provision-user.py --url https://router.example --credential-output /etc/infra-probe/targets/NAME.credentials.json --owner infra-probe' < /absolute/path/admin-password
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

   Общий config остаётся root:infra-probe 0640, `state_dir=/var/lib/infra-probe`. API endpoint — `https://ainfra.tupo.best`. Top-level `token_file` может ссылаться на первый target; отправка использует отдельный token каждого target.
6. Сборка: `make build-agent-armv7`. Устанавливай бинарник в `/usr/local/bin/infra-probe`, root:root 0755. Используются [service](../packaging/systemd/infra-probe.service) и [timer](../packaging/systemd/infra-probe.timer); Linux-agent pi не заменяется.

   ```sh
   sudo -u infra-probe /usr/local/bin/infra-probe check-config --config /etc/infra-probe/agent.json
   sudo systemctl start infra-probe.service
   systemctl show infra-probe.service -p Result -p ExecMainStatus
   ```

7. Проверь `nodes show NODE_UUID`, last_report и timestamps секций, `status=ok`, `complete=true`, применение отчёта и очередь `infractl request GET ingest/queue --json`. Ack `queued` не заменяет эту проверку. Затем включи `infra-probe.timer` и проверь следующий срок, повторный sequence, карточку/context и topology. Для устройства без WG ожидается успешный пустой снимок.
8. Удали исходные временные admin-password files в согласованный момент после успешной настройки. На pi остаются только отдельные credentials чтения и scoped tokens. Записывай в результат UUID, статусы и расписание, без секретов.

Опрос — **раз в час**, случайный сдвиг до 5 минут (по выбору владельца). Первоначальный вариант раз в 5 минут заменён. Частота настраивается локально на pi; успешно доставленный heartbeat не освежает неуспешную секцию.

## Установленные устройства

| Узел | UUID | Endpoint | Проверенный результат |
|---|---|---|---|
| Keenetic belgrade | aa43b5cd-1b42-4595-9e17-7d2e3f3b29bb | https://artanis-bg.keenetic.pro | Viva KN-1910, 5.1.6, 128 MiB, 36 интерфейсов, 15 IPv4 routes, 6 WG-интерфейсов |
| Keenetic MSK | a14d77cc-c48a-468f-8ce0-ea9a614bea08 | https://artanis.netcraze.pro | Ultra KN-1810, 5.1.6, 256 MiB, 30 интерфейсов, 61 IPv4 routes, WG-интерфейсов нет |

Сервис работает от `infra-probe` без capabilities. Секреты 0600 принадлежат этому пользователю; config 0640 принадлежит root. Обе учётки роутеров имеют только `http` и `readonly`. На момент приёмки все три секции обоих узлов применены с `ok/complete`, inbox пуст. Входные точки отмечены в карточках и на карте меткой `labels.entry_point="true"`; в площадке их может быть несколько. Отдельная сущность Site остаётся [предложением модели](site-gateway.md).

Основа протокола: [официальный справочник RCI](https://storage.googleapis.com/docs.help.keenetic.com/cli/3.1/en/cli_manual_kn-1310_tr.pdf), [HTTP Proxy API и Digest](https://support.keenetic.com/skipper-dsl/kn-2112/en/55035-using-api-methods-through-the-http-proxy-service.html). Описанная выше проверка относится к живому KN-1910 5.1.6, а не ко всем вариантам из справочников.
