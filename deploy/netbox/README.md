# NetBox

NetBox 4.7 (приложение, фоновый обработчик задач, PostgreSQL 17 и Valkey) в общей сети `umbrella`. При первом запуске создаются администратор и API-токен для Umbrella.

```sh
cp .env.example .env   # задайте пароли, NETBOX_SECRET_KEY, NETBOX_API_TOKEN_PEPPER, NETBOX_API_KEY и NETBOX_API_TOKEN
docker compose up -d
```

Первый запуск с миграциями занимает несколько минут. Интерфейс: http://localhost:8000, вход по `NETBOX_ADMIN_USER` и `NETBOX_ADMIN_PASSWORD`.

Ключи для `.env`:

```sh
openssl rand -base64 48   # NETBOX_SECRET_KEY, NETBOX_API_TOKEN_PEPPER (не короче 50 символов)
openssl rand -hex 6       # NETBOX_API_KEY
openssl rand -hex 20      # NETBOX_API_TOKEN
```

Администратор и токен создаются только при первом запуске. Чтобы сменить токен позже, создайте новый в **Admin → API Tokens** (версия v2) или удалите тома: `docker compose down -v`. `NETBOX_API_TOKEN_PEPPER` после запуска не меняйте: с другим значением прежние токены перестают действовать.

## Подключение к Umbrella

В Umbrella: **Автоматизация → NetBox**:

- адрес — `http://netbox:8080`, если Umbrella запущена из `deploy/umbrella` на этом хосте, иначе `http://<хост>:8000`;
- API-токен — `nbt_<NETBOX_API_KEY>.<NETBOX_API_TOKEN>`, например `nbt_0123456789ab.0123456789abcdef0123456789abcdef01234567`. Umbrella отправляет его как `Authorization: Bearer` и хранит в OpenBao.

Включите интеграцию и сохраните (подключение проверяется перед сохранением), затем нажмите **Синхронизировать**. Токен администратора даёт и запись, которая нужна для регистрации КЕ в NetBox; для одного только чтения создайте отдельного пользователя и токен без права записи.
