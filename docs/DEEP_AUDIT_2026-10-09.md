# Глубокий аудит Lanpaper

**Дата:** 2026-10-09  
**Версия приложения:** 0.15.1 + исправление `ea5d220`  
**Объём:** безопасность, функциональность, производительность, архитектура, UX/UI, эксплуатация и тестирование.

## 1. Методика и ограничения

Аудит выполнен поэтапно:

1. инвентаризация маршрутов, модулей, зависимостей и границ доверия;
2. трассировка аутентификации, сессий, CSRF, уровней доступа и proxy trust;
3. анализ загрузки файлов, удалённого скачивания, SSRF и файловых операций;
4. анализ конкурентного доступа, атомарности метаданных и восстановления после ошибок;
5. анализ публичной раздачи, кеширования, Range/HEAD и ограничителей ресурсов;
6. анализ frontend/PWA, локализации, доступности и адаптивности;
7. анализ Docker/CI, зависимостей и эксплуатационной документации;
8. запуск доступных автоматических проверок.

Проверено около 18 800 строк основного кода и разметки. В репозитории 148 Go-тестов, 1 benchmark и 31 frontend unit/contract test. Frontend-тесты прошли полностью. `git diff --check` прошёл. В текущем sandbox отсутствуют `go` и `gofmt`, поэтому локально не выполнены `go test -race`, `go vet`, сборка и `govulncheck`; все эти проверки присутствуют в CI. Полноценное динамическое нагрузочное тестирование и тестирование реального reverse proxy не проводились.

## 2. Итоговая оценка

| Область | Оценка | Вывод |
|---|---:|---|
| Безопасность | 8.5/10 | Сильная защита для компактного self-hosted сервиса; критических дефектов при чтении не найдено |
| Надёжность данных | 8.5/10 | Атомарная запись и продуманная публикация файлов; есть крайние случаи деградации диска |
| Функциональность | 8.5/10 | Богатый набор функций при компактном продукте; ряд сценариев требует более явной обратной связи |
| Производительность | 8/10 | Хорошо оптимизирован горячий путь; масштабирование ограничено full-map JSON persistence и монолитным UI |
| Архитектура backend | 8/10 | Ясные слои и хорошие инварианты, но глобальное состояние затрудняет композицию и тестовые окружения |
| Архитектура frontend | 6.5/10 | Работает без framework-зависимостей, но `app.js` стал монолитом в 2809 строк |
| UX/UI и доступность | 8/10 | Сильная адаптивность, клавиатура, ARIA и reduced motion; нужны дополнительные browser/a11y тесты |
| Эксплуатация/CI | 9/10 | Непривилегированный контейнер, pinned Actions, race/vet/vuln checks и подробная документация |

**Общий вывод:** приложение заметно лучше среднего для небольшого self-hosted проекта. Код демонстрирует систематическую работу над fail-closed поведением, SSRF, TOCTOU, кешированием секретных ресурсов, ограничением памяти и конкурентностью. Главные риски теперь не в очевидных уязвимостях, а в редких отказах persistence, сложности монолитного frontend и пределе масштабирования JSON-хранилища.

## 3. Приоритетные находки

### P1 — исправлено в ходе аудита: Basic Auth отменял выход из аккаунта

Десктопный браузер мог автоматически повторно открыть `/admin` сохранёнными Basic credentials после удаления session cookie. Исправлено: browser UI принимает только session cookie; Basic Auth оставлен на API для скриптов. Добавлен регрессионный тест и обновлена документация.

### P2 — отзыв сессии при ошибке записи может отмениться после рестарта

`revokeSession` сначала удаляет digest из памяти, затем пытается сохранить `sessions.json`. Если persistence не удался, DELETE всё равно отвечает `204`, а старый digest остаётся в файле. До рестарта сессия отозвана, но после рестарта она может восстановиться до окончания 14-дневного TTL.

**Риск:** средний по последствиям, низкий по вероятности; нужен одновременно отказ записи и последующий рестарт.

