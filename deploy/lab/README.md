# Стенд Umbrella

Стенд поднимает на одном сервере Umbrella и полноценное окружение мониторинга вокруг неё. Всё ставится одной командой из готовых шаблонов конфигураций, связывается через API и проверяется тестом.

```text
                         ┌───────────── OpenBao (KV v2, AppRole, аудит) ─────────────┐
                         │ пароли, токены, ключи PagerDuty, вебхуки — только здесь  │
                         └──────────────────────────────┬─────────────────────────────┘
Zabbix agent 2 ─► Zabbix server ─ webhook ───────────────┤
node-exporter ┐                                          │
cAdvisor ─────┴► Prometheus ─► Alertmanager ─ webhook ───┤
                    └─ PromQL (правила RED/USE) ─────────┤
PostgreSQL (БД Zabbix) ─ журнал ┐                        ├─► Umbrella ─► PagerDuty / Teams / Zoom
логи всех контейнеров ──────────┴► Telegraf ─► Kafka ─► Fluentd ─► OpenSearch ─ опрос ─┤
NetBox (устройства, ВМ, контакты) ─ API: КЕ и ответственные ──────────────────────────┘
                                                Grafana: дашборд инцидентов, метрики, логи
```

| Слой | Компоненты |
| --- | --- |
| Секреты | OpenBao 2.7 (Raft, Шамир 3 из 5, AppRole для Umbrella, аудит в журнал контейнера) |
| Метрики инфраструктуры | Zabbix 7.0 (сервер, веб, PostgreSQL, agent 2) |
| Метрики хоста и контейнеров | Prometheus, Alertmanager, node-exporter (хост), cAdvisor (контейнеры Docker) |
| Логи | Telegraf (журнал PostgreSQL Zabbix и логи всех контейнеров) → Kafka 4.2 (KRaft) → Fluentd (разбор) → OpenSearch 2.19 + Dashboards |
| Инвентарь | NetBox 4.4 (PostgreSQL, Valkey): устройство-хост, ВМ для каждого сервиса, ответственные |
| Визуализация | Grafana 12: дашборд инцидентов Umbrella, источники Prometheus, Umbrella API, логи PostgreSQL и контейнеров |
| Umbrella | приложение; состояние в PostgreSQL `umbrella-db` (выбирается мастером настройки), секреты в OpenBao |

node-exporter собирает метрики самого хоста; метрики контейнеров Docker отдаёт cAdvisor (node-exporter их не собирает).

Zabbix agent 2 работает в контейнере, но читает `/proc/meminfo`, `/proc/stat`, `/proc/loadavg`, `/proc/uptime`, `/proc/swaps`, `/proc/diskstats` и `/proc/cpuinfo` хоста. В LXC-контейнере Proxmox это значения самого LXC (lxcfs), а не всего узла; smoke-test сверяет память в Zabbix и node-exporter. Файлы `/etc/*`, которые Docker монтирует в контейнер, исключены из обнаружения файловых систем. Узел «Zabbix server» мониторит сам сервер Zabbix (шаблон «Zabbix server health») без шаблона агента.

### Docker в LXC (Proxmox)

AppArmor в ядре включён, а LXC не даёт Docker загрузить профиль `docker-default`: ни один контейнер не стартует. `preflight.sh` распознаёт этот случай. В `/etc/pve/lxc/<id>.conf` на узле Proxmox нужны `features: nesting=1,keyctl=1`, `lxc.apparmor.profile: unconfined` и `lxc.mount.entry: /dev/null sys/module/apparmor/parameters/enabled none bind 0 0`, затем `pct reboot <id>`.

## Установка одной командой

Сервер Debian или Ubuntu: 4 vCPU, 10 ГБ памяти (минимум 8 ГБ), 40 ГБ диска, доступ в интернет. Под root:

```bash
curl -fsSL https://raw.githubusercontent.com/sbrw-evc/Umbrella-Monitoring/feature/mvp-app/deploy/lab/install.sh | bash
```

или из клона:

