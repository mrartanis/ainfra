# Границы безопасности

API использует случайные 256-битные bearer tokens; SQLite хранит SHA-256 digest. Agent/probe могут только POST observations для назначенных UUID. Node identity не определяется hostname/IP. Reader читает, editor редактирует документы/черновики, admin управляет инвентарём и approval. Bootstrap — локальная команда, публичной регистрации нет.

UI имеет собственные ограниченные по времени сессии в памяти, HttpOnly/SameSite=Strict/Secure cookies и CSRF для POST. Авторизация API не зависит от cookie. Токен UI остаётся в памяти процесса и исчезает при рестарте. HTTPS выполняет внешний proxy. Не публикуйте loopback HTTP порты непосредственно в интернет.

Нет remote executor, SSH shell или командной очереди. Конфигурация collectors/checks/probe задаётся локально. WireGuard dump/showconf/private/preshared keys не запрашиваются. Checks принимают типизированные поля. Тексты документов и vendor evidence недоверенные; UI экранирует HTML, context помечает границу данных.

Максимальный HTTP body 16 MiB, Content-Encoding не поддерживается. API errors не содержат raw stderr/credentials. Private CA поддерживается, TLS verification не отключается. Сборщики ограничивают время и stdout. Системные зависимости берутся с доверенного PATH службы.

SQLite и backup хранят чувствительную инфраструктурную информацию. State directory 0700, секреты 0600. Audit не криптографически защищён от владельца БД. Retention удаляет старую историю согласно политике; это не архив для compliance.

Модель происхождения APT — совпадение repository metadata, не криптографическое доказательство исторической установки .deb. Runtime patching, достижимость уязвимого кода и exploitability не оцениваются.
