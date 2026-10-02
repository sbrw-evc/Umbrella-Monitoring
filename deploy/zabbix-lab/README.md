# Стенд Umbrella

Стенд поднимает на одном сервере Umbrella MVP и источники событий вокруг него:

- Zabbix 7.0: сервер, веб-интерфейс, PostgreSQL и agent 2;
- Prometheus, Alertmanager и node-exporter;
- OpenSearch и OpenSearch Dashboards;
- Grafana с дашбордом инцидентов;
- приёмник уведомлений, который заменяет webhook Teams и Zoom.

Скрипты сами связывают всё с Umbrella и проверяют функции MVP тестом.

## Установка одной командой

Подойдёт чистый сервер Debian или Ubuntu: 2 vCPU, 6 ГБ памяти (минимум 4 ГБ), 20 ГБ диска и доступ в интернет. Команду запускают под root:

```bash
curl -fsSL https://raw.githubusercontent.com/sbrw-evc/Umbrella-Monitoring/feature/mvp-app/deploy/zabbix-lab/install.sh | bash
```

Если репозиторий закрытый, клонируйте его и запустите скрипт из клона:

```bash
git clone -b feature/mvp-app https://github.com/sbrw-evc/Umbrella-Monitoring.git /opt/umbrella-monitoring
/opt/umbrella-monitoring/deploy/zabbix-lab/install.sh
```

Что делает `install.sh`:

1. Ставит git, curl, jq, openssl, Docker Engine и плагин compose.
2. Выставляет `vm.max_map_count=262144` для OpenSearch и сохраняет значение в `/etc/sysctl.d/99-umbrella-opensearch.conf`.
3. Пишет `.env` со случайными паролями и токенами. Файл доступен только root. Если `.env` уже есть, скрипт сохраняет его значения и дописывает только недостающие ключи, поэтому старый стенд обновляется без смены паролей.
4. Собирает образ Umbrella и запускает контейнеры (`docker compose up -d --build`).
5. Запускает `configure.sh`, который настраивает связку.
6. Запускает `smoke-test.sh --all` и печатает итог. Тест можно пропустить: `SKIP_TEST=1`.

Скрипт печатает адреса сервисов, а пароли не печатает: они лежат только в `.env`. Повторный запуск обновляет код, пересобирает и перезапускает контейнеры.

### Windows (PowerShell)

Нужен Docker Desktop в режиме Linux-контейнеров и 8 ГБ памяти на машине. Если Docker нет, поставьте его командой `winget install -e --id Docker.DockerDesktop`. Затем выполните в PowerShell:

```powershell
irm https://raw.githubusercontent.com/sbrw-evc/Umbrella-Monitoring/feature/mvp-app/deploy/zabbix-lab/install.ps1 -OutFile $env:TEMP\umbrella-install.ps1; powershell -ExecutionPolicy Bypass -File $env:TEMP\umbrella-install.ps1
```

`install.ps1` делает то же, что `install.sh`, но Docker не ставит, а при необходимости запускает Docker Desktop. Код скачивается в `%USERPROFILE%\umbrella-monitoring` через git, а если git нет, архивом. Параметры: `-Dir`, `-Branch`, `-NoBuild`, `-SkipTest`. Порты и адрес задаются переменными окружения, как в Linux.

Docker Desktop запускает контейнеры в виртуальной машине WSL2, и Windows не может изменить её `vm.max_map_count`. Если контейнер `opensearch` останавливается, выполните:

```powershell
wsl -d docker-desktop sysctl -w vm.max_map_count=262144
```

Чтобы значение сохранялось после перезапуска, добавьте строку `kernelCommandLine = sysctl.vm.max_map_count=262144` в раздел `[wsl2]` файла `%USERPROFILE%\.wslconfig`.

У скриптов настройки и теста тоже есть версии для PowerShell. `configure.ps1` и `smoke-test.ps1 [-Zabbix | -Stack | -All]` повторяют bash-версии и проверяют те же пункты. Они работают в Windows PowerShell 5.1 и PowerShell 7 и обходятся без jq.