```bash
git clone -b feature/mvp-app https://github.com/sbrw-evc/Umbrella-Monitoring.git /opt/umbrella-monitoring
/opt/umbrella-monitoring/deploy/lab/install.sh
```

Windows: Docker Desktop (Linux-контейнеры, 12 ГБ памяти для WSL2) и git, затем в PowerShell:

```powershell
irm https://raw.githubusercontent.com/sbrw-evc/Umbrella-Monitoring/feature/mvp-app/deploy/lab/install.ps1 -OutFile $env:TEMP\umbrella-install.ps1; powershell -ExecutionPolicy Bypass -File $env:TEMP\umbrella-install.ps1
```

PowerShell-скрипты работают в режиме ConstrainedLanguage. Настройка и тест (`configure.sh`, `smoke-test.sh`) выполняются в контейнере `toolbox` той же логикой, что в Linux, поэтому `configure.ps1` и `smoke-test.ps1` только запускают их. Если контейнер `opensearch` останавливается, выполните `wsl -d docker-desktop sysctl -w vm.max_map_count=262144`.

Что делает `install.sh`:

1. Ставит git, curl, jq, openssl, Docker Engine с плагином compose; выставляет `vm.max_map_count` для OpenSearch.
1. Проверяет AppArmor (`preflight.sh`): включён ли он в ядре, есть ли `apparmor_parser` (без него Docker не загрузит профиль docker-default; на Debian/Ubuntu пакет `apparmor` ставится сам), не установлен ли Docker из snap (его профиль не даёт монтировать каталог стенда и `docker.sock`), запускаются ли пробные контейнеры под docker-default, привилегированный (cAdvisor), с `docker.sock` (Telegraf) и с каталогом стенда; показывает недавние отказы AppArmor для контейнеров. `install.ps1` и `update.ps1` делают те же пробы в Docker Desktop.
2. `env.sh` пишет `.env` со случайными паролями и токенами (только root). Существующий `.env` сохраняется, дописываются недостающие ключи.
3. Запускает OpenBao и контейнер `openbao-init`: инициализация (5 ключей, порог 3), распечатывание, KV v2 `umbrella/`, политика `umbrella`, AppRole `umbrella`, начальные секреты `umbrella/bootstrap`. Ключи и root-токен — в `secrets/openbao/init.txt`, role_id и secret_id Umbrella — в `secrets/umbrella/`.
4. Собирает образы Umbrella и Fluentd и запускает все контейнеры.
5. `configure.sh` проходит мастер настройки Umbrella через API (тема светлая, язык русский, хранилище PostgreSQL `umbrella-db`, пароль БД — в OpenBao) и связывает стенд (см. ниже).
6. `smoke-test.sh --all` проверяет стенд (пропустить: `SKIP_TEST=1`).

## Компоненты и порты

| Что | Адрес | Вход |
| --- | --- | --- |
| Umbrella | `http://<сервер>:8080` | `admin` / `UMBRELLA_ADMIN_PASSWORD`, `lab-owner` / `LAB_OWNER_PASSWORD` |
| Grafana | `:3000` | `admin` / `GRAFANA_ADMIN_PASSWORD` |
| Zabbix | `:8081` | `Admin` / `ZABBIX_ADMIN_PASSWORD` |
| NetBox | `:8000` | `admin` / `NETBOX_ADMIN_PASSWORD` |
| Prometheus | `:9090` | без входа |
| OpenSearch Dashboards | `:5601` | без входа |
| OpenBao | `127.0.0.1:8200` (только сам сервер) | root-токен из `secrets/openbao/init.txt` |
| Alertmanager, Kafka, OpenSearch, Fluentd, Telegraf, cAdvisor, node-exporter, umbrella-db | только внутри сети стенда | — |

Порты меняются в `.env`. У Prometheus и OpenSearch Dashboards нет входа: откройте порты только своим адресам (`ufw allow from <IP> to any port 8080,8081,8000,3000,9090,5601 proto tcp`).

## Секреты и OpenBao

