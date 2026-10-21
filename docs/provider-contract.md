# Контракт providers и provenance

Проверка официальных источников и fixtures: 2026-10-21.

- Go 1.27.3: https://go.dev/dl/?mode=json ; SHA256 архива проверен перед установкой.
- modernc.org/sqlite v1.61.0: https://pkg.go.dev/modernc.org/sqlite ; FTS5 и VACUUM INTO проверены на реальной БД.
- Debian tracker: https://security-tracker.debian.org/tracker/data/json ; семантика https://security-team.debian.org/security_tracker.html . Fixture `fixtures/feeds/debian-openssl.json` — извлечённая source-группа из реального snapshot.
- Canonical: https://documentation.ubuntu.com/security/security-updates/osv/ ; фактический архив https://security-metadata.canonical.com/osv/osv-all.tar.xz . `ubuntu-0.json`, `ubuntu-1.json`, `ubuntu-cve.json` извлечены из архива без изменения provider content. Canonical data: CC-BY-SA-4.0, https://github.com/canonical/ubuntu-security-notices . Fixtures сохраняют attribution и исходные идентификаторы.
- Dpkg/APT fixtures получены локально на Ubuntu 22.04 через команды из ТЗ, `LC_ALL=C`. Исходные форматы: https://manpages.debian.org/trixie/dpkg/dpkg-query.1.en.html и https://manpages.debian.org/trixie/apt/apt-cache.8.en.html . Repository mirror URL не является credential.
- WG selectors: https://git.zx2c4.com/wireguard-tools/about/src/man/wg.8 . RouterOS protocol: https://help.mikrotik.com/docs/spaces/ROS/pages/47579162/REST+API ; protocol tests используют synthetic HTTPS server, не живое устройство.

Application зависит от нормализованного AdvisorySnapshot, не от HTTP/tar/OSV. FetchTo использует conditional headers; parser передаёт ограниченный поток в Stage. Stage группирует записи на диске, дедуплицирует по CVE/release/channel/source, предпочитает CVE record перед USN. USN-only остаётся unknown: связь upstream не доказывает применимость каждой CVE каждому source из notice.

Ubuntu ingestion охватывает 22.04/24.04/26.04 LTS, обычный/Pro раздельно. Специальные FIPS/Realtime/Livepatch ecosystems не приравниваются к обычным. Debian использует source version и конкретный release. `fixed_version=0` — vendor not-affected sentinel; resolved с ненулевой fixed version сравнивается с установленной source version.

Бюджеты: HTTP Debian 256 MiB; Canonical compressed 80 MiB, expanded stream 16 GiB, record 16 MiB; normalized staging 1 GiB, 4 млн записей, 20 тыс. sources, максимум 100 тыс. уникальных записей на source. Большие пределы expanded stream обусловлены реальным snapshot ~12 GB при ~48 MB сжатия; файлы архива не извлекаются на диск.