Скрипты рассчитаны на режим ConstrainedLanguage (AppLocker или WDAC): в них нет Add-Type и вызовов .NET, только командлеты. HTTP идёт через `Invoke-WebRequest`, секреты берутся из `New-Guid`, архив распаковывается встроенным `tar.exe`. Проверка WebSocket использует встроенный `curl.exe`, а без него пропускается. Если политика запрещает запускать сами файлы .ps1, их нужно подписать или добавить в разрешённые.

## Компоненты и порты

| Что | Адрес | Вход |
| --- | --- | --- |
| Umbrella | `http://<сервер>:8080` (`UMBRELLA_PORT`) | `admin`, пароль `UMBRELLA_ADMIN_PASSWORD` |
| Grafana | `http://<сервер>:3000` (`GRAFANA_PORT`) | `admin`, пароль `GRAFANA_ADMIN_PASSWORD` |
| Zabbix | `http://<сервер>:8081` (`ZABBIX_WEB_PORT`) | `Admin`, пароль `ZABBIX_ADMIN_PASSWORD` |
| Prometheus | `http://<сервер>:9090` (`PROMETHEUS_PORT`) | без входа |
| OpenSearch Dashboards | `http://<сервер>:5601` (`OSD_PORT`) | без входа |
| Alertmanager, node-exporter, OpenSearch, notify-sink | только внутри сети стенда | — |

У Prometheus и OpenSearch Dashboards на стенде нет входа: плагин безопасности OpenSearch отключён. Откройте порты только для своих адресов, например `ufw allow from <ваш IP> to any port 8080,8081,3000,9090,5601 proto tcp`.

Памяти больше всех занимает OpenSearch: куча 512 МБ, около 1 ГБ всего. Весь стенд занимает около 3 ГБ.

### Пароли и токены в `.env`

| Ключ | Для чего |
| --- | --- |
| `UMBRELLA_ADMIN_USER`, `UMBRELLA_ADMIN_PASSWORD` | первый администратор Umbrella. Политика паролей: от 10 символов, буквы и цифры, пароль не совпадает с логином |
| `LAB_OWNER_PASSWORD` | пользователь `lab-owner`, владелец бизнес-услуги `lab-shop` |
| `UMBRELLA_GRAFANA_TOKEN` | API-токен (`umb_...`) сервисной учётной записи `grafana` с ролью reader. Через него Grafana читает API Umbrella |
| `UMBRELLA_METRICS_TOKEN` | bearer-токен для `/metrics`. Prometheus получает его как compose secret. Если ключ пустой, `/metrics` открыт |
| `ZABBIX_WEBHOOK_TOKEN`, `PROMETHEUS_WEBHOOK_TOKEN`, `SMOKE_WEBHOOK_TOKEN` | токены входящих webhook коннекторов. Umbrella читает их как `UMB_SECRET_LAB_*` (`openbao://lab/...`) |
| `GRAFANA_ADMIN_PASSWORD`, `ZABBIX_ADMIN_PASSWORD`, `ZABBIX_DB_PASSWORD` | пароли Grafana и Zabbix |
| `LAB_ZOOM_TOKEN` | verification token, который канал Zoom отправляет приёмнику стенда |
| `TEAMS_WEBHOOK_URL`, `ZOOM_WEBHOOK_URL`, `ZOOM_VERIFICATION_TOKEN` | настоящие Teams и Zoom (см. ниже). Если ключи пустые, уведомления уходят в приёмник стенда |

Шаблон всех ключей лежит в `.env.example`.

## Чистый старт, вход и роли

По умолчанию стенд стартует без демо-данных (`UMBRELLA_DEMO=false`): CMDB, коннекторы и инциденты пусты. Карту наполняет `configure.sh`. Если событие называет неизвестную КЕ, Umbrella сама добавляет её в CMDB как хост с `origin: auto`; метка `service` при этом создаёт ИТ-сервис и связывает с ним хост (`UMBRELLA_CMDB_AUTO`). Чтобы рядом с реальными событиями видеть демо-CMDB и демо-трафик, задайте `UMBRELLA_DEMO=true` в `.env` и выполните `docker compose up -d`.

