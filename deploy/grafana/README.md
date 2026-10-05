# Grafana

Grafana 13 в общей сети `umbrella` с готовой точкой контакта «Umbrella» и дашбордом контекста инцидента.

```sh
cp .env.example .env   # задайте GRAFANA_ADMIN_PASSWORD и UMBRELLA_INGEST_TOKEN
docker compose up -d
```

Интерфейс: http://localhost:3000, вход `admin` и пароль из `.env`. Если Grafana открывают по другому адресу, задайте его в `GRAFANA_ROOT_URL`: из него строятся ссылки в оповещениях.

## Алерты Grafana → Umbrella

1. В Umbrella: **Автоматизация → Учётные данные** — создайте Bearer-токен. **Коннекторы** — создайте коннектор из пресета «Grafana Alerting», выберите токен в узле webhook и опубликуйте.
2. В `.env` Grafana: `UMBRELLA_INGEST_URL` — адрес приёма коннектора (`http://umbrella:8080/api/ingest/<slug>`, если Umbrella запущена из `deploy/umbrella` на этом хосте), `UMBRELLA_INGEST_TOKEN` — токен. Затем `docker compose up -d`.
3. Точка контакта «Umbrella» создаётся из [`provisioning/alerting/umbrella.yaml`](provisioning/alerting/umbrella.yaml) с заголовком `Authorization: Bearer <токен>`. Выберите её в **Alerting → Notification policies** (политика по умолчанию или вложенная). Проверить доставку можно кнопкой **Test** в **Alerting → Contact points**.

Пресет берёт важность из метки `severity` (или `priority`), КЕ — из `instance`, `host` или `service`, значение — из `valueString`. Задайте эти метки в правилах алертов.

## Контекст инцидента

Дашборд **Umbrella → Umbrella incident** принимает переменные, которые Umbrella подставляет в ссылку из инцидента: `incident`, `ci`, `ci_id`, `service`, `team`, `signal`. Скопируйте его, добавьте свои панели с фильтром по этим переменным и укажите адрес копии в Umbrella: **Настройки → Оповещения → Grafana** (например, `http://localhost:3000/d/umbrella-incident/umbrella-incident`).