Umbrella не хранит секретов у себя: всё, что вводится в интерфейсе или передаётся через API (пароли и токены систем мониторинга, ключи PagerDuty, адреса вебхуков Teams и Zoom), сразу записывается в OpenBao, а в приложении остаётся ссылка `openbao://umbrella/<путь>#<ключ>`. Umbrella входит по AppRole (token TTL 1 ч, продление автоматически), политика даёт доступ только к `umbrella/*`.

Пароль администратора, токен Grafana и токен `/metrics` Umbrella при старте читает из `umbrella/bootstrap` (в `docker-compose.yml` переменные — ссылки `openbao://…`). Скрипт кладёт их туда из `.env` один раз; дальше источник истины — OpenBao.

На стенде контейнер `openbao-init` после перезапуска сервера сам распечатывает OpenBao ключами из `secrets/openbao/init.txt`. В продуктиве так не делают: распечатывание через auto-unseal (transit или KMS) либо ключи у разных хранителей, root-токен отзывается после настройки (`bao token revoke`).

Посмотреть секреты Umbrella:

```bash
export BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN=$(awk '/Initial Root Token/ {print $NF}' secrets/openbao/init.txt)
docker compose exec -e BAO_ADDR=http://127.0.0.1:8200 -e BAO_TOKEN openbao bao kv list umbrella/
```

## Как стенд связан с Umbrella

`configure.sh` ничего не пишет в конфигурационные файлы Umbrella: всё задаётся через её API — так же, как в разделе «Интеграции» веб-интерфейса.

| Интеграция Umbrella | Что делает |
| --- | --- |
| Zabbix | вход в API (Admin), автонастройка: тип оповещения webhook «Umbrella», медиа пользователя, действие для проблем и восстановлений; загрузка узлов как КЕ |
| Prometheus Alertmanager | приём `/api/ingest/prometheus` с Bearer-токеном (alertmanager.yml); КЕ — метка `ci`, иначе `host`, иначе `instance` |
| Prometheus | источник метрик для правил RED/USE и загрузка целей как КЕ |
| NetBox | устройства и ВМ → КЕ, контакты → ответственные (контакт объекта, иначе площадки, иначе арендатора), арендатор → команда |
| PostgreSQL logs (OpenSearch) | опрос `pg-logs-*` каждые 20 с: fatal, error, warning |
| Container logs (OpenSearch) | опрос `container-logs-*`: fatal, critical, error |

Один и тот же хост из Zabbix, Prometheus и NetBox становится одной КЕ: хост стенда называется `LAB_HOST` (по умолчанию `lab-host-1`) во всех трёх системах, сервисы — именами сервисов compose. КЕ сопоставляются по ID в источнике, затем по имени и DNS, затем по IP; NetBox задаёт название, тип, команду и ответственных.

`configure.sh` также заводит команды `monitoring`, `infra`, `dba` (коды совпадают с арендаторами NetBox) с участниками, бизнес-услуги «Мониторинг (стенд)» и «Каталог (NetBox)» с ИТ-сервисами и связями до КЕ сервисов и хоста, пользователя `lab-owner` (владелец «Мониторинг (стенд)»), ссылку на дашборд Grafana и семь правил RED/USE. Если в `.env` заданы `UMBRELLA_PD_ROUTING_KEY`, `TEAMS_WEBHOOK_URL` или `ZOOM_WEBHOOK_URL`, он подключает PagerDuty и каналы; удобнее сделать это в интерфейсе (Интеграции → PagerDuty, Уведомления).

### Логи

```text
PostgreSQL Zabbix: log_min_duration_statement=PG_SLOW_MS, префикс "%m [%p] %e %q%u@%d "
  → файл log/postgresql.log в томе БД → Telegraf (inputs.tail, многострочные записи)
логи контейнеров → Telegraf (inputs.docker_log, метка com.docker.compose.service)
  → Kafka: logs.postgres_log, logs.docker_log
  → Fluentd: уровень, SQLSTATE, длительность, сервис → OpenSearch pg-logs-*, container-logs-*
  → Umbrella: событие на КЕ сервиса, сигнал opensearch:<сервис>:<код>
```