В веб-интерфейс входят под `admin` с паролем `UMBRELLA_ADMIN_PASSWORD`. Скрипты входят через API:

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/token \
  -d '{"username":"admin","password":"<UMBRELLA_ADMIN_PASSWORD>"}' | jq -r .token)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/incidents?view=open
```

С заголовком Bearer CSRF-токен не нужен. Без входа API отвечает 401. Открыты без входа только приём событий `/api/ingest/<коннектор>`, webhook PagerDuty, `/metrics` (если не задан `UMBRELLA_METRICS_TOKEN`) и `/healthz`.

Встроенные роли:

- admin: всё, включая пользователей и роли;
- monitoring: коннекторы, CMDB, правила, каналы уведомлений;
- oncall: инциденты всех услуг;
- owner: инциденты и обслуживание своих бизнес-услуг;
- viewer: просмотр инцидентов своих бизнес-услуг;
- auditor: журнал аудита;
- reader: сервисное чтение, его использует Grafana.

Роли owner и viewer видят только инциденты и КЕ под своими бизнес-услугами: саму услугу и всё, от чего она зависит.

### Демонстрация области видимости

`configure.sh` строит такую CMDB:

```text
lab-shop (business_service)
  ├─ umbrella-lab (it_service) ─ umbrella-lab-agent (Zabbix), umbrella-lab-node (Prometheus)
  └─ lab-shop-api (it_service) ─ shop-api-1 (логи OpenSearch)
lab-billing (business_service)
  └─ lab-billing-api (it_service) ─ billing-api-1
```

Пользователь `lab-owner` (роль owner, услуга `lab-shop`, пароль `LAB_OWNER_PASSWORD`) видит инциденты Zabbix, Prometheus и OpenSearch по `lab-shop`. Инцидентов `lab-billing` и чужих КЕ он не видит, а коннекторы ему закрыты (403). Чтобы проверить, войдите под ним в другом браузере.

## Как устроены связки

Все коннекторы опубликованы, запущены и имеют короткое имя (slug). Адрес приёма `/api/ingest/<slug>` поэтому не меняется после перезапуска Umbrella.

### Zabbix

```text
Zabbix trigger -> action "Send problems to Umbrella" -> media type "Umbrella" (webhook)
  -> POST http://umbrella:8080/api/ingest/zabbix с заголовком X-Umbrella-Token
  -> коннектор Zabbix: trigger.webhook -> parse.json -> map.severity -> enrich.labels
     -> map.event -> out.event -> ack.response (2xx)
  -> инцидент Umbrella на КЕ umbrella-lab-agent -> PagerDuty (или dry-run)
```

- Важность сопоставляется так: Disaster → critical, High → error, Average и Warning → warning, остальное → info.
- Сигнал строится как `zabbix:<id триггера>`, поэтому повторы одной проблемы складываются в один инцидент. Восстановление из Zabbix закрывает инцидент.
- `configure.sh` меняет пароль `Admin` на пароль из `.env` и создаёт webhook media type «Umbrella». Если Umbrella отвечает не 2xx, Zabbix повторяет отправку до трёх раз.
- Он же создаёт действие для проблем и восстановлений и хост `umbrella-lab-agent` с шаблоном «Linux by Zabbix agent», trapper-элементом `umbrella.test` и триггером «Umbrella test problem» уровня High.

### Prometheus и Alertmanager

```text
node-exporter (метрики хоста и *.prom из node-textfile/) -> Prometheus (правила prometheus/rules/lab.yml)
  -> Alertmanager (alertmanager/alertmanager.yml, group_wait 5s)
  -> POST http://umbrella:8080/api/ingest/prometheus, Authorization: Bearer PROMETHEUS_WEBHOOK_TOKEN
  -> коннектор Prometheus: parse.json (alerts) -> map.severity (labels.severity) -> enrich.labels -> map.event
