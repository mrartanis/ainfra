# Локальная runtime-матрица

2026-10-21, Linux amd64, Docker. Команда `python3 scripts/collector-matrix.py` после `make build`. Digest образов закреплён в script и используется CI.

| ОС | Установленных пакетов | Результат |
|---|---:|---|
| Debian 12 | 88 | passed |
| Debian 13 | 78 | passed |
| Ubuntu 22.04 | 101 | passed |
| Ubuntu 24.04 | 92 | passed |
| Ubuntu 26.04 | 87 | passed |

Условия: non-root UID/GID, `--network none`, `--read-only`, `--cap-drop ALL`, `no-new-privileges`. Готовый static agent; в контейнер не устанавливались Go/Python/Node/Docker. Проверены OS, полный dpkg inventory с exact binary/source name/arch/version и local disk collector. Ожидаемый набор независимо получен отдельным вызовом dpkg-query. Отчёты в `.local/matrix`, сводка `.local/matrix-results.json`.

Контейнерные package metadata могут быть пустыми/старыми; это не проверка актуальности candidate updates из сети. Не проверены этой матрицей: systemd startup/reboot, WireGuard privileges, running kernel корректность для VM/host, живой RouterOS, выполнение arm64/macOS binaries. Linux arm64 и macOS arm64 CLI пока проверены сборкой, не runtime.
