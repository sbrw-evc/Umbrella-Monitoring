# =====================================================================
# v3: компонентные схемы всех контейнеров (C4 уровень 3)
# =====================================================================
REPLACED = {"cmp"}
pages[:] = [pg for pg in pages if pg.pid not in REPLACED]
TGT = not MVP
CW3, CH3 = 210, 104
GRP = NODE_BOUND + "dashed=1;dashPattern=4 4;"
SUF = "" if MVP else ""


def cp(pid, name, title):
    p = Page(name, pid)
    pages.append(p)
    p.text(f"<b>{title}</b>", 40, 20, 1500, 30, TITLE)
    return p


def comp(p, key, name, desc, x, y, w=CW3, h=CH3):
    p.node(key, c4(name, "Component", desc), x, y, w, h, COMP)


def cont(p, key, name, kind, desc, x, y, w=200, h=100, st=CONT):
    p.node(key, c4(name, kind, desc), x, y, w, h, st)


def ext(p, key, name, desc, x, y, w=200, h=90, kind="External System"):
    p.node(key, c4(name, kind, desc), x, y, w, h, EXT)


def bao(p, key, desc, x, y, w=220, h=110):
    """Хранилище секретов: единый вид на всех схемах."""
    p.node(key, c4("Хранилище секретов", "Container: OpenBao ×3, Raft", desc), x, y, w, h, DB)


def fy(p, key, y):
    """Доля высоты узла для точки входа или выхода на уровне y."""
    _, ny, _, nh = p.geo[key]
    return round((y - ny) / nh, 4)


def fx(p, key, x):
    nx, _, nw, _ = p.geo[key]
    return round((x - nx) / nw, 4)


# ---------------------------------------------------------------------
# Веб-интерфейс
# ---------------------------------------------------------------------
p = cp("cweb", "C4-3 Веб-интерфейс", "C4 · Уровень 3 · Компоненты веб-интерфейса (React, TypeScript, React Flow)")
V = [("dash", "Дашборд инцидентов",
      ("Таблица и карточки, счётчики по severity, фильтры, полнотекстовый поиск, сохранённые представления, предустановки группы, "
       "живые обновления; клик → Grafana" if MVP else
       "Инциденты от Correlation Engine: фильтры, поиск (и по истории), сохранённые представления, предустановки группы, "
       "живые обновления; клик → Grafana")),
     ("side", "Боковая панель инцидента «i»", "События, хронология, статус в PagerDuty, журнал резервных отправок; ack и silence по правам"),
     ("map", "Карта CMDB", "Граф КЕ и сервисов со статусами; ревью изменений карты"),
     ("builder", "Конструктор коннекторов", "Холст React Flow, палитра блоков, dry-run на живых данных"),
     ("editors", "Редакторы шаблонов и правил", "Шаблоны источников и событий, RED/USE, дедупликация; CEL, JSONata"),
     ("ctxl", "Редактор «Связи контекста»", "Low-code редактор правил для Grafana: условие → обход карты → панели; dry-run на инциденте"),
     ("maint", "Окна и маршрутизация", "Окна обслуживания, маршруты в PagerDuty, резервные контакты и каналы"),
     ("roles", "Роли и предустановки", "Привязка групп каталога к ролям, областям и предустановкам дашборда; два ключа"),
     ("admin", "Администрирование", "Пользователи, команды, журнал аудита, ошибки разбора, самоконтроль")]
if TGT:
    V += [("rep", "Отчёты и аналитика", "История, MTTA и MTTR, шумные источники, покрытие RED/USE"),
          ("itsm", "Интеграции ITSM", "Статус синхронизации с корпоративной CMDB и ITSM")]