```

- Коннектор разбирает webhook Alertmanager: из каждого элемента `alerts[]` получается событие.
- КЕ берётся из `labels.host`, а без него из `labels.instance`. Важность берётся из `labels.severity`, метод RED/USE из `labels.method`.
- Заголовок берётся из `annotations.summary`, ID в источнике из `fingerprint`. При `status: resolved` инцидент закрывается.
- Prometheus также собирает `/metrics` самой Umbrella, Alertmanager и себя. Метка `host` у целей задаёт имя КЕ.

Правила стенда:

- `UmbrellaLabTestFailure`: `umbrella_lab_test_failure > 0`, тестовое;
- `HostHighLoad`: load5 выше 2 на CPU 5 минут;
- `HostLowMemory`: свободно меньше 10 % памяти;
- `TargetDown`: цель недоступна.

Тест включает правило `UmbrellaLabTestFailure` так: пишет файл `node-textfile/umbrella_lab.prom` со строкой `umbrella_lab_test_failure 1`, а потом удаляет его. То же можно сделать вручную.

### OpenSearch

```text
приложение пишет логи в индекс lab-logs (@timestamp, level, host, service, message, error_code)
  <- коннектор OpenSearch (pull): trigger.schedule 15s -> fetch.http POST http://opensearch:9200/lab-logs/_search
     (level error/critical/fatal за 15 минут) -> parse.json (hits.hits) -> map.severity -> enrich.labels -> map.event
```

- Один документ даёт одно событие: КЕ из `host`, сигнал `opensearch:<service>:<error_code>`, ID в источнике `_id`. Повторные опросы того же документа отбрасываются.
- Метка `service` привязывает новую КЕ к ИТ-сервису.
- OpenSearch доступен только внутри сети стенда. Скрипты обращаются к нему через консоль OpenSearch Dashboards (`/api/console/proxy`).
- `configure.sh` создаёт индекс `lab-logs` с маппингом и index pattern `lab-logs*` в Dashboards (Discover).

Записать ошибку вручную можно в Dashboards → Dev Tools:

```text
POST lab-logs/_doc?refresh=true
{"@timestamp":"2026-10-02T12:00:00Z","level":"error","host":"shop-api-1","service":"lab-shop-api","message":"Payment gateway timeout","error_code":"PAY-504"}
```

## Grafana

В Grafana при старте подключаются два источника данных и дашборд **Umbrella: инциденты** (uid `umbrella-incidents`, папка Umbrella; он же домашняя страница).

Источники данных:

- `Prometheus` (`http://prometheus:9090`);
- `Umbrella API` (плагин Infinity, `http://umbrella:8080`, bearer-токен `UMBRELLA_GRAFANA_TOKEN`).

Что показывает дашборд:

- активные инциденты по важности, сервисам и командам;
- инциденты в резервном канале, без КЕ, подавленные обслуживанием;
- состояние доставки в PagerDuty;
- динамику открытых и решённых инцидентов;
- события и ошибки разбора по коннекторам;
- доставки в Teams и Zoom;
- состояние коннекторов и CMDB;
- таблицу открытых инцидентов из `GET /api/incidents?view=open`. ID в таблице ведёт в карточку инцидента в Umbrella.

Переменные дашборда `team`, `service` и `severity` берутся из меток метрик. Ссылка «Grafana» в инциденте Umbrella (`UMBRELLA_GRAFANA_URL`) открывает этот дашборд с окном времени инцидента.

Исходник дашборда лежит в `deploy/grafana/umbrella-incidents.yaml`. Это ресурс Grafana 12 (`apiVersion: dashboard.grafana.app/v1beta1`, `kind: Dashboard`), в `spec` которого обычная JSON-модель. Его можно загрузить в другую Grafana через `grafanactl` или kubectl-совместимый клиент. Провижининг из файлов читает только JSON, поэтому после правки YAML выполните:

```bash
python3 deploy/grafana/build.py           # пересобрать deploy/grafana/dashboards/umbrella-incidents.json
python3 deploy/grafana/build.py --check   # проверить, что JSON совпадает с YAML
```

