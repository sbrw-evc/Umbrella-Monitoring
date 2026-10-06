# Umbrella — зонтичный мониторинг

Umbrella — веб-приложение зонтичного мониторинга: принимает события из систем мониторинга и облаков через low-code конструктор коннекторов, подтверждает получение источнику, нормализует и схлопывает дубли в инциденты по КЕ и сигналу, ведёт каталог КЕ и бизнес-сервисов с картой CMDB, формирует алерты по правилам RED и USE и передаёт инциденты в PagerDuty. Если PagerDuty не принял инцидент, ответственных оповещают резервные каналы (почта и Telegram). Из инцидента по ссылке открывается дашборд контекста в Grafana с переменными инцидента. Доступ к страницам и функциям настраивается ролями; роли и команды можно выдавать по группам корпоративного каталога (LDAP / AD или Microsoft Entra ID).

Код приложения (Go и React) лежит в [app](app/README.md): там описано, что уже работает, как запустить, и есть [скриншоты](app/README.md#скриншоты).

![Инциденты](app/docs/screens/incidents-light-ru.png)

Снимок сделан в одной из прежних версий интерфейса.

## Окружение для стенда

В [deploy](deploy) — отдельные наборы Docker Compose, которые подключаются к общей сети `umbrella`. Каждый запускается из своей папки командой `docker compose up -d` после копирования `.env.example` в `.env`:

| Папка | Что поднимает |
| --- | --- |
| [deploy/umbrella](deploy/umbrella) | само приложение: образ собирается из `app`, данные в томе `/data`, порт `8080` |
| [deploy/postgres](deploy/postgres) | PostgreSQL 17 с `pg_stat_statements` (база `umbrella`) и pgAdmin |
| [deploy/openbao](deploy/openbao) | OpenBao 2.7 с файловым хранилищем; контейнер инициализации распечатывает его, включает KV v2 `umbrella/` и AppRole `umbrella` и кладёт `role_id` и `secret_id` в `secrets/umbrella` |
| [deploy/ad](deploy/ad) | контроллер домена Samba AD (LDAP 389, LDAPS 636) с тестовыми пользователями, служебной учётной записью `svc-umbrella` и группами Umbrella, плюс LDAP Account Manager |
| [deploy/zabbix](deploy/zabbix/README.md) | Zabbix 7.0 LTS (сервер, веб-интерфейс, PostgreSQL, агент 2) и способ оповещения Umbrella |
| [deploy/netbox](deploy/netbox/README.md) | NetBox 4.7 с фоновым обработчиком, PostgreSQL и Valkey; при первом запуске создаются администратор и API-токен |
| [deploy/grafana](deploy/grafana/README.md) | Grafana 13 с точкой контакта «Umbrella» и дашбордом контекста инцидента |

Как подключить Zabbix, NetBox и Grafana к Umbrella, описано в README их папок. Скрипта, который ставит весь стенд одной командой, в репозитории нет.

## Архитектурная документация

Документы архитектуры (MVP и целевая архитектура) и схемы к ним хранятся отдельно от этого репозитория. Здесь лежат документация приложения ([app/README.md](app/README.md) и [app/docs](app/docs)) и разбор возможностей n8n для конструктора коннекторов ([n8n.md](n8n.md)).