**Рекомендация:** возвращать ошибку из `revokeSession`, отвечать `500` при недолговечном отзыве и показывать пользователю, что сервер не смог гарантировать logout. Более сильный вариант — журнал отозванных digest/tombstone с отдельной атомарной записью.

### P2 — Basic Auth для `auth`-ссылок остаётся origin-wide credential

Публичные ссылки уровня `auth` выдают Basic challenge. Браузер кеширует credentials на origin, после чего может автоматически прикладывать их к API. Исправление `/admin` гарантирует отображение формы после logout, но другой открытый tab или запрос API потенциально всё ещё может быть авторизован кешированным Basic Auth.

**Рекомендация:** документировать Basic Auth как режим для API-клиентов, а browser-доступ к `auth`-медиа переводить на session cookie без `WWW-Authenticate`, либо выделить media/admin на разные origins.

### P2 — файловая модель метаданных имеет предел масштабирования

Каждая мутация клонирует map, сериализует весь `wallpapers.json`, делает `fsync` и rename. Это отлично для целостности, но стоимость записи растёт линейно с числом ссылок. `writeMu` сериализует все изменения.

**Рекомендация:** определить поддерживаемый SLO/лимит числа ссылок; добавить benchmark на 1k/10k/50k записей. При выходе за целевой предел перейти на SQLite/WAL или snapshot + append-only journal.

### P2 — frontend-монолит повышает риск регрессий

`static/js/app.js` содержит 2809 строк и объединяет состояние, API, rendering, dialogs, upload, playlist, history, settings и keyboard handling. Contract-тесты полезны, но большинство поведения не тестируется через DOM.

**Рекомендация:** без обязательного framework разделить код на ES modules: `api`, `state`, `render`, `dialogs`, `uploads`, `history`, `settings`, `a11y`. Сохранить CSP и отсутствие внешнего runtime.

### P3 — ограниченная динамическая проверка UI

Playwright покрывает login/logout на desktop и phone, но не основные операции панели, keyboard navigation, import/export, upload, history и playlist. Нет автоматического axe/accessibility анализа и visual regression.

**Рекомендация:** добавить критический E2E-набор и axe-core; минимум — create/upload/change access/copy token/rotate/delete, плюс keyboard-only сценарий.

### P3 — operational visibility ограничена логами

Есть хорошие security log lines и health/readiness, но нет структурированных метрик: latency, active uploads, rate-limit rejects, decode queue, disk usage, persistence failures.

**Рекомендация:** опциональный `/metrics` под admin auth или structured JSON logging. Не добавлять visitor identifiers сверх уже используемого rate key.

## 4. Безопасность

### 4.1 Аутентификация и сессии

**Сильные стороны**

- 256-битные случайные session tokens.
- На диске хранятся только SHA-256 digest и expiry; файл создаётся с `0600`.
- Cookie: `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` при HTTPS.
- Сессия не выдаётся, если её невозможно сохранить.
- TTL ограничен 14 днями, максимум 1000 активных сессий.
- Basic и form login используют единый lockout budget.
- Сравнение секретов сделано constant-time после hashing, поэтому длина не даёт раннего выхода.
- При повреждении session storage система fail-safe заставляет войти заново.

**Замечания**

- Пароль администратора остаётся shared secret и сравнивается напрямую; это допустимо для env-based self-hosting, но не даёт индивидуальных пользователей, MFA, audit trail или password rotation без рестарта.
- Сессии не имеют metadata по устройствам и нет функции «выйти со всех устройств».
- Expired digest удаляется из памяти лениво и не сразу сохраняется на диск; это не создаёт доступ, но оставляет мусор до следующей записи.
- См. P2 о persistence ошибки при logout.

**Рекомендации**

1. Исправить durable logout.
2. Добавить endpoint `DELETE /api/sessions` для отзыва всех сессий.
3. В долгосрочной перспективе хранить hash пароля (Argon2id/bcrypt) либо явно оставить credentials только в secret manager/env и запретить `adminPass` в `config.json`.
4. Добавить optional session idle timeout и отображение даты истечения.

### 4.2 Авторизация и уровни доступа