Нужен PyYAML (`apt install python3-yaml`). Grafana перечитывает файлы раз в минуту.

Плагин Infinity Grafana скачивает с grafana.com при старте (`GF_PLUGINS_PREINSTALL`). Без интернета Grafana всё равно запускается, но таблица открытых инцидентов остаётся пустой, а проверка источника `Umbrella API` в тесте не проходит. Остальные панели работают на Prometheus.

## Уведомления в Microsoft Teams и Zoom

`configure.sh` создаёт два канала:

- «Lab Teams»: все услуги, важность от warning, события open, escalate, ack, resolve, fallback;
- «Lab Zoom»: только услуга `lab-shop`, важность от error.

По умолчанию оба канала отправляют сообщения в контейнер `notify-sink`. Он отвечает 200 на любой запрос и пишет его в журнал:

```bash
docker compose logs -f notify-sink
```

Поэтому `UMBRELLA_ALLOW_HTTP_WEBHOOKS=true` разрешён только на стенде. Результаты отправки видны в Umbrella (в разделе каналов уведомлений и через `GET /api/deliveries`) и на дашборде Grafana.

Чтобы отправлять в настоящие мессенджеры, впишите адреса в `.env` и выполните `./configure.sh` ещё раз. Настоящие адреса должны быть https.

**Microsoft Teams (Workflows).**

1. В канале Teams откройте «…» → Workflows и выберите шаблон «Post to a channel when a webhook request is received» («Публиковать в канале при получении запроса webhook»).
2. Укажите команду и канал и сохраните.
3. Скопируйте выданный HTTP POST URL (вида `https://...logic.azure.com/...` или `https://...powerplatform.com/...`).
4. Впишите его в `.env`: `TEAMS_WEBHOOK_URL=<URL>`.

Umbrella отправляет Adaptive Card, этот формат принимают и Workflows, и старые Incoming Webhook (Office 365 connector).

**Zoom Team Chat (Incoming Webhook).**

1. В Zoom App Marketplace установите приложение «Incoming Webhook».
2. В нужном канале Team Chat выполните `/inc connect umbrella`.
3. Бот пришлёт Endpoint URL (`https://integrations.zoom.us/chat/webhooks/incomingwebhook/...`) и Verification Token.
4. Впишите их в `.env`: `ZOOM_WEBHOOK_URL=<Endpoint URL>`, `ZOOM_VERIFICATION_TOKEN=<Verification Token>`.

Umbrella сама добавляет `format=full` и отправляет токен в заголовке `Authorization`.

Адреса webhook можно задать и в веб-интерфейсе, а в целевой схеме хранить в OpenBao (`url_ref: openbao://...`). Режим fallback отправляет сообщение только тогда, когда PagerDuty не принял инцидент вовремя.

## Тест функций MVP

```bash
./smoke-test.sh            # только Umbrella
./smoke-test.sh --zabbix   # плюс сквозная проверка через Zabbix
./smoke-test.sh --stack    # плюс Prometheus, OpenSearch, Teams/Zoom, Grafana, Dashboards
./smoke-test.sh --all      # всё (так запускает install.sh)
.\smoke-test.ps1 -All      # то же в PowerShell
```

Тест входит как администратор и создаёт сервисную учётную запись `smoke-test` (роли monitoring и auditor). Основные проверки идут с её API-токеном. Каждый прогон создаёт свои КЕ, сигналы и пользователей, а в конце удаляет пользователей и токен, поэтому тест можно повторять.