NR = (len(V) + 2) // 3
GX, GY, VW, VH = 420, 110, 230, 120
p.node("grp", "<b>Представления</b>", GX, GY, 770, NR * 140 + 30, GRP)
p.node("bnd", "<b>Веб-интерфейс</b> [Container: SPA на React, раздаётся nginx]", 400, 90, 1130, NR * 140 + 90, BOUND)
for i, (k, n, d) in enumerate(V):
    comp(p, k, n, d, GX + 20 + (i % 3) * 250, GY + 40 + (i // 3) * 140, VW, VH)
comp(p, "shell", "App Shell", "Вход через OIDC (PKCE), обновление токена, скрытие разделов по разрешениям роли", 1290, 130, 220, 112)
comp(p, "client", "API Client", "REST по OpenAPI и подписки WebSocket; фильтры дашборда в URL; повтор при обрыве", 1290, 300, 220, 112)
ext(p, "graf", "Grafana", "дашборд контекста инцидента; вход через корпоративный IdP", 40, 155, 200, 110)
p.node("duty", c4("Дежурный инженер", "Person", "дашборд, разбор инцидента"), 40, 320, 200, 90, PERSON)
p.node("eng", c4("Инженер мониторинга", "Person", "настройка"), 40, 470, 200, 90, PERSON)
ext(p, "idp", "Корпоративный IdP", "OIDC", 1640, 130, 200, 112)
cont(p, "api", "Core API", "Container", "REST /api/v1, WebSocket /ws, ссылки /go/", 1640, 300, 200, 112)
p.edge("dash", "graf", "новая вкладка<br>(redirect из /go/…)", exit=(0, 0.5), entry=(1, 0.5))
p.edge("duty", "grp", "HTTPS 443", exit=(1, 0.5), entry=(0, fy(p, "grp", 365)))
p.edge("eng", "grp", "HTTPS 443", exit=(1, 0.5), entry=(0, fy(p, "grp", 515)))
p.edge("grp", "client", "вызовы API", exit=(1, fy(p, "grp", 356)), entry=(0, 0.5))
p.edge("shell", "idp", "OIDC")
p.edge("client", "api", "REST,<br>WebSocket")
p.edge("shell", "client", "токен", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.text("Веб-интерфейс не хранит данных и секретов: всё идёт через Core API с токеном пользователя. Разделы и действия, на которые у роли нет "
       "разрешений, скрыты, но проверка прав и области всегда выполняется на стороне Core API. Клик по инциденту открывает в новой вкладке "
       "/go/incidents/{id}/grafana: Core API готовит дашборд контекста и перенаправляет браузер в Grafana.",
       40, 90 + NR * 140 + 90 + 30, 1680, 60)
p.legend(labels={NODE_BOUND: "Группа компонентов"})

# ---------------------------------------------------------------------
# Core API
# ---------------------------------------------------------------------
p = cp("capi", "C4-3 Core API", "C4 · Уровень 3 · Компоненты Core API (Go, REST + WebSocket)")
C, R, AH = [320, 560, 800, 1040], [130, 290, 450, 610], 120
p.node("bnd", "<b>Core API</b> [Container: Go]", 300, 90, 980, 680, BOUND)
p.node("grp", "<b>Сервисы домена</b>", 540, 260, 730, 490, GRP)
comp(p, "rest", "REST Handlers", "OpenAPI /api/v1: валидация запросов, версии API; ссылки /go/…", C[0], R[0], h=AH)
comp(p, "authn", "AuthN", "Проверка OIDC-токена, группы из claims при входе; сервисные токены с TTL", C[1], R[0], h=AH)
comp(p, "rbac", "AuthZ / RBAC Policy Engine", "Роли, разрешения на функции, области; объединение по группам; права себе не выдаются", C[2], R[0], h=AH)
comp(p, "dsync", "Directory Sync", "Группы каталога UMB-*: LDAPS pull раз в 15 мин или SCIM 2.0 push; отзыв сессий и токенов", C[3], R[0], h=AH)
comp(p, "audit", "Audit Logger", "Неизменяемый журнал всех изменений и действий; выгрузка в SIEM", C[0], R[1], h=AH)
comp(p, "ws", "WebSocket Hub", "Живые обновления дашборда: alerts.changed, pd.delivery" + ("" if MVP else ", incidents.changed"), C[0], R[2], h=AH)
comp(p, "pres", "Presets Service", "Предустановки дашборда по группам, сохранённые представления; версии", C[1], R[1], h=AH)
comp(p, "srch", "Incident Query & Search",
     "Фильтры, полнотекстовый поиск (FTS и триграммы PostgreSQL), экспорт CSV" if MVP else
     "Фильтры, полнотекстовый поиск, экспорт CSV; история через History Service", C[2], R[1], h=AH)
comp(p, "ctxl", "Context Link", "GET /go/incidents/{id}/grafana → context.request → redirect в Grafana", C[3], R[1], h=AH)
comp(p, "cfg", "Config Service", "Коннекторы, шаблоны, правила, связи контекста: версии, публикация, откат; два ключа", C[1], R[2], h=AH)
comp(p, "dry", "Dry-run Runner", "Прогон черновика на примере через NATS request/reply в песочнице", C[2], R[2], h=AH)
comp(p, "act", "Alert Actions",
     "ack, silence, комментарии → alerts.commands; проверка одноразового ack-токена, вход через IdP" if MVP else
     "ack, silence, массовые действия → alerts.commands; одноразовые ack-ссылки, вход через IdP", C[3], R[2], h=AH)
comp(p, "mapq", "CMDB и РСМ", "Чтение карты, ревью изменений, бизнес-услуги, резервные контакты", C[1], R[3], h=AH)
comp(p, "sec", "Secrets Broker", "Кладёт введённые логины и токены в хранилище секретов, в БД пишет только ссылку", C[3], R[3], h=AH)
if TGT:
    comp(p, "rep", "Reports Proxy", "Запросы отчётов к History Service с проверкой прав и области", C[2], R[3], h=AH)
cont(p, "web", "Веб-интерфейс", "Container", "SPA", 40, 135, 160, 110)
ext(p, "siem", "SIEM", "журнал аудита", 40, 300, 160, 100)
ext(p, "idp", "Корпоративный IdP и каталог (AD)", "OIDC, LDAPS 636, SCIM 2.0", 1390, 130, 220, 120)
p.node("pg", c4("PostgreSQL", "Container", "конфигурация, CMDB, роли, предустановки, аудит"), 1390, 290, 220, 120, DB)
bao(p, "bao", "логины, токены, ключи коннекторов", 1390, 610, 220, 120)
p.node("nats", c4("NATS JetStream", "Container", "alerts.commands, alerts.changed, pd.delivery, connector.query, context.request"
                  + ("" if MVP else ", incidents.changed")), 540, 830, 560, 70, BUS)
cont(p, "cb", "Context Builder", "Container", "дашборд инцидента в Grafana", 1390, 820, 220, 90)
if TGT:
    cont(p, "hist", "History Service", "Container", "отчёты, поиск по истории", 1390, 450, 220, 100)
    p.edge("grp", "hist", "REST: отчёты,<br>история", exit=(1, fy(p, "grp", 500)), entry=(0, 0.5))
p.edge("web", "rest", "REST", exit=(1, 0.5), entry=(0, 0.5))
p.edge("web", "ws", "WebSocket /ws", EDGE_ORTHO, points=[(20, 190), (20, 510)], exit=(0, 0.5), entry=(0, 0.5))
p.edge("rest", "authn")
p.edge("authn", "rbac")
p.edge("dsync", "rbac", "", EDGE_DASH, exit=(0, 0.5), entry=(1, 0.5))
p.edge("authn", "idp", "OIDC", EDGE_ORTHO, points=[(665, 110), (1500, 110)], exit=(0.5, 0), entry=(0.5, 0))
p.edge("dsync", "idp", "LDAPS 636,<br>раз в 15 мин", exit=(1, 0.3), entry=(0, 0.3))
p.edge("idp", "dsync", "SCIM 2.0 push,<br>HTTPS 443", EDGE_DASH, exit=(0, 0.75), entry=(1, 0.75))
p.edge("rbac", "grp", "", exit=(0.5, 1), entry=(fx(p, "grp", 905), 0))
p.edge("grp", "audit", "", exit=(0, fy(p, "grp", 350)), entry=(1, 0.5))
p.edge("audit", "siem", "syslog TLS<br>6514", exit=(0, 0.5), entry=(1, 0.5))
p.edge("grp", "pg", "SQL", exit=(1, fy(p, "grp", 350)), entry=(0, 0.5))
p.edge("sec", "bao", "секрет; в БД<br>только ссылка", exit=(1, 0.5), entry=(0, 0.5))
p.edge("grp", "nats", "alerts.commands, context.request, connector.query", exit=(fx(p, "grp", 700), 1), entry=(fx(p, "nats", 700), 0))
p.edge("nats", "ws", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(505, 865)], exit=(0, 0.5), entry=(fx(p, "ws", 505), 1))
p.text("alerts.changed,<br>pd.delivery" if MVP else "alerts.changed,<br>pd.delivery,<br>incidents.changed", 398, 640, 100, 50,
       NOTE.replace("fontSize=12", "fontSize=10") + "align=right;")
p.edge("nats", "cb", "context.request<br>(request/reply)", exit=(1, 0.5), entry=(0, 0.5))
p.text("Каждый запрос проходит AuthN и RBAC Policy Engine (разрешение на функцию и область данных); каждое изменение пишется в аудит до ответа. "
       "Права выдаются через группы каталога: Directory Sync забирает группы по LDAPS раз в 15 минут или принимает SCIM; удаление из группы "
       "отзывает доступ, сессии и токены при следующей синхронизации. Context Link по клику на инцидент запрашивает Context Builder через "
       "context.request и перенаправляет браузер в Grafana. Secrets Broker сразу передаёт секреты в хранилище секретов и возвращает ссылку.",
       40, 950, 1570, 60)
p.legend(labels={NODE_BOUND: "Группа компонентов"})

# ---------------------------------------------------------------------
# Ingest Gateway
# ---------------------------------------------------------------------
p = cp("cing", "C4-3 Ingest Gateway", "C4 · Уровень 3 · Компоненты Ingest Gateway: приём push-событий")
p.node("bnd", "<b>Ingest Gateway</b> [Container: Go]", 280, 175, 1010, 365, BOUND)
comp(p, "lst", "HTTP Listener", "POST /ingest/{connector}; за ingress и WAF; HTTPS 8443 mTLS от ingress", 300, 240)
comp(p, "guard", "Size & Format Guard", "Лимит тела 1 МБ, content-type, пачки до 1000 событий", 540, 240)
comp(p, "auth", "Auth Verifier", "Токен или HMAC коннектора, подпись облака; ключи из хранилища секретов", 780, 240)
comp(p, "rate", "Rate Limiter", "Лимиты на коннектор и IP; при превышении 429", 1040, 240, 230)
comp(p, "pub", "Publisher", "Пишет в ingest.in и ждёт PubAck от JetStream", 1040, 400, 230)
comp(p, "resp", "Responder", "2xx только после PubAck; 503 при недоступной шине, источник повторит", 300, 400)
ext(p, "src", "Push-источники", "webhook систем мониторинга и облаков", 40, 242, 180, 100)
p.node("pg", c4("PostgreSQL", "Container", "реестр коннекторов (кеш)"), 535, 60, 220, 90, DB)
bao(p, "bao", "ключи коннекторов", 775, 55, 220, 100)
p.node("nats", c4("NATS JetStream", "Container", "ingest.in"), 1380, 407, 200, 90, BUS)
p.edge("src", "lst", "HTTPS")
p.edge("lst", "guard")
p.edge("guard", "auth")
p.edge("auth", "rate")
p.edge("rate", "pub", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("pub", "nats", "ingest.in")
p.edge("pub", "resp", "PubAck")
p.edge("resp", "src", "2xx или 503", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(130, 452)], exit=(0, 0.5), entry=(0.5, 1))
p.edge("auth", "bao", "ключи", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1))
p.edge("guard", "pg", "коннектор", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1))
p.text("Ingest Gateway не разбирает события: он только проверяет, что запрос подлинный, и надёжно пишет тело в шину. "
       "Разбор и дедупликацию выполняет Connector Runtime по тому же коннектору, что и для pull-источников.", 40, 580, 1500, 40)