Модель `public/local/token/auth` реализована последовательно. Проверка происходит до открытия файла и до условного `304`. Неизвестный access level fail-closed. Token имеет 256 бит энтропии и сравнивается constant-time. Admin preview отделён от public static path.

Особенно хорошо:

- `DISABLE_AUTH=true` не делает `auth`-ссылки публичными;
- publish key имеет узкую capability и не может заменить уже опубликованный live media;
- selector версии/playlist разрешается только через metadata, а не превращается в путь;
- секретные token/auth media имеют `no-store`.

Риск остаётся у browser Basic Auth, описанный выше. Также access tokens лежат в `wallpapers.json` открытым текстом. Это функционально необходимо для показа/copy URL, но означает: backup data directory равен доступу ко всем token links. Это должно оставаться явно задокументировано.

### 4.3 CSRF, CORS и browser isolation

Защита сильная:

- unsafe methods проверяют `Sec-Fetch-Site` и строгий Origin с port;
- forwarded host/proto доверяются только известному proxy;
- CSP без `unsafe-inline`/`unsafe-eval`;
- `frame-ancestors 'none'`, XFO DENY, COOP/CORP/COEP, nosniff, no-referrer;
- HSTS только когда запрос действительно HTTPS;
- CORS закрыт по умолчанию и allowlist нормализуется.

Наблюдение: non-browser клиент без Origin допускается к CSRF middleware, что корректно, поскольку затем обязан пройти auth. Это осознанная и документированная модель.

### 4.4 SSRF и удалённое скачивание

Одна из самых сильных частей приложения:

- запрещены private, loopback, link-local, metadata, multicast, transition и special-purpose ranges;
- проверяются все DNS answers;
- соединение pin-ится на проверенный IP, а Host/SNI сохраняются;
- каждый redirect проверяется заново;
- для HTTP proxy предотвращено повторное разрешение original hostname;
- лимитируются redirects, headers, body и общий timeout;
- download streaming идёт на диск.

Остаточный operational риск: `INSECURE_SKIP_VERIFY` выключает проверку как target, так и HTTPS proxy. Предупреждение есть. Желательно разделить эти opt-in настройки, чтобы self-signed target не ослаблял proxy TLS.

### 4.5 Загрузка и обработка файлов

Сильные стороны:

- request/body/file limits;
- временные файлы вместо RAM buffering;
- MIME/формат проверяется содержимым;
- изображения полностью декодируются до публикации;
- есть budget конкурентного image decode и общий upload semaphore;
- stage и destination находятся на одном filesystem;
- publish через rename с rollback;
- MP4 brand validation;
- local gallery открывается через `os.Root`, защищая от symlink race/traversal;
- media открывается с `O_NOFOLLOW` и проверкой regular file.

Рекомендации:

- fuzzing для parsers/inspectMediaFile и multipart form;
- fixture tests для polyglot/zip-bomb-подобных изображений и extreme dimensions;
- separate CPU budget для preview regeneration, чтобы admin maintenance не влиял на upload latency.

### 4.6 Rate limiting и DoS

Есть public/upload rate limits, brute-force lockout, IPv6 /64 grouping, bounded maps с cleaner, request deadlines, header limits и upload concurrency. Это хорошая база.

Пределы:

- rate limit in-memory и per-instance; несколько replicas умножают бюджет;
- fixed-window допускает burst на границе окна;
- `0` отключает некоторые limits — допустимо, но опасно на Internet-facing install;
- public media bandwidth по определению может быть главным DoS-вектором.

Рекомендуется документировать single-instance semantics и добавить предупреждение при отключённых лимитах на публичном bind.

### 4.7 Supply chain и контейнер

Положительно:

- Actions pinned по commit SHA;
- Dependabot для Go, Actions и Docker;
- `go mod verify`, `go vet`, race tests, govulncheck;
- multi-stage static build;
- runtime non-root;
- data directory `0700`;
- TLS >= 1.2;
- минимальный runtime image.

Рекомендации:

- генерировать SBOM и provenance attestation;
- подписывать container image (cosign);
- добавить read-only root filesystem и `no-new-privileges` в deployment example;
- проверить, что версия Go 1.27 builder реально доступна/поддерживается в release pipeline относительно `go 1.26.0`.