| Раздел | Что проверяется |
| --- | --- |
| Сервис | `/healthz`, `/api/meta`, самодиагностика, палитра блоков, веб-интерфейс |
| Вход и роли | 401 без входа, вход через `/api/auth/token`, `/api/auth/me`, семь встроенных ролей, API-токен, 403 без права, сервисная запись не входит по паролю, токен Grafana только читает, `/metrics` с токеном |
| CMDB | бизнес-услуга, ИТ-сервис и хосты, связи в графе |
| Конструктор | отладочный прогон графа, маппинг важности и шаблоны, ошибка блока на битом JSON, отказ публикации графа без триггера, публикация и запуск |
| Приём | отказ без токена и с неверным токеном (401), приём с токеном (202), привязка к КЕ, сервису и команде, метод RED/USE |
| Дедупликация | повтор external_id отбрасывается, повтор сигнала складывается в один инцидент, важность повышается, второй источник попадает в тот же инцидент |
| Действия | подтверждение, повторное подтверждение (409), комментарий, события в карточке |
| Восстановление | инцидент активен, пока не восстановятся все источники; повтор в окне склейки открывает тот же инцидент; ручное решение |
| Обслуживание | событие во время окна обслуживания подавлено и не уходит в PagerDuty |
| Ошибки | неизвестная КЕ создаётся автоматически (или инцидент без КЕ), битое сообщение попадает в журнал ошибок разбора |
| Групповые действия, представления | подтверждение и решение двух инцидентов; фильтры, тепловая карта, журнал событий, карточка КЕ, WebSocket |
| PagerDuty | имитация отказа и восстановление доставки |
| Коннектор и аудит | остановленный коннектор отклоняет события (409), записи аудита |
| Область видимости | слабый пароль отклоняется, owner видит свою услугу и не видит чужую (404 на чужой инцидент), viewer не может подтвердить (403), owner подтверждает своё |
| Prometheus | цели up, правила загружены, метрика из textfile → правило → Alertmanager → инцидент, удаление метрики → инцидент решён |
| OpenSearch | Dashboards green, index pattern, индекс; ошибка в логе → инцидент на shop-api-1, info не создаёт инцидент, повторные опросы не дублируют; `lab-owner` видит этот инцидент |
| Teams и Zoom | тестовые сообщения (200), сообщения об открытии и решении реального инцидента, фильтр канала Zoom по услуге, счётчик в `/metrics` |
| Grafana | здоровье, дашборд из провижининга, адрес Umbrella в ссылках, источник Prometheus, запрос `umbrella_up` через Grafana, источник Umbrella API (Infinity) |
| Zabbix | вход в API, хост и триггер, доступность агента, проблема → инцидент (High → error), Zabbix отмечает отправку как Sent, восстановление → инцидент решён |

На стенде в изолированной среде без доступа к grafana.com результат был 110 из 111 и в bash, и в PowerShell 7 в режиме ConstrainedLanguage. Не прошла только проверка источника Infinity: плагин не скачался.

![Инцидент из Zabbix в Umbrella](screens/zabbix-incident.png)

![Коннектор Zabbix в конструкторе](screens/zabbix-connector.png)

![Журнал действий Zabbix: отправки в Umbrella](screens/zabbix-actionlog.png)

## Управление

```bash
cd /opt/umbrella-monitoring/deploy/zabbix-lab
docker compose ps
docker compose logs -f umbrella
docker compose down        # остановить; данные Zabbix, Prometheus, OpenSearch и Grafana остаются в томах
docker compose down -v     # удалить вместе с томами
```

Umbrella MVP хранит данные в памяти. После перезапуска контейнера `umbrella` выполните `./configure.sh` ещё раз: он заново создаст CMDB, коннекторы, каналы и `lab-owner`. Пользователи `admin` и `grafana` создаются при старте из `.env`.

PagerDuty: чтобы отправлять инциденты по-настоящему, укажите `UMBRELLA_PD_ROUTING_KEY` в `.env` и выполните `docker compose up -d`. Без ключа шлюз работает в режиме dry-run.

Зеркала образов: если Docker Hub недоступен или упирается в лимит загрузок, задайте в `.env` образы из своего реестра:

- `PROMETHEUS_IMAGE`, `ALERTMANAGER_IMAGE`, `NODE_EXPORTER_IMAGE`;
- `OPENSEARCH_IMAGE`, `OSD_IMAGE`;
- `GRAFANA_IMAGE`, `SINK_IMAGE`, `POSTGRES_IMAGE`.

Значения по умолчанию указаны в `docker-compose.yml`.