p.legend()

# ---------------------------------------------------------------------
# Normalizer
# ---------------------------------------------------------------------
p = cp("cnrm", "C4-3 Normalizer", "C4 · Уровень 3 · Компоненты Normalizer: шаблоны источников и единая модель события")
p.node("bnd", "<b>Normalizer</b> [Container: Go, CEL, JSONata]", 280, 90, 1010, 490, BOUND)
comp(p, "cons", "Raw Consumer", "Durable consumer events.raw; ack только после публикации результата", 300, 130)
comp(p, "sel", "Template Selector", "Опубликованная версия шаблона источника", 540, 130)
comp(p, "map", "Mapper", "Маппинг полей выражениями JSONata и CEL (в Sandbox)", 780, 130)
comp(p, "sig", "Signal Classifier", "Сигнал по справочнику, method: red, use, other", 1040, 130, 230)
comp(p, "sev", "Severity Normalizer", "critical, error, warning, info", 1040, 290, 230)
comp(p, "ci", "CI Linker", "Кандидат КЕ по меткам, FQDN, облачным ID; кеш карты CMDB", 780, 290)
comp(p, "key", "Dedup Key Builder", "Кандидат ключа = КЕ + сигнал + метки; окончательный ключ — в Alert Engine", 540, 290)
comp(p, "pub", "Norm Publisher", "events.norm без поля raw; ошибки разбора в events.dlq", 300, 290)
comp(p, "box", "Sandbox", "Выражения JSONata и CEL: лимиты времени и памяти, без сети и файлов", 1040, 450, 230)
comp(p, "cache", "CMDB Cache", "Индексы КЕ в памяти; обновляется по cmdb.changed", 780, 450)
p.node("nats", c4("NATS JetStream", "Container", "events.raw, events.norm, events.dlq, cmdb.changed"), 20, 130, 150, 424, BUS)
p.node("pg", c4("PostgreSQL", "Container", "шаблоны, справочник сигналов, КЕ"), 1340, 130, 200, 104, DB)
p.edge("nats", "cons", "events.raw", exit=(1, fy(p, "nats", 182)), entry=(0, 0.5))
p.edge("cons", "sel")
p.edge("sel", "map")
p.edge("map", "sig")
p.edge("sig", "sev", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("sev", "ci")
p.edge("ci", "key")
p.edge("key", "pub")
p.edge("pub", "nats", "events.norm,<br>events.dlq", exit=(0, 0.5), entry=(1, fy(p, "nats", 342)))
p.edge("sev", "box", "выражения", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.edge("ci", "cache", "", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.edge("nats", "cache", "cmdb.changed", EDGE_DASH, exit=(1, fy(p, "nats", 502)), entry=(0, 0.5))
p.edge("sel", "pg", "шаблоны, справочник (SQL)", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(645, 110), (1440, 110)],
       exit=(0.5, 0), entry=(0.5, 0))
p.text("Normalizer без состояния: реплики делят поток events.raw через общий consumer. Событие, которое не разобралось, не теряется: "
       "оно уходит в events.dlq с исходным телом и видно в разделе «Ошибки разбора». В events.norm поле raw не передаётся; исходное тело "
       "хранится в PostgreSQL 7 дней, чтобы шаблон можно было пересчитать задним числом.",
       40, 620, 1500, 40)
p.legend()

# ---------------------------------------------------------------------
# Rule Engine
# ---------------------------------------------------------------------
p = cp("crul", "C4-3 Rule Engine", "C4 · Уровень 3 · Компоненты Rule Engine: алерты по RED и USE")
p.node("bnd", "<b>Rule Engine</b> [Container: Go, CEL]", 300, 90, 990, 330, BOUND)
comp(p, "sch", "Rule Scheduler", "Раз в 30 с; правила делятся между репликами через lease в NATS KV", 320, 130)
comp(p, "fetch", "Metric Fetcher", "Запрос метрик через коннекторы: connector.query (request/reply)", 560, 130)
comp(p, "eval", "Evaluator", "CEL: статический порог, базовая линия, расход бюджета ошибок SLO", 800, 130)
comp(p, "state", "State Tracker", "Длительность условия, гистерезис против дребезга", 1040, 130, 230)
comp(p, "base", "Baseline Store", "Базовая линия: то же время неделю назад", 800, 290)
comp(p, "emit", "Event Emitter", "Событие с method и signal в events.raw", 1040, 290, 230)
p.node("nats", c4("NATS JetStream", "Container", "connector.query, events.raw, KV lease"), 1370, 130, 200, 264, BUS)
p.node("pg", c4("PostgreSQL", "Container", "правила, базовые линии"), 20, 130, 170, 264, DB)
cont(p, "cr", "Connector Runtime", "Container", "Query Responder: исполняет запросы к API источников", 1370, 460, 200, 110)
p.edge("sch", "fetch")
p.edge("fetch", "eval")
p.edge("eval", "state")
p.edge("state", "emit", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("eval", "base", "", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.edge("emit", "nats", "events.raw", exit=(1, 0.5), entry=(0, fy(p, "nats", 342)))
p.edge("fetch", "nats", "connector.query", EDGE_ORTHO, points=[(665, 110), (1470, 110)], exit=(0.5, 0), entry=(0.5, 0))
p.edge("nats", "cr", "запрос метрик", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.edge("sch", "pg", "правила", EDGE_DASH, exit=(0, 0.5), entry=(1, fy(p, "pg", 182)))
p.edge("base", "pg", "базовые линии (SQL)", EDGE_DASH, exit=(0, 0.5), entry=(1, fy(p, "pg", 342)))
p.text("Rule Engine сам не ходит в источники: метрики забирает Connector Runtime теми же коннекторами и секретами по connector.query. "
       "Готовые алерты систем мониторинга Rule Engine не дублирует: правило включается только там, где источник не алертит (экран «Покрытие»).",
       40, 610, 1500, 40)
p.legend()

# ---------------------------------------------------------------------
# Alert Engine
# ---------------------------------------------------------------------
p = cp("cale", "C4-3 Alert Engine", "C4 · Уровень 3 · Компоненты Alert Engine: тревоги, схлопывание, влияние")
AC = [340, 580, 820, 1060]
p.node("bnd", "<b>Alert Engine</b> [Container: Go]", 320, 90, 990, 490, BOUND)
comp(p, "cons", "Event Consumer", "events.norm, pd.inbound, alerts.commands, cmdb.changed" + ("" if MVP else ", incidents.changed"), AC[0], 130)
comp(p, "cires", "CI Resolver", "Окончательная привязка к КЕ по карте CMDB и пересчёт ключа дедупликации", AC[1], 130)
comp(p, "dedup", "Cross-source Dedup", "Один ключ из любого числа источников; окно 10 мин", AC[2], 130)
comp(p, "supp", "Suppression", "Окна обслуживания, silences, правила игнора", AC[3], 130, 230)
comp(p, "impact", "Service Impact", "Влияние на ИТ-сервисы и бизнес-услуги по РСМ", AC[3], 290, 230)
if MVP:
    comp(p, "corr", "RED/USE Link", "RED-тревога сервиса и USE-тревога его КЕ в окне 10 мин связываются; USE — вероятная причина", AC[2], 290)
else:
    comp(p, "corr", "Incident Link", "Привязка тревоги к инциденту из incidents.changed; сама не коррелирует", AC[2], 290)
comp(p, "life", "Lifecycle", "open → acknowledged → resolved → closed; resolve, когда все источники в норме", AC[1], 290)
comp(p, "wr", "State Writer", "Изменение тревоги и строка outbox в одной транзакции PostgreSQL", AC[0], 290)
comp(p, "relay", "Outbox Relay", "Читает outbox, публикует alerts.changed, отмечает отправленное", AC[0], 450)
comp(p, "sync", "Source Sync-back", "ack и silence обратно в источники: connector.command", AC[1], 450)
p.node("nats", c4("NATS JetStream", "Container", "events.norm, pd.inbound, alerts.commands, cmdb.changed, "
                  + ("" if MVP else "incidents.changed, ") + "alerts.changed, connector.command"), 1360, 130, 200, 400, BUS)
p.node("pg", c4("PostgreSQL", "Container", "тревоги, outbox, окна, РСМ"), 20, 290, 170, 220, DB)
p.edge("nats", "cons", "", EDGE_ORTHO, points=[(1460, 110), (445, 110)], exit=(0.5, 0), entry=(0.5, 0))
p.edge("cons", "cires")
p.edge("cires", "dedup")
p.edge("dedup", "supp")
p.edge("supp", "impact", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("impact", "corr")
p.edge("corr", "life")
p.edge("life", "wr")
p.edge("life", "sync", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("wr", "pg", "тревога + outbox<br>(1 транзакция)", exit=(0, 0.5), entry=(1, fy(p, "pg", 342)))
p.edge("relay", "pg", "outbox", EDGE_DASH, exit=(0, 0.5), entry=(1, fy(p, "pg", 502)))
p.edge("relay", "nats", "alerts.changed", EDGE_ORTHO, points=[(529, 610), (1460, 610)], exit=(0.9, 1), entry=(0.5, 1))
p.edge("sync", "nats", "connector.command", exit=(1, 0.5), entry=(0, fy(p, "nats", 502)))
p.text("Alert Engine хранит состояние тревог в PostgreSQL по схеме transactional outbox: изменение тревоги и запись outbox делаются в одной "
       "транзакции, Outbox Relay публикует alerts.changed только после фиксации. Подавленная тревога сохраняется и видна в UI, "
       "но не уходит ни в PagerDuty, ни в резервные каналы. " + ("RED/USE Link детерминированно связывает RED-тревогу сервиса и USE-тревогу его КЕ в одном окне и помечает USE вероятной причиной." if MVP else
       "Группировку тревог в инциденты выполняет Correlation Engine; Alert Engine только привязывает тревогу к инциденту."), 40, 650, 1520, 60)
p.legend()

# ---------------------------------------------------------------------
# PagerDuty Gateway
# ---------------------------------------------------------------------
p = cp("cpdg", "C4-3 PagerDuty Gateway", "C4 · Уровень 3 · Компоненты PagerDuty Gateway: основной канал оповещения")
PC, PR = [320, 590, 860, 1130], [140, 320, 500, 680]
p.node("bnd", "<b>PagerDuty Gateway</b> [Container: Go]", 300, 100, 1052, 710, BOUND)
comp(p, "lst", "Alert Listener" if MVP else "Incident Listener",
     "Читает alerts.changed: открытие, рост severity, resolve" if MVP else "Читает incidents.changed: открытие, рост severity, resolve",
     PC[0], PR[0], h=110)
comp(p, "router", "Service Router", "Сервис PagerDuty по РСМ; routing_key по ссылке из хранилища секретов", PC[1], PR[0], h=110)
comp(p, "mapper", "Event Mapper",
     ("summary, severity, dedup_key umb-‹id›, links: контекст в Grafana /go/incidents/{id}/grafana" if MVP else
      "summary, severity, dedup_key umb-inc-‹id›, links: контекст в Grafana /go/incidents/{id}/grafana"), PC[2], PR[0], h=110)
comp(p, "sender", "Outbox Sender", "Outbox в PostgreSQL, повторы с backoff, лимиты API, circuit breaker", PC[3], PR[0], h=110)
comp(p, "track", "Delivery Tracker", "pd.delivery: принято, ошибка, состояние канала (норма или деградация)", PC[2], PR[1], h=110)
comp(p, "recv", "Webhook Receiver", "Webhooks v3: проверка подписи; acknowledged, resolved, reassigned", PC[3], PR[1], h=110)
comp(p, "stp", "Status Publisher", "Тревога по dedup_key → pd.inbound" if MVP else "Инцидент по dedup_key → pd.inbound", PC[2], PR[2], h=110)
comp(p, "rec", "Reconciler", "Раз в 5 мин сверяет статусы инцидентов по REST API", PC[3], PR[2], h=110)
comp(p, "onc", "On-call Sync", "Раз в 5 мин копирует расписания и escalation policy в кеш дежурств" + ("" if MVP else " (на 7 дней вперёд)"),
     PC[3], PR[3], h=110)
p.node("nats", c4("NATS JetStream", "Container", ("alerts.changed" if MVP else "incidents.changed") + ", pd.delivery, pd.inbound"),
       20, 140, 150, 460, BUS)
p.node("pg", c4("PostgreSQL", "Container", "outbox, связь с инцидентами, кеш дежурств"), 20, 680, 170, 110, DB)
ext(p, "pd", "PagerDuty", "Events API v2, Webhooks v3, REST API" + ("" if MVP else " (токен только на чтение)"), 1480, 140, 210, 650)
bao(p, "bao", "routing_key, REST-токен, секрет подписи webhook", 1480, 850, 210, 110)
p.edge("nats", "lst", "alerts.changed" if MVP else "incidents.changed", exit=(1, fy(p, "nats", 195)), entry=(0, 0.5))
p.edge("lst", "router")
p.edge("router", "mapper")
p.edge("mapper", "sender")
p.edge("sender", "pd", "Events API v2", exit=(1, 0.5), entry=(0, fy(p, "pd", 195)))
p.edge("sender", "track", "", EDGE_ORTHO, points=[(1172, 285), (965, 285)], exit=(0.2, 1), entry=(0.5, 0))
p.edge("track", "nats", "pd.delivery", exit=(0, 0.5), entry=(1, fy(p, "nats", 375)))
p.edge("pd", "recv", "webhooks", EDGE_DASH, exit=(0, fy(p, "pd", 375)), entry=(1, 0.5))
p.edge("recv", "stp", "", EDGE_ORTHO, points=[(1100, 413), (1100, 555)], exit=(0, 0.85), entry=(1, 0.5))
p.edge("stp", "nats", "pd.inbound", exit=(0, 0.5), entry=(1, fy(p, "nats", 555)))
p.edge("rec", "pd", "REST: статусы", exit=(1, 0.5), entry=(0, fy(p, "pd", 555)))
p.edge("onc", "pd", "REST: расписания", exit=(1, 0.5), entry=(0, fy(p, "pd", 735)))
p.edge("onc", "pg", "кеш дежурств", EDGE_DASH, exit=(0, 0.5), entry=(1, 0.5))
p.edge("bnd", "bao", "секреты", EDGE_ORTHO, points=[(1254, 905)], exit=(fx(p, "bnd", 1254), 1), entry=(0, 0.5))
p.text("Сбой отправки не теряет алерт: запись в outbox делается в одной транзакции с изменением тревоги, повтор безопасен благодаря dedup_key. "
       "Delivery Tracker сообщает Fallback Notifier, приняла ли PagerDuty событие; circuit breaker открывается после 5 ошибок подряд или 60 с без ответа "
       "и только прекращает попытки отправки. Каждое событие содержит ссылку на контекст инцидента в Grafana. "
       "Кеш дежурств нужен именно на случай, когда PagerDuty недоступен.", 40, 1000, 1600, 60)
p.legend()

# ---------------------------------------------------------------------
# Fallback Notifier
# ---------------------------------------------------------------------
CH_MVP = "почта (SMTP 587 через корпоративный релей), HTTPS webhook во внутренний мессенджер"
CH_TGT = "почта, SMS- и голосовой шлюз, мессенджеры, HTTPS webhook, запасная платформа оповещения"
p = cp("cfbn", "C4-3 Fallback Notifier", "C4 · Уровень 3 · Компоненты Fallback Notifier: резервное оповещение, когда PagerDuty недоступен")
FC, FR, FH = [340, 600, 860, 1120], [240, 410, 580], 120
BB = 720 if TGT else 600
p.node("bnd", "<b>Fallback Notifier</b> [Container: Go]", 320, 180, 1040, BB - 180, BOUND)
comp(p, "lst", "Alert & Delivery Listener", "alerts.changed и pd.delivery: что отправлено и что PagerDuty принял", FC[0], FR[0], 220, FH)
comp(p, "det", "Failover Detector", "Тревога error или critical не принята за 2 мин с открытия (отсчёт идёт и при открытом circuit breaker)",
     FC[1], FR[0], 220, FH)
comp(p, "rcp", "Recipient Resolver", "Дежурные из кеша расписаний; кеш пуст или старше 24 ч → резервные контакты сервиса", FC[2], FR[0], 220, FH)
comp(p, "comp", "Message Composer", "Текст по шаблону события, ссылка на контекст в Grafana, ссылка подтверждения, сводка при шторме",
     FC[3], FR[0], 220, FH)
comp(p, "rcv", "Recovery Notifier", "PagerDuty снова принимает: сообщение «канал восстановлен», итог по тревогам", FC[1], FR[1], 220, FH)
comp(p, "log", "Delivery Log", "Журнал отправок в PostgreSQL, защита от повторов, лимиты на канал; notify.log", FC[0], FR[1], 220, FH)
comp(p, "esc", "Escalation Timer",
     "Нет подтверждения 10 мин → резервным контактам сервиса" if MVP else
     "Уровни escalation policy из кеша; таймауты уровней; после последнего уровня → резервные контакты", FC[2], FR[1], 220, FH)
comp(p, "ad", "Channel Adapters", CH_MVP if MVP else CH_TGT, FC[3], FR[1], 220, FH)
if TGT:
    comp(p, "ack", "Ack Handler", "Ответы на SMS и кнопки мессенджеров: проверка подписи → alerts.commands", FC[1], FR[2] + 20, 220, 105)
p.node("nats", c4("NATS JetStream", "Container", "alerts.changed, pd.delivery, notify.log" + ("" if MVP else ", alerts.commands")),
       20, 240, 150, 460 if TGT else 340, BUS)
p.node("pg", c4("PostgreSQL", "Container", "кеш дежурств, резервные контакты, журнал отправок"), 850, 60, 240, 100, DB)
if MVP:
    ext(p, "smtp", "Корпоративный почтовый релей", "SMTP 587 TLS, внутренняя сеть", 1420, 330, 220, 80)
    ext(p, "hook", "Внутренний мессенджер", "входящий webhook, HTTPS 443", 1420, 450, 220, 80)
    CHS = ("smtp", "hook")
else:
    ext(p, "smtp", "Корпоративный почтовый релей", "SMTP 587 TLS", 1420, 290, 220, 70)
    ext(p, "sms", "SMS- и голосовой шлюз", "через egress; резервный провайдер", 1420, 380, 220, 70)
    ext(p, "hook", "Мессенджеры", "бот и webhook, кнопка «Принять»", 1420, 470, 220, 70)
    ext(p, "alt", "Запасная платформа оповещения", "HTTPS webhook", 1420, 560, 220, 70)
    ext(p, "rp", "Reverse proxy + WAF", "DMZ: ответы SMS-шлюза, кнопки мессенджеров", FC[1], 800, 220, 90, "Infrastructure")
    CHS = ("smtp", "sms", "hook", "alt")
bao(p, "bao", "пароль SMTP-релея, адреса и токены webhook" if MVP else "пароль SMTP-релея, токены шлюзов и ботов, ключ подписи",
    1420, 680 if TGT else 620, 220, 110)
p.edge("nats", "lst", "alerts.changed,<br>pd.delivery", exit=(1, fy(p, "nats", 300)), entry=(0, 0.5))
p.edge("lst", "det")
p.edge("det", "rcp")
p.edge("rcp", "comp")
p.edge("comp", "ad", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("rcp", "pg", "кеш дежурств, резервные контакты", EDGE_DASH, exit=(0.5, 0), entry=(fx(p, "pg", 970), 1))
p.edge("lst", "rcv", "PagerDuty снова<br>принимает", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(520, 385), (680, 385)],
       exit=(fx(p, "lst", 520), 1), entry=(fx(p, "rcv", 680), 0))
p.edge("rcv", "ad", "«PagerDuty снова работает», итог", EDGE_ORTHO, points=[(790, FR[1] + FH + 15), (1170, FR[1] + FH + 15)],
       exit=(fx(p, "rcv", 790), 1), entry=(fx(p, "ad", 1170), 1))
p.edge("ad", "esc", "", EDGE_DASH, exit=(0, 0.5), entry=(1, 0.5))
p.edge("esc", "rcp", "нет ack: следующий получатель", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1))
p.edge("ad", "log", "журнал отправок", EDGE_ORTHO, points=[(1260, FR[1] + FH + 40), (450, FR[1] + FH + 40)],
       exit=(fx(p, "ad", 1260), 1), entry=(0.5, 1))
p.edge("log", "nats", "notify.log", exit=(0, 0.5), entry=(1, fy(p, "nats", FR[1] + FH / 2)))
ADY = FR[1] + FH / 2
for k in CHS:  # веер от Channel Adapters: общий ствол x = 1380, к каждому каналу своя горизонталь
    ky = p.geo[k][1] + p.geo[k][3] / 2
    if abs(ky - ADY) < 1:
        p.edge("ad", k, "", exit=(1, 0.5), entry=(0, 0.5))
    else:
        p.edge("ad", k, "", EDGE_ORTHO, points=[(1380, ADY), (1380, ky)], exit=(1, 0.5), entry=(0, 0.5))
p.edge("ad", "bao", "секреты", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1296, 735 if TGT else 675)], exit=(0.8, 1), entry=(0, 0.5))
if TGT:
    p.edge("ack", "nats", "alerts.commands:<br>ack", exit=(0, 0.5), entry=(1, fy(p, "nats", 652.5)))
    p.edge("rp", "ack", "ответы SMS, кнопки<br>мессенджеров (подпись)", exit=(0.5, 0), entry=(0.5, 1))
p.text("Резервный канал включается только для тревог уровня error и critical, которые PagerDuty не принял за 2 минуты с момента открытия; "
       "открытый circuit breaker прекращает попытки отправки, но отсчёт 2 минут не останавливает. PagerDuty остаётся основным: после "
       "восстановления туда уходит trigger с тем же dedup_key. Каждое сообщение содержит ссылку на контекст инцидента в Grafana. "
       + ("Подтверждение по одноразовой подписанной ссылке из сообщения обрабатывает Core API (Alert Actions) после входа через IdP."
          if MVP else "Ссылки подтверждения обрабатывает Core API; ответы на SMS и кнопки мессенджеров приходят через reverse proxy и WAF в Ack Handler."),
       40, BB + (210 if TGT else 170), 1600, 60)
p.legend()

# ---------------------------------------------------------------------
# Watchdog
# ---------------------------------------------------------------------
p = cp("cwdg", "C4-3 Watchdog", "C4 · Уровень 3 · Компоненты Watchdog: самоконтроль конвейера")
p.node("bnd", "<b>Watchdog</b> [Container: Go, 2 реплики, leader election]", 300, 90, 990, 340, BOUND)
comp(p, "gen", "Probe Generator", "Раз в минуту тестовое событие в ingest.in; тестовый сервис PagerDuty: low urgency, auto-resolve", 320, 130, 210, 110)
comp(p, "trk", "Pipeline Tracker", "Ждёт тестовую тревогу в alerts.changed и попытку отправки в pd.delivery", 560, 130, 210, 110)
comp(p, "slo", "SLO Evaluator", "Задержка событие → PagerDuty Gateway, доля успешных проб; p95 ≤ 30 с", 800, 130, 210, 110)
comp(p, "hb", "Heartbeat Pinger", "Ping внешнего heartbeat, если проба дошла до PagerDuty Gateway; не зависит от доступности PagerDuty",
     1040, 130, 230, 110)
comp(p, "lead", "Leader Election", "Lease в NATS KV: пробы шлёт только лидер, вторая реплика в резерве", 320, 290, 210, 110)
comp(p, "cht", "Channel Tester", "Раз в сутки тестовое сообщение по каждому резервному каналу", 560, 290, 210, 110)
comp(p, "met", "Metrics Exporter", "Метрики OpenMetrics: задержки, очереди, outbox", 800, 290, 210, 110)
if TGT:
    comp(p, "lag", "Replica Lag Monitor", "Задержка реплики PostgreSQL и зеркала NATS на резервной площадке; тревога при > 1 мин",
         1040, 290, 230, 110)
    p.node("dr", c4("Резервная площадка", "тёплый резерв", "PostgreSQL-реплика (5432), зеркало NATS (7422)"), 1040, 490, 230, 100, REPL)
p.node("nats", c4("NATS JetStream", "Container", "ingest.in, alerts.changed, pd.delivery, notify.log, KV lease"), 20, 130, 170, 270, BUS)
ext(p, "ext", "Внешний heartbeat", "поднимает инцидент, если ping пропал 3 мин", 1370, 130, 200, 110)
cont(p, "fbn", "Fallback Notifier", "Container", "тестовая отправка", 560, 490, 210, 90)
p.edge("gen", "nats", "ingest.in", exit=(0, 0.5), entry=(1, fy(p, "nats", 185)))
p.edge("nats", "trk", "alerts.changed, pd.delivery", EDGE_ORTHO, points=[(105, 110), (665, 110)], exit=(0.5, 0), entry=(0.5, 0))
p.edge("trk", "slo")
p.edge("slo", "hb")
p.edge("hb", "ext", "HTTPS")
p.edge("lead", "nats", "KV lease", EDGE_DASH, exit=(0, 0.5), entry=(1, fy(p, "nats", 345)))
p.edge("cht", "fbn", "тестовая команда", exit=(0.5, 1), entry=(0.5, 0))
p.edge("slo", "met", "", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
if TGT:
    p.edge("lag", "dr", "задержка репликации", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.text("Зонтичный мониторинг не может следить сам за собой, поэтому последняя линия защиты внешняя: heartbeat уходит, только если тестовая проба "
       "прошла конвейер до PagerDuty Gateway. Если Umbrella перестала работать целиком, ping прекращается и внешний сервис сам поднимает инцидент. "
       "Недоступность самой PagerDuty heartbeat не останавливает: её закрывает резервное оповещение.", 40, 640, 1500, 60)
p.legend()

# ---------------------------------------------------------------------
# Context Builder: дашборд инцидента в Grafana
# ---------------------------------------------------------------------
p = cp("cctx", "C4-3 Context Builder", "C4 · Уровень 3 · Компоненты Context Builder: дашборд инцидента в Grafana")
XC, XW = [350, 610, 870, 1130, 1390], 210
Y1, Y2, Y3 = 260, 440, 620
p.node("bnd", "<b>Context Builder</b> [Container: Go]", 330, 190, 1290, 550, BOUND)
comp(p, "api", "Context API", "context.request (request/reply), ответ: URL с интервалом времени; подписка alerts.changed"
     + ("" if MVP else ", incidents.changed"), XC[0], Y1, XW, 120)
comp(p, "rm", "Rule Matcher", "Правило «Связи контекста» по типу КЕ, сигналу, сервису; опубликованная версия", XC[1], Y1, XW, 120)
comp(p, "rr", "Relation Resolver",
     "Обход карты CMDB: тип связи, направление, глубина ≤ 3" if MVP else
     "Все КЕ коррелированного инцидента + обход карты: тип связи, направление, глубина ≤ 3", XC[2], Y1, XW, 120)
comp(p, "vb", "Variable Binder", "${ci.host}, ${ci.name}, ${service}: экранирование под язык запроса, белый список шаблонов",
     XC[3], Y1, XW, 120)
comp(p, "tw", "Time Window", "Начало инцидента − 10 мин … закрытие или сейчас + 10 мин; настраивается в правиле", XC[4], Y1, XW, 120)
comp(p, "cache", "Cache", "Готовый дашборд по инциденту и версии правил; сброс при изменении инцидента или правил",
     XC[0], Y2, XW, 120)
comp(p, "cln", "Cleanup Job", "Удаляет дашборды через 30 дней после закрытия инцидента", XC[1], Y2, XW, 120)
comp(p, "fts", "Folder & Team Sync", "Команды Grafana по группам каталога, права на папки по привязкам групп из PostgreSQL",
     XC[2], Y2, XW, 120)
comp(p, "aw", "Annotation Writer", "Аннотации с событиями инцидента: открытие, ack, эскалации, resolve", XC[3], Y2, XW, 120)
comp(p, "dc", "Dashboard Composer",
     "JSON дашборда: панели метрик, логов, трейсов, сводка инцидента; раскладка; uid umb-‹id›" if MVP else
     "JSON дашборда: панели всех КЕ инцидента, вероятная причина выделена; uid umb-‹id›", XC[4], Y2, XW, 120)
comp(p, "gc", "Grafana Client",
     "HTTP API Grafana: POST /api/dashboards/db (идемпотентно, папка команды), аннотации, команды и права папок; "
     "токен сервисного аккаунта из хранилища секретов; права только на папки Umbrella", XC[1], Y3, XC[4] + XW - XC[1], 100)
p.node("nats", c4("NATS JetStream", "Container", "context.request, alerts.changed" + ("" if MVP else ", incidents.changed")),
       20, 260, 170, 150, BUS)
p.node("pg", c4("PostgreSQL", "Container", "связи контекста, шаблоны панелей, карта CMDB, привязки групп"), 610, 60, 560, 100, DB)
bao(p, "bao", "токен сервисного аккаунта Grafana", 1760, 615, 220, 110)
ext(p, "graf", "Grafana", ("корпоративная или Grafana OSS ×2 в зоне приложений; вход через IdP (OIDC)" if MVP else
                           "корпоративная или Grafana OSS ×2 на каждой площадке; вход через IdP (OIDC)"), 990, 800, 260, 110)
ext(p, "store", "Хранилища метрик, логов и трейсов", "существующие системы, источники данных Grafana", 1400, 805, 260, 100)
p.node("duty", c4("Дежурный инженер", "Person", "открывает дашборд из Umbrella или PagerDuty"), 600, 810, 200, 90, PERSON)
p.edge("nats", "api", "context.request", exit=(1, fy(p, "nats", 280)), entry=(0, fy(p, "api", 280)))
p.edge("api", "nats", "ответ: URL", EDGE_DASH, exit=(0, fy(p, "api", 320)), entry=(1, fy(p, "nats", 320)))
p.edge("nats", "api", "alerts.changed" if MVP else "alerts.changed,<br>incidents.changed", EDGE_DASH,
       exit=(1, fy(p, "nats", 360)), entry=(0, fy(p, "api", 360)))
p.edge("api", "rm")
p.edge("rm", "rr")
p.edge("rr", "vb")
p.edge("vb", "tw")
p.edge("tw", "dc", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("dc", "aw", "", exit=(0, 0.5), entry=(1, 0.5))
p.edge("api", "cache", "чтение и сброс", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.edge("rm", "pg", "правила, шаблоны панелей", EDGE_DASH, exit=(0.5, 0), entry=(fx(p, "pg", 715), 1))
p.edge("rr", "pg", "карта CMDB", EDGE_DASH, exit=(0.5, 0), entry=(fx(p, "pg", 975), 1))
p.edge("fts", "pg", "привязки групп", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1105, Y2 + 30), (1105, 200)],
       exit=(1, 0.25), entry=(fx(p, "pg", 1105), 1), lpos=0.6)
for k, lab in (("cln", "удаление"), ("fts", "команды, права папок"), ("aw", "аннотации"), ("dc", "JSON дашборда")):
    x0, _, w0, _ = p.geo[k]
    p.edge(k, "gc", lab, exit=(0.5, 1), entry=(fx(p, "gc", x0 + w0 / 2), 0))
p.edge("gc", "graf", "HTTPS 443, HTTP API", exit=(fx(p, "gc", 1120), 1), entry=(0.5, 0))
p.edge("gc", "bao", "токен сервисного<br>аккаунта", EDGE_DASH, exit=(1, 0.5), entry=(0, 0.5))
p.edge("duty", "graf", "HTTPS 443, OIDC", exit=(1, 0.5), entry=(0, fy(p, "graf", 855)))
p.edge("graf", "store", "запросы панелей", EDGE_DASH, exit=(1, fy(p, "graf", 855)), entry=(0, 0.5))
p.text("Клик по инциденту на дашборде открывает в новой вкладке /go/incidents/{id}/grafana. Core API отправляет context.request; Context Builder "
       "находит правило «Связи контекста», обходит связи КЕ по карте CMDB, подставляет переменные в шаблоны запросов, создаёт или обновляет "
       "дашборд umb-‹id› в папке команды и пишет аннотации; Core API перенаправляет браузер в Grafana с интервалом от начала инцидента − 10 мин "
       "до закрытия (или текущего момента) + 10 мин. Готовый дашборд берётся из кеша (цель ≤ 2 с) и пересобирается при изменении инцидента или правил. "
       + ("" if MVP else "В дашборд попадают все КЕ коррелированного инцидента, вероятная причина выделена. ")
       + "Та же ссылка уходит в PagerDuty (links) и в резервные сообщения. Umbrella не хранит метрики, логи и трейсы: панели читают их из "
       "существующих хранилищ через источники данных Grafana, учётные данные которых выдаёт хранилище секретов.", 40, 960, 1940, 80)
p.legend()

# ---------------------------------------------------------------------
# Только целевая архитектура: History Service, Correlation Engine, ITSM Sync
# ---------------------------------------------------------------------
if TGT:
    p = cp("chst", "C4-3 History Service", "C4 · Уровень 3 · Компоненты History Service: история событий, отчёты и поиск")
    p.node("bnd", "<b>History Service</b> [Container: Go]", 280, 90, 1010, 400, BOUND.replace("verticalAlign=bottom", "verticalAlign=top"))
    comp(p, "arc", "Stream Archiver", "events.norm без raw, alerts.changed, incidents.changed, notify.log → пакетная вставка", 300, 130, 210, 110)
    comp(p, "ret", "Retention Manager", "Партиции по дням, TTL 13 месяцев, агрегаты на 3 года", 540, 130, 210, 110)
    comp(p, "qry", "Query API", "Отчёты MTTA, MTTR, шум, покрытие, SLA; поиск по истории для дашборда инцидентов", 780, 130, 210, 110)
    comp(p, "rep", "Report Scheduler", "Регулярные отчёты владельцам сервисов", 1040, 130, 230, 110)
    comp(p, "mask", "Masking", "Скрывает поля по разрешённому списку, правам и области", 1040, 290, 230, 110)
    comp(p, "bf", "Backfill", "После недоступности ClickHouse догружает пропущенное из NATS и горячих данных PostgreSQL (7 дней)",
         300, 290, 150, 140)
    p.node("pg", c4("PostgreSQL", "Container", "горячие данные 7 дней"), 40, 300, 180, 100, DB)
    p.edge("pg", "bf", "SQL", EDGE_DASH, exit=(1, 0.5), entry=(0, fy(p, "bf", 350)))
    p.node("nats", c4("NATS JetStream", "Container", "events.norm, alerts.changed, incidents.changed, notify.log"), 40, 130, 200, 110, BUS)
    p.node("ch", c4("ClickHouse", "Container: ClickHouse, 3 узла", "события, тревоги, инциденты, отправки"), 300, 550, 690, 100, DB)
    cont(p, "api", "Core API", "Container", "Reports Proxy, Incident Query & Search", 1340, 130, 220, 110)
    p.edge("nats", "arc", "", exit=(1, 0.5), entry=(0, 0.5))
    p.edge("arc", "ch", "INSERT пачками", exit=(0.92, 1), entry=(fx(p, "ch", 493), 0))
    p.edge("bf", "ch", "INSERT", exit=(0.5, 1), entry=(fx(p, "ch", 375), 0), lpos=-0.5)
    p.edge("ret", "ch", "TTL, партиции", EDGE_DASH, exit=(0.5, 1), entry=(fx(p, "ch", 645), 0))
    p.edge("qry", "ch", "SELECT", EDGE_DASH, exit=(0.5, 1), entry=(fx(p, "ch", 885), 0))
    p.edge("api", "qry", "REST: отчёты, поиск по истории", EDGE_ORTHO, points=[(1450, 110), (885, 110)], exit=(0.5, 0), entry=(0.5, 0))
    p.edge("rep", "qry", "", EDGE_DASH, exit=(0, 0.3), entry=(1, 0.3))
    p.edge("qry", "mask", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1015, 224), (1015, 345)], exit=(1, 0.85), entry=(0, 0.5))
    p.text("История живёт в ClickHouse и не нагружает PostgreSQL: там остаются только горячие данные за 7 дней. Query API читает ClickHouse и "
           "отвечает Core API на отчёты и на поиск по истории с дашборда инцидентов. Отчёты читают копию событий, поэтому потеря ClickHouse "
           "не влияет на передачу алертов. ClickHouse на резервную площадку не реплицируется: после переключения он восстанавливается из S3, "
           "а Backfill догружает пропущенное из PostgreSQL.", 40, 690, 1520, 60)
    p.legend()

    p = cp("ccor", "C4-3 Correlation Engine", "C4 · Уровень 3 · Компоненты Correlation Engine: инциденты и вероятная причина")
    p.node("bnd", "<b>Correlation Engine</b> [Container: Go]", 280, 90, 1000, 500, BOUND)
    comp(p, "win", "Alert Window", "Скользящее окно открытых тревог по сервисам и КЕ из alerts.changed", 300, 130)
    comp(p, "topo", "Topology Correlator", "Общая причина по связям КЕ: хост → поды → сервисы", 540, 130)
    comp(p, "reduse", "RED/USE Correlator", "RED-симптом сервиса + USE-причина на его КЕ: USE помечается вероятной причиной", 780, 130)
    comp(p, "rank", "Root Cause Ranker", "Оценка вероятной причины: глубина в графе, время, история", 1040, 130, 230)
    comp(p, "learn", "Pattern Learner", "Статистика совместных срабатываний; предлагает правила в UI, не применяет сам", 300, 290)
    comp(p, "inc", "Incident Builder", "Каждая тревога сразу входит в инцидент; инцидент и строка outbox в одной транзакции", 1040, 290, 230)
    comp(p, "relay", "Outbox Relay", "Читает outbox, публикует incidents.changed: состав и причина", 1040, 450, 230)
    p.node("nats", c4("NATS JetStream", "Container", "alerts.changed, cmdb.changed, incidents.changed"), 40, 130, 200, 130, BUS)
    p.node("pg", c4("PostgreSQL", "Container", "инциденты, outbox, граф КЕ"), 1390, 290, 200, 104, DB)
    p.node("ch", c4("ClickHouse", "Container", "история для обучения"), 50, 450, 190, 90, DB)
    p.edge("nats", "win", "", exit=(1, fy(p, "nats", 182)), entry=(0, 0.5))
    p.edge("win", "topo")
    p.edge("topo", "reduse")
    p.edge("reduse", "rank")
    p.edge("rank", "inc", "", exit=(0.5, 1), entry=(0.5, 0))
    p.edge("inc", "pg", "инцидент + outbox<br>(1 транзакция)", exit=(1, 0.5), entry=(0, 0.5))
    p.edge("relay", "pg", "outbox", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1490, 502)], exit=(1, 0.5), entry=(0.5, 1))
    p.edge("relay", "nats", "incidents.changed", EDGE_ORTHO, points=[(1155, 630), (20, 630), (20, 230)], exit=(0.5, 1), entry=(0, fy(p, "nats", 230)))
    p.edge("topo", "pg", "граф КЕ", EDGE_ORTHO, points=[(645, 110), (1490, 110)], exit=(0.5, 0), entry=(0.5, 0))
    p.edge("learn", "ch", "SQL", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(145, 342)], exit=(0, 0.5), entry=(0.5, 0))
    p.text("Корреляция детерминированная и объяснимая: в инциденте видно, по какой связи КЕ тревоги объединены. "
           "Pattern Learner только предлагает правила инженеру, автоматически ничего не включает. Инцидент и строка outbox пишутся в одной транзакции, "
           "Outbox Relay публикует incidents.changed только после фиксации.", 40, 670, 1550, 40)
    p.legend()

    p = cp("citm", "C4-3 ITSM Sync", "C4 · Уровень 3 · Компоненты ITSM Sync: корпоративная CMDB и ITSM")
    IC = [300, 540, 780, 1020, 1260]
    p.node("bnd", "<b>ITSM Sync</b> [Container: Go]", 280, 90, 1200, 490, BOUND)
    comp(p, "exp", "CMDB Exporter", "Технические КЕ и связи из карты Umbrella в корпоративную CMDB", IC[0], 130)
    comp(p, "tick", "Ticket Creator", "Инцидент в ITSM по правилам для критичных бизнес-услуг: запись в outbox", IC[1], 130)
    comp(p, "st", "Status Sync", "Статусы тикетов и инцидентов в обе стороны", IC[2], 130)
    comp(p, "imp", "Business Service Import", "Бизнес-услуги и владельцы из ITSM, мастер-система там", IC[3], 130)
    comp(p, "rev", "Change Review", "Конфликты импорта на ревью инженеру", IC[4], 130)
    comp(p, "ob", "Outbox", "Outbox в PostgreSQL: заявки в ITSM с повторами, идемпотентно по внешнему id", IC[1], 290)
    comp(p, "adp", "Adapter Blocks", "Блоки конструктора: REST, SOAP, SQL; без кода; учётные данные из хранилища секретов",
         IC[0], 450, IC[3] + CW3 - IC[0], 100)
    p.node("nats", c4("NATS JetStream", "Container", "cmdb.changed, incidents.changed"), 20, 130, 150, 110, BUS)
    ext(p, "itsm", "Корпоративная CMDB и ITSM", "REST, SOAP или SQL", 1520, 440, 220, 120)
    p.node("pg", c4("PostgreSQL", "Container", "связи с объектами ITSM, outbox, конфликты импорта"), 1560, 130, 220, 104, DB)
    p.edge("nats", "exp", "cmdb.changed", exit=(1, 0.5), entry=(0, fy(p, "exp", 185)))
    p.edge("nats", "tick", "incidents.changed", EDGE_ORTHO, points=[(95, 110), (645, 110)], exit=(0.5, 0), entry=(0.5, 0))
    p.edge("tick", "ob", "outbox", exit=(0.5, 1), entry=(0.5, 0))
    p.edge("imp", "rev")
    p.edge("rev", "pg", "", exit=(1, 0.5), entry=(0, fy(p, "pg", 182)))
    for k, lab in (("exp", "КЕ и связи"), ("ob", "заявки"), ("st", "статусы")):
        x0, _, w0, _ = p.geo[k]
        p.edge(k, "adp", lab, exit=(0.5, 1), entry=(fx(p, "adp", x0 + w0 / 2), 0))
    p.edge("adp", "imp", "бизнес-услуги", EDGE_DASH, exit=(fx(p, "adp", 1125), 0), entry=(0.5, 1))
    p.edge("adp", "itsm", "REST, SOAP, SQL", exit=(1, 0.3), entry=(0, fy(p, "itsm", 480)))
    p.edge("itsm", "adp", "ответы, статусы,<br>бизнес-услуги", EDGE_DASH, exit=(0, fy(p, "itsm", 520)), entry=(1, 0.7))
    p.text("Корпоративная CMDB остаётся мастером бизнес-услуг, Umbrella — мастером технических КЕ из мониторинга. Заявки в ITSM пишутся в outbox "
           "в одной транзакции с изменением инцидента и не теряются при сбое ITSM. Учётные данные ITSM, как и все остальные, лежат только в "
           "хранилище секретов (OpenBao).", 40, 620, 1740, 60)
    p.legend()