## 5. Надёжность и целостность данных

### 5.1 Что реализовано хорошо

- metadata persist-before-publish: клиент не видит состояние, которое не удалось записать;
- temp + fsync + rename + best-effort directory sync;
- readers не держатся во время disk I/O;
- map records возвращаются как копии, включая slices;
- per-link locks имеют стабильный порядок и освобождаются из registry;
- media replacement имеет backup и rollback;
- startup отказывается перезаписывать повреждённые metadata;
- version numbers не переиспользуются, уменьшая cache confusion.

### 5.2 Крайние случаи

- Metadata и media не могут быть одной ACID-транзакцией. Код тщательно компенсирует ошибки, но power loss между rename файла и metadata commit остаётся сложным классом сценариев.
- Некоторые cleanup ошибки только логируются, что может оставить orphan files и расход диска.
- `historyBytesTotal` зависит от дисциплины вызова `NoteHistoryBytes`; комментарии и тесты помогают, но инвариант не инкапсулирован типом.

**Рекомендации:** startup reconciliation report, dry-run repair command и периодический orphan scanner. Не удалять автоматически без отчёта; сначала безопасный audit mode.

## 6. Производительность

### 6.1 Сильные решения

- публичная раздача сохраняет `sendfile` через `ReadFrom`;
- media не gzip-ится, Range и HEAD не ломаются;
- текст gzip BestSpeed и writer pooling;
- sorted snapshot кешируется;
- remote/upload streaming;
- previews уменьшают panel bandwidth;
- playlist items не создают ненужные previews;
- hot path избегает parsing query без query string;
- `atomic.Pointer` для parsed proxy config;
- stats in-memory и не блокируют persistence.

### 6.2 Bottlenecks

1. Full JSON rewrite на каждую мутацию.
2. Full client-side array filtering/sorting/rendering; chunking уменьшает DOM, но state всё равно целиком в памяти.
3. Preview/image encoding CPU-bound.
4. Один процесс и локальный filesystem исключают горизонтальное масштабирование без внешней координации.
5. Service worker использует network-first для static assets: корректно для freshness, но первый ответ всегда зависит от сети.

### 6.3 План измерений

- benchmark CRUD при 1k/10k/50k links;
- p50/p95/p99 public media latency с Range и 304;
- concurrent upload benchmark по форматам и megapixels;
- panel startup/render при 100/1k/5k links;
- disk-full fault injection;
- proxy + TLS + slow-client scenarios.

## 7. Архитектура

### Backend

Слои `config → middleware → handlers → storage/utils` понятны. Авторизация централизована; storage не зависит от HTTP; helpers для SSRF/path validation изолированы. Комментарии фиксируют security invariants — это повышает сопровождаемость.

Недостатки:

- глобальные `config.Current`, `storage.Global`, limiter/session stores усложняют multiple app instances в одном процессе и dependency injection;
- route dispatch частично ручной через suffix/contains;
- handlers остаются крупными orchestration functions;
- persistence interface отсутствует, поэтому переход с JSON потребует заметного refactor.

Рекомендуемое направление без big-bang rewrite:

1. `App`/`Server` struct с config, store, sessions, limiter;
2. interfaces только на устойчивых границах (`WallpaperStore`, `SessionStore`, `RemoteFetcher`);
3. small services для upload transaction/history/playlist;
4. затем optional SQLite implementation.

### Frontend

Плюсы: нулевой runtime dependency, быстрый deploy, CSP-friendly, plain DOM, единый state, локализация отделена JSON-файлами.

Минусы: один огромный closure, глобальный bridge `window.LanpaperApp`, ручное управление overlays/focus/state и тесная связь rendering с mutation logic.

Рефакторинг должен сохранять progressive simplicity: native ES modules, без обязательного bundler. Сначала вынести чистые функции и API; потом components/dialog controllers.

## 8. Функциональность и UX

### Положительно

