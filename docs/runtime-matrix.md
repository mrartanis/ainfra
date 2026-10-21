# Локальная runtime-матрица

2026-10-21, Linux amd64 native и Linux arm64 через QEMU на amd64 host, Docker. Команда `python3 scripts/collector-matrix.py` после `make build`. Digest образов закреплён в script и используется CI.

| ОС | Установленных пакетов | amd64 | arm64/QEMU |
|---|---:|---|---|
| Debian 12 | 88 | passed | passed |
| Debian 13 | 78 | passed | passed |
| Ubuntu 22.04 | 101 | passed | passed |
| Ubuntu 24.04 | 92 | passed | passed |
| Ubuntu 26.04 | 87 | passed | passed |

Условия: non-root UID/GID, `--network none`, `--read-only`, `--cap-drop ALL`, `no-new-privileges`. Готовый static agent; в контейнер не устанавливались Go/Python/Node/Docker. Проверены OS, полный dpkg inventory с exact binary/source name/arch/version и local disk collector. Ожидаемый набор независимо получен отдельным вызовом dpkg-query. Отчёты в `.local/matrix`, сводки `.local/matrix-results-amd64.json` и `.local/matrix-results-arm64.json`.

Контейнерные package metadata могут быть пустыми/старыми; это не проверка актуальности candidate updates из сети. Не проверены этой матрицей: systemd startup/reboot, WireGuard privileges, running kernel корректность для VM/host, живой RouterOS и выполнение macOS binary. Linux arm64 выполнен через QEMU; macOS arm64 CLI пока проверен сборкой, не runtime.


ARM64 команда: `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 .tools/go/bin/go build -trimpath -o .local/infra-agent-arm64 ./cmd/infra-agent`, затем `python3 scripts/collector-matrix.py --arch arm64 --agent .local/infra-agent-arm64`. Нужен нативный ARM64 Linux либо установленный binfmt/QEMU. Скрипт проверяет ELF architecture и архитектуру отчёта, чтобы случайно не прогнать amd64 agent внутри arm64 image. Child manifest digests взяты из тех же закреплённых multi-platform indexes через registry manifest inspection.

В CI добавлен отдельный нативный `ubuntu-24.04-arm` job для всех пяти образов; этот тип runner документирован в [GitHub-hosted runners reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners). Фактический результат CI проверяется отдельно от локального QEMU-прогона.
