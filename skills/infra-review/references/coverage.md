# Покрытие и неизвестность

Оценка основана на пакетах дистрибутива и source version. Версия APT candidate не является доказательством installability. Hold виден отдельно. Unknown origin, backports без доказуемого покрытия и специальные каналы не следует считать чистыми.

Ubuntu без fixed может означать и affected, и needs-triage: `affected_or_under_investigation`. USN без однозначной CVE-записи не усиливает вывод. Pro — отдельный канал.

Remediation: `candidate_fixes`, `fix_published_not_candidate`, `no_fix_published`, `unknown`. Подавление — ручное решение со сроком, не изменение assessment. Отсутствующий collector и сломанный feed не закрывают finding.

Runtime patches, Livepatch и эксплуатируемость не оцениваются. RouterOS version собирается, CVE coverage RouterOS не обещается. Последняя успешная копия, последняя попытка и restore test — разные факты.