- create/rename/delete/pin/filter/sort/search;
- upload file/URL/server gallery;
- history/rollback и playlist rotation;
- access levels и token rotation/copy;
- import/export без утечки access tokens;
- PWA, темы, palette, 6 языков;
- responsive desktop/tablet/mobile layouts;
- keyboard shortcuts и focus trap;
- clear destructive confirmations;
- mobile canvas safeguards.

### Недочёты и улучшения

- logout persistence failure сейчас скрыт;
- import валидирует до 5000 записей, но создаёт их последовательно — операция может быть очень долгой; нужен progress/cancel и server bulk endpoint;
- export называется backup, но не содержит media, tokens и не восстанавливает часть metadata — интерфейс должен максимально явно называть это «экспорт списка ссылок» (в UI уже ближе к правде, документацию следует держать синхронно);
- network errors показываются toast, но долгие операции нуждаются в retry affordance;
- нет UI управления активными сессиями;
- нужны empty/error states для частично недоступного storage/read-only gallery.

## 9. Дизайн и доступность

Проверка разметки показывает хорошие практики:

- semantic buttons/forms/labels;
- dialogs, alertdialog, tablist/tabpanel, radiogroup;
- `aria-expanded`, `aria-selected`, `aria-live`, `aria-busy`;
- focus-visible;
- keyboard focus trap;
- `prefers-reduced-motion` и `prefers-contrast`;
- touch-specific target adjustments;
- breakpoints от 340px до wide desktop.

Что проверить динамически:

- корректное объявление tabs screen reader-ом после rerender;
- возврат focus при nested dialogs;
- toast announcements без повторного `role=status` nesting;
- contrast всех palette/theme combinations;
- zoom 200–400%;
- long German/French strings;
- safe-area inset на iOS PWA;
- orientation change и virtual keyboard.

Рекомендуется автоматический axe pass и ручная матрица VoiceOver/NVDA/TalkBack хотя бы перед major releases.

## 10. Тестирование

Тестовая база необычно сильная для размера проекта: security headers, proxy trust, SSRF, concurrency, persistence, snapshots, compression, readonly, media formats и frontend contracts.

Следующие пробелы наиболее ценны:

1. fault injection durable logout;
2. E2E всех основных admin workflows;
3. fuzz tests для URL, selectors, multipart, media sniffers;
4. browser tests в Chromium + WebKit, особенно PWA/iOS-подобный flow;
5. performance regression thresholds;
6. recovery после SIGKILL/power-loss в разных точках upload transaction;
7. malformed/corrupt metadata repair workflow.

## 11. Рекомендуемый roadmap

### Немедленно

1. Сделать logout durability observable и не отвечать успехом при failed persistence.
2. Запустить CI после исправления Basic-auth logout: gofmt, vet, race, govulncheck, E2E.
3. Явно описать residual browser Basic Auth behavior для `auth` links.

### Ближайший релиз

4. Добавить session management/revoke-all.
5. Расширить Playwright на CRUD/upload/access/history.
6. Добавить metrics или structured operational counters.
7. Добавить benchmark больших metadata sets.

### Среднесрочно

8. Разбить `app.js` на native modules.
9. Ввести `App` struct вместо большинства globals.
10. Сделать audit/repair utility для orphan/missing files.
11. Добавить SBOM, image signing и hardened compose example.

### При росте данных

12. Перейти на SQLite/WAL за `WallpaperStore` interface.
13. Добавить server-side pagination/filtering.
14. Рассмотреть background worker pool для decode/preview tasks.

## 12. Финальное заключение

Lanpaper имеет зрелую defensive design культуру: fail-closed defaults, узкие capabilities, атомарные записи, строгую работу с путями, SSRF pinning, качественные security headers и тесты на конкурентность. После исправления desktop logout наиболее важны не срочные «дыры», а повышение гарантий при отказе диска, устранение остаточного browser Basic Auth coupling, модульность frontend и измеримый предел JSON persistence.

На основании статического анализа критических уязвимостей не обнаружено. Это не является формальной гарантией безопасности: перед Internet-facing deployment нужны успешный CI, актуальный `govulncheck`, динамические proxy/TLS tests и периодический внешний review.