Коды: `SQLSTATE_<код>` для ошибок PostgreSQL, `SLOW_QUERY` для запросов дольше `PG_SLOW_MS`, `LOG_<УРОВЕНЬ>` для контейнеров. В Grafana логи смотрят через источники «PostgreSQL logs» и «Container logs», в OpenSearch Dashboards — через index pattern `pg-logs*` и `container-logs*`.

### Низкие пороги

Пороги занижены, чтобы алерты на стенде срабатывали часто.

| Где | Сигнал | Порог |
| --- | --- | --- |
| Zabbix (макросы хоста) | CPU, load на ядро, память, заполнение ФС | 3 %, 0,1, 25 %, 5 % / 10 % |
| Prometheus | CPU хоста, load на ядро, память хоста | 5 % 1 мин, 0,2, 40 % |
| Prometheus | CPU и память контейнера, перезапуск | 3 % 1 мин, 300 МБ, любой |
| Umbrella RED/USE | CPU и память хоста, насыщение, CPU и память контейнера | 2 %, 30 %, 0,1, 2 %, 200 МБ |
| Umbrella RED | поток и p99 запросов Prometheus | < 5 в секунду, > 20 мс |
| PostgreSQL | медленный запрос | `PG_SLOW_MS`, 20 мс |

## Обновление без пересоздания

```bash
./update.sh               # новый код → новый образ Umbrella → замена только контейнера umbrella
./update.sh --all         # плюс новые образы и изменённые конфигурации всех сервисов
./update.sh --no-pull     # собрать из текущих файлов
./update.sh --configure   # и повторить настройку связей (идемпотентно)
./update.sh --test        # и прогнать smoke-test
./update.sh --rollback    # вернуть предыдущий образ Umbrella
.\update.ps1 [-NoPull] [-All] [-Configure] [-Test] [-Rollback]
```

Обновление ничего не удаляет. Состояние Umbrella (КЕ, инциденты, коннекторы, интеграции, правила, команды, пользователи) хранится в PostgreSQL `umbrella-db` и сохраняется каждые 2 секунды и при остановке; настройки подключения и сессии — в томе `umbrella-data`, секреты — в OpenBao. Новый образ собирается, пока работает старый; затем пересоздаётся только контейнер `umbrella`, остальные сервисы не перезапускаются. Если новая версия не прошла проверку здоровья, скрипт сам возвращает предыдущий образ. Версия видна в заголовке `X-Umbrella-Version` ответа `/healthz` и на странице «Самоконтроль».

## Тест

```bash
./smoke-test.sh             # Umbrella: OpenBao, доступ, секреты, интеграции, CMDB, инциденты, правила, области
./smoke-test.sh --zabbix    # плюс Zabbix → Umbrella
./smoke-test.sh --stack     # плюс Prometheus/Alertmanager, логи PostgreSQL и контейнеров, Grafana, NetBox
./smoke-test.sh --restart   # плюс перезапуск контейнера umbrella: данные и сессии сохраняются
./smoke-test.sh --all       # --zabbix --stack
.\smoke-test.ps1 -All
```

| Раздел | Что проверяется |
| --- | --- |
| Сервис | версия в `/healthz`, вход паролем из OpenBao, состояние OpenBao (распечатан, AppRole, политика), мастер завершён и не запускается повторно, тема и язык по умолчанию, состояние в PostgreSQL, `/metrics` с токеном |
| Доступ | 401 без входа, 403 без прав, дежурный не меняет настройки PagerDuty |
| Секреты | секреты интеграций — ссылки `openbao://`, ни один пароль или токен не возвращается API |
| Интеграции | проверка подключения Zabbix, Alertmanager, Prometheus, NetBox, двух индексов OpenSearch; загрузка КЕ из NetBox |
| CMDB и команды | у хоста идентификаторы Zabbix, Prometheus и NetBox, ответственные из NetBox (объект, арендатор), создание, изменение и удаление КЕ и связей; команды стенда с участниками и КЕ, создание, изменение и удаление команды, отказ удалять используемую команду |
| Инциденты | токен приёма из OpenBao, inbox, схлопывание, рост важности, подтверждение, решение источником, состояние PagerDuty, окно обслуживания, ошибки разбора |
| Правила | правила стенда вычисляются без ошибок и срабатывают, проверка на данных, срабатывание → инцидент, удаление правила → решение |
| Области | `lab-owner` видит инциденты своей услуги и не видит чужие |
| Zabbix | автонастройка в Zabbix, агент доступен, проблема → инцидент, восстановление → решение, срабатывания шаблона с низкими порогами |
| Prometheus | цели node, cAdvisor, umbrella, alertmanager; метрики контейнеров; textfile-метрика → Alertmanager → инцидент → решение; срабатывания низких порогов |
| Логи | ошибка SQL в БД Zabbix → OpenSearch (разобрана Fluentd) → инцидент на КЕ zabbix-db; поток журнала; логи нескольких контейнеров с уровнями; топики Kafka |
| Grafana, NetBox | здоровье, источники данных, ссылка на дашборд, API NetBox |

После прогона тест убирает всё, что создал, даже если упал на середине: инциденты с событиями, ошибки разбора, интеграцию «Smoke webhook» с коннектором и секретами в OpenBao, пользователей, команды, правила, КЕ (в том числе созданные из тестовых событий), окна обслуживания, тестовые строки в OpenSearch и файл метрики. Остаются только записи журнала аудита.

Срабатывания шаблона Zabbix появляются через 5–6 минут после установки (триггеры смотрят на 5 минут данных).

## Управление

```bash
cd /opt/umbrella-monitoring/deploy/lab
docker compose ps
docker compose logs -f umbrella
docker compose logs -f fluentd telegraf
docker compose down          # остановить; тома и секреты остаются
docker compose up -d         # запустить снова (OpenBao распечатается сам)
```

`docker compose down -v` удаляет тома вместе с данными Umbrella (umbrella-db, umbrella-data), OpenBao, Zabbix и NetBox. После этого удалите и `secrets/`, иначе `openbao-init` откажется инициализировать пустой OpenBao поверх старых ключей.

Зеркала образов задаются в `.env`: `OPENBAO_IMAGE`, `POSTGRES_IMAGE`, `ZABBIX_SERVER_IMAGE`, `ZABBIX_WEB_IMAGE`, `ZABBIX_AGENT_IMAGE`, `PROMETHEUS_IMAGE`, `ALERTMANAGER_IMAGE`, `NODE_EXPORTER_IMAGE`, `CADVISOR_IMAGE`, `KAFKA_IMAGE`, `TELEGRAF_IMAGE`, `OPENSEARCH_IMAGE`, `OSD_IMAGE`, `NETBOX_IMAGE`, `VALKEY_IMAGE`, `GRAFANA_IMAGE`.

### Файлы стенда

| Файл | Назначение |
| --- | --- |
| `docker-compose.yml` | все сервисы, тома, сеть |
| `env.sh`, `.env.example` | генерация `.env`, список ключей |
| `openbao/openbao.hcl`, `openbao/umbrella-policy.hcl`, `openbao/init.sh` | сервер OpenBao, политика Umbrella, инициализация и распечатывание |
| `prometheus/prometheus.yml`, `prometheus/rules/lab.yml`, `alertmanager/alertmanager.yml` | сбор метрик, правила с низкими порогами, отправка в Umbrella |
| `telegraf/telegraf.conf`, `fluentd/Dockerfile`, `fluentd/fluent.conf` | сбор логов в Kafka, разбор и запись в OpenSearch |
| `preflight.sh` | проверка AppArmor и Docker перед установкой и обновлением |
| `toolbox/Dockerfile` | bash, curl, jq, psql для запуска скриптов в контейнере |
| `install.sh`, `update.sh`, `configure.sh`, `smoke-test.sh`, `lib.sh` | установка, обновление, связывание, тест |
| `install.ps1`, `update.ps1`, `configure.ps1`, `smoke-test.ps1`, `lib.ps1` | то же для Windows |
