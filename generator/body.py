
MONO = "font-family:Courier New;font-size:10px"
LEFT = "align=left;spacingLeft=10;spacingRight=6;"

# =====================================================================
# 1. C4 L1 — контекст
# =====================================================================
p = Page("C4-1 Контекст", "ctx"); pages.append(p)
p.text("<b>C4 · Уровень 1 · Контекст системы Umbrella</b>", 40, 20, 900, 30, TITLE)
W = 250
UX, UY, UW, UH = 520, 450, 1150, 380
p.node("umb", c4("Umbrella", "Software System",
                 "Зонтичный мониторинг: забирает события из любых источников через low-code коннекторы, подтверждает получение, "
                 "парсит и нормализует, схлопывает дубли, строит карту CMDB, формирует тревоги по RED и USE"
                 + (" и передаёт их в PagerDuty" if MVP else ", объединяет их в инциденты по топологии и передаёт инциденты в PagerDuty")
                 + "; если PagerDuty не принял алерт, оповещает через резервные каналы. Дашборд инцидентов с фильтрами и поиском, "
                 "контекст инцидента в Grafana по связям из low-code, ролевая модель по группам корпоративного каталога"),
       UX, UY, UW, UH, SYSTEM + "fontSize=13;")
p.cells[-1] = p.cells[-1][:2] + (p.cells[-1][2].replace("font-size:11px", "font-size:15px").replace("<b>Umbrella</b>", "<b><font style='font-size:20px'>Umbrella</font></b>"),) + p.cells[-1][3:]
# верх: люди, Grafana и хранилища
p.node("eng", c4("Инженер мониторинга", "Person", "Собирает коннекторы в low-code конструкторе, настраивает парсинг, шаблоны, правила RED/USE, дедупликацию, обнаружение КЕ и связи контекста для Grafana"), 560, 190, W, 150, PERSON)
p.node("graf", c4("Grafana", "External System", "Дашборд инцидента: метрики, логи, трейсы"), 1000, 230, 260, 110, EXT)
p.node("stores", c4("Хранилища метрик, логов и трейсов", "External System", "Источники данных Grafana; Umbrella их не хранит"), 1000, 70, 260, 100, EXT)
p.node("duty", c4("Дежурный инженер", "Person", "Получает звонки и push от PagerDuty, подтверждает инциденты, разбирает их в дашборде инцидентов и Grafana"), 1400, 190, W, 150, PERSON)
p.edge("eng", "umb", "Настраивает<br>[HTTPS, веб-UI]", exit=(0.5, 1), entry=(round((685 - UX) / UW, 4), 0))
p.edge("umb", "graf", "Дашборд инцидента<br>[HTTPS API]", exit=(round((1130 - UX) / UW, 4), 0), entry=(0.5, 1))
p.edge("graf", "stores", "запросы панелей", exit=(0.5, 0), entry=(0.5, 1))
p.edge("duty", "graf", "открывает из<br>дашборда инцидентов", exit=(0, 0.6333), entry=(1, 0.5))
p.edge("duty", "umb", "Дашборд инцидентов,<br>контекст, карта CMDB<br>[HTTPS, веб-UI]", exit=(0.5, 1), entry=(round((1525 - UX) / UW, 4), 0))
# источники слева
SRC = [("mon", "Системы мониторинга", "Любые: события, метрики и инвентарь по API или webhook", "API, webhook;<br>подтверждение получения"),
       ("db", "Базы данных систем мониторинга", "Таблицы событий, проблем, хостов; чтение SQL", "SQL-запросы;<br>отметка о передаче"),
       ("cloud", "Облачные провайдеры", "AWS и другие: алармы, метрики, события, инвентарь", "API облака, события;<br>теги и группы ресурсов"),
       ("stream", "Потоки событий", "Очереди сообщений, syslog, почтовые ящики, файлы", "подписка на очередь,<br>ack сообщения")]
for i, (k, n, d, lab) in enumerate(SRC):
    y = 470 + i * 90
    p.node(k, c4(n, "External System", d), 40, y, 320, 80, EXT + "fontSize=11;")
    p.edge(k, "umb", lab, exit=(1, 0.5), entry=(0, round((y + 40 - UY) / UH, 4)))
# справа: PagerDuty и резервные каналы
RX = 1810
p.node("pd", c4("PagerDuty", "External System", "Инциденты, расписания, escalation policy; доставка звонком, push, SMS, email"), RX, 450, 260, 160, EXT)
p.node("fbx", c4("Резервные каналы оповещения", "External System", FB_CHANNELS), RX, 650, 260, 130, EXT)
p.edge("umb", "pd", "Алерты<br>[Events API v2:<br>trigger, resolve]", exit=(1, round((485 - UY) / UH, 4)), entry=(0, round(35 / 160, 4)))
p.edge("pd", "umb", "Статусы инцидентов<br>[Webhooks v3:<br>ack, resolve]", EDGE_DASH, exit=(0, round(125 / 160, 4)), entry=(1, round((575 - UY) / UH, 4)))
p.edge("umb", "fbx", "Резервное оповещение,<br>если PagerDuty<br>не принял алерт", exit=(1, round((715 - UY) / UH, 4)), entry=(0, round(65 / 130, 4)))
p.edge("duty", "pd", "получает уведомления;<br>ack и resolve", EDGE_ORTHO + "startArrow=block;startFill=1;startSize=6;", points=[(RX + 130, 302.5)], exit=(1, 0.75), entry=(0.5, 0))
p.edge("fbx", "duty", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(2130, 715), (2130, 235)], exit=(1, 0.5), entry=(1, 0.3))
p.text("письмо, webhook" if MVP else "SMS, звонок,<br>мессенджер, почта", 2140, 460, 130, 40, NOTE + "fontSize=10;fontColor=#404040;")
# снизу: каталог, SIEM, heartbeat, S3 (+ CMDB/ITSM и резервная площадка)
BOT = [("idp", c4("Корпоративный IdP и каталог (AD)", "External System", "Вход по OIDC; группы пользователей для ролевой модели"), EXT,
        "вход OIDC; группы<br>[LDAPS 636 / SCIM]"),
       ("siem", c4("SIEM", "External System", "Журнал аудита и событий безопасности"), EXT, "аудит<br>[syslog TLS 6514]"),
       ("hb", c4("Внешний heartbeat-сервис", "External System", "Поднимает тревогу, если Umbrella перестала слать heartbeat"), EXT, "heartbeat<br>[HTTPS]"),
       ("s3", c4("Объектное хранилище S3", "External System", "Резервные копии; другая площадка или регион"), EXT, "резервные копии<br>[HTTPS 443]")]
if TGT:
    BOT += [("itsm", c4("Корпоративная CMDB и ITSM", "External System", "Эталон КЕ и бизнес-услуг, инциденты и изменения"), EXT,
             "КЕ, связи, инциденты<br>[REST]"),
            ("dr", c4("Резервная площадка Umbrella", "Тёплый резерв", "Реплики PostgreSQL, NATS, OpenBao; включается при потере основной"), REPL,
             "репликация<br>[5432, 7422, 443]")]
n = len(BOT)
BW = 200 if MVP else 178
gap = (UW - n * BW) / (n - 1)
for i, (k, lab, st, el) in enumerate(BOT):
    x = round(UX + i * (BW + gap))
    p.node(k, lab, x, UY + UH + 130, BW, 130, st)
    p.edge("umb", k, el, exit=(round((x + BW / 2 - UX) / UW, 4), 1), entry=(0.5, 0))
p.text("Umbrella не привязана к конкретным системам мониторинга: подключение к любому источнику собирается из блоков в low-code конструкторе. "
       "Основной канал оповещения — PagerDuty: один алерт на " + ("проблему" if MVP else "инцидент") + " уходит туда, а PagerDuty по расписаниям и escalation policy "
       "звонит, шлёт push, SMS и email. Если PagerDuty не принял алерт severity error или critical за 2 минуты, Umbrella сама оповещает дежурных по резервным каналам. "
       "Статусы ack и resolve возвращаются в Umbrella и дальше в источники. Дежурный работает в дашборде инцидентов Umbrella; по клику Umbrella строит "
       "дашборд инцидента в Grafana (метрики, логи и трейсы берутся из существующих хранилищ, Umbrella их не хранит). "
       "Доступ к функциям, областям и предустановкам дашборда задаётся ролями по группам корпоративного каталога (AD); аудит уходит в SIEM.",
       40, UY + UH + 300, 2200, 60)

# =====================================================================
# 2. C4 L2 — контейнеры
# =====================================================================
p = Page("C4-2 Контейнеры", "cnt"); pages.append(p)
p.text("<b>C4 · Уровень 2 · Контейнеры Umbrella</b>", 40, 20, 500, 30, TITLE)
c0, c1, c2, c3 = 400, 660, 920, 1180
CW, CH = 230, 120
BX, BY = 360, 190
BH = 760 if MVP else 1000
p.node("bnd", "<b>Umbrella</b> [Software System]", BX, BY, 1306, BH, BOUND)
BIDIR = EDGE + "startArrow=block;startFill=1;startSize=6;"
p.node("eng", c4("Инженер мониторинга", "Person", "Собирает коннекторы, шаблоны, правила, связи контекста"), 600, 40, CW, 110, PERSON)
p.node("duty", c4("Дежурный инженер", "Person", "Звонок, push, SMS от PagerDuty; дашборд инцидентов, Grafana"), c3, 30, 240, 110, PERSON)
r1 = 220
p.node("sec", c4("Хранилище секретов", "Container: OpenBao ×3, Raft", "Все логины, пароли, токены, ключи и сертификаты; сервисы получают их через agent injector"), c0, r1 - 5, CW, 140, DB)
p.node("web", c4("Веб-интерфейс", "Container: React, TypeScript, React Flow", "Дашборд инцидентов, карта CMDB, low-code конструктор коннекторов, шаблонов, правил и связей контекста"), c1, r1, CW, 130, CONT)
p.node("wd", c4("Watchdog", "Container: Go", "Тестовое событие через весь конвейер, heartbeat во внешний сервис, тест резервных каналов"), c2, r1, CW, 130, CONT)
p.node("fbn", c4("Fallback Notifier", "Container: Go", "Резервное оповещение дежурных, когда PagerDuty не принял алерт"), c3, r1, CW, 130, CONT)
r2 = 420
p.node("ingest", c4("Ingest Gateway", "Container: Go", "Приём webhook (push); отвечает 2xx только после записи в шину"), c0, r2, CW, CH, CONT)
p.node("api", c4("Core API", "Container: Go, REST + WebSocket", "Конфигурация, тревоги, дашборд инцидентов, карта CMDB, RBAC, синхронизация групп каталога"), c1, r2, CW, CH + 10, CONT)
p.node("pg", c4("PostgreSQL", "Container: PostgreSQL 16", "Коннекторы, курсоры, CMDB и РСМ, тревоги, роли, связи контекста, аудит"), c2, r2 - 5, CW, CH + 20, DB)
p.node("pdg", c4("PagerDuty Gateway", "Container: Go", "Events API v2 с dedup_key, повторы; webhooks; кеш дежурств"), c3, r2, CW, CH, CONT)
STREAMS = ("ingest.in, events.raw, events.dlq, events.norm, alerts.changed, alerts.commands, pd.delivery, pd.inbound, "
           "connector.query, connector.command, cmdb.changed, notify.log" + ("" if MVP else ", incidents.changed")
           + "; запрос-ответ context.request")
R3X = [400, 610, 820, 1030, 1240, 1450]
R3W, R3H = 196, 150
p.node("bus", c4("Шина событий", "Container: NATS JetStream", STREAMS), R3X[0], 620, R3X[-1] + R3W - R3X[0], 80, BUS)
r3 = 760
p.node("conn", c4("Connector Runtime", "Container: Go", "Исполняет low-code коннекторы: API, SQL, очереди, облака, syslog; курсоры, парсинг, ack источнику"), R3X[0], r3, R3W, R3H, CONT)
p.node("rule", c4("Rule Engine", "Container: Go", "Тревоги по RED и USE; метрики запрашивает у коннекторов"), R3X[1], r3, R3W, R3H, CONT)
p.node("norm", c4("Normalizer", "Container: Go, CEL, JSONata", "Маппинг и шаблоны, кандидат КЕ, сигнал, ключ дедупликации"), R3X[2], r3, R3W, R3H, CONT)
p.node("alert", c4("Alert Engine", "Container: Go", "Схлопывание дублей, окончательная привязка к КЕ, окна, РСМ; transactional outbox"), R3X[3], r3, R3W, R3H, CONT)
p.node("cmdb", c4("CMDB Discovery", "Container: Go", "Карта КЕ и связей из инвентаря; плавающие ID облака → устойчивая КЕ"), R3X[4], r3, R3W, R3H, CONT)
p.node("ctxb", c4("Context Builder", "Container: Go", "Дашборд инцидента в Grafana по low-code связям контекста"), R3X[5], r3, R3W, R3H, CONT)
# слева: каталог и источники
p.node("idp", c4("Корпоративный IdP и каталог (AD)", "External System", "OIDC; группы: LDAPS / SCIM"), 20, 170, 240, 110, EXT)
p.node("src_bnd", "<b>Источники</b>", 10, 400, 260, 470, NODE_BOUND)
SX, SWd = 20, 240
p.node("mon", c4("Системы мониторинга", "External System", "API, webhook"), SX, 430, SWd, 90, EXT)
p.node("db", c4("БД систем мониторинга", "External System", "SQL"), SX, 540, SWd, 90, EXT)
p.node("cloud", c4("Облачные провайдеры", "External System", "API, события, теги"), SX, 650, SWd, 90, EXT)
p.node("stream", c4("Потоки событий", "External System", "очереди, syslog, почта"), SX, 760, SWd, 90, EXT)
# справа: внешние системы
rx = 1830
p.node("fbx", c4("Резервные каналы", "External System", FB_CHANNELS), rx, 220, 240, 130, EXT)
p.node("pd", c4("PagerDuty", "External System", "Основной канал: инциденты, эскалации, доставка"), rx, 420, 240, 160, EXT)
p.node("graf", c4("Grafana", "External System", "Дашборд инцидента: метрики, логи, трейсы"), rx, 775, 240, 120, EXT)
p.node("stores", c4("Хранилища метрик, логов и трейсов", "External System", "Источники данных Grafana"), rx, 955 if MVP else 935, 240, 90 if MVP else 80, EXT)
# связи
p.edge("eng", "web", "Использует [HTTPS]", lpos=-0.45, exit=(round(125 / CW, 4), 1), entry=(round(65 / CW, 4), 0))
p.edge("duty", "web", "дашборд инцидентов,<br>контекст, карта CMDB", EDGE_ORTHO, points=[(850, 85)], exit=(0, 0.5), entry=(round(190 / CW, 4), 0))
p.edge("web", "api", "REST / WebSocket")
p.text("OIDC; группы<br>[LDAPS 636 / SCIM 443]", 150, 300, 150, 40, NOTE + "fontSize=10;fontColor=#404040;")
p.edge("api", "idp", "", EDGE_ORTHO, points=[(645, 438), (645, 380), (140, 380)], exit=(0, 0.1385), entry=(0.5, 1))
p.edge("api", "pg", "SQL", exit=(1, 0.5), entry=(0, round(70 / 140, 4)))
p.edge("mon", "ingest", "webhook", exit=(1, 0.5), entry=(0, round(55 / CH, 4)))
p.edge("cloud", "ingest", "события", EDGE_ORTHO, points=[(320, 695), (320, 515)], exit=(1, 0.5), entry=(0, round(95 / CH, 4)))
p.edge("conn", "src_bnd", "API, SQL,<br>очереди; ack", lpos=0.35, exit=(0, 0.5), entry=(1, round((835 - 400) / 470, 4)))
CMY = 1000 if MVP else 1240
p.edge("cmdb", "src_bnd", "инвентарь, теги, группы", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;",
       points=[(R3X[4] + 4, CMY), (140, CMY)], exit=(round(4 / R3W, 4), 1), entry=(0.5, 1))
p.vert("ingest", "bus", "ingest.in")
p.vert("api", "bus", "alerts.commands,<br>context.request", BIDIR)
p.vert("pdg", "bus", ("alerts.changed" if MVP else "incidents.changed") + ",<br>pd.delivery, pd.inbound", BIDIR)
p.vert("conn", "bus", "ingest.in, events.raw,<br>events.dlq, connector.*", BIDIR, down=False)
p.vert("rule", "bus", "events.raw,<br>connector.query", BIDIR, down=False)
p.vert("norm", "bus", "events.norm", BIDIR, down=False)
p.vert("alert", "bus", "alerts.changed,<br>cmdb.changed", BIDIR, down=False)
p.vert("cmdb", "bus", "cmdb.changed", down=False)
p.vert("ctxb", "bus", "context.request,<br>alerts.changed" + ("" if MVP else ",<br>incidents.changed"), BIDIR, down=False)
p.edge("pdg", "fbn", "pd.delivery: не принято", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1))
p.edge("fbn", "fbx", "резервное оповещение", exit=(1, 0.5), entry=(0, 0.5), lpos=0.6)
p.edge("pdg", "pd", "Events API v2", exit=(1, 0.3), entry=(0, round(36 / 160, 4)))
p.edge("pd", "pdg", "Webhooks v3", EDGE_DASH, exit=(0, round(90 / 160, 4)), entry=(1, 0.75))
p.edge("pd", "duty", "уведомления", EDGE_ORTHO, points=[(2120, 444), (2120, 60)], exit=(1, 0.15), entry=(1, round(30 / 110, 4)))
p.edge("fbx", "duty", "письмо, webhook" if MVP else "SMS, звонок, мессенджер", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(rx + 120, 110)], exit=(0.5, 0), entry=(1, round(80 / 110, 4)))
p.edge("duty", "graf", "", EDGE_ORTHO, points=[(1380, 15), (2170, 15), (2170, 835)], exit=(round(200 / 240, 4), 0), entry=(1, 0.5))
p.text("открывает из<br>дашборда инцидентов", 2180, 680, 140, 40, NOTE + "fontSize=10;fontColor=#404040;")
p.edge("ctxb", "graf", "дашборды, аннотации<br>[HTTPS API]", exit=(1, 0.5), entry=(0, round(60 / 120, 4)))
p.edge("graf", "stores", "запросы панелей", exit=(0.5, 1), entry=(0.5, 0))
if TGT:
    r4 = 1010
    p.node("ch", c4("ClickHouse", "Container: ClickHouse, 3 узла", "История 13 мес."), R3X[1], r4, 170, R3H, DB)
    p.node("hist", c4("History Service", "Container: Go", "История событий и тревог, отчёты"), R3X[2], r4, R3W, R3H, CONT)
    p.node("corr", c4("Correlation Engine", "Container: Go", "Объединяет тревоги в инциденты по топологии, вероятная причина"), R3X[3], r4, R3W, R3H, CONT)
    p.node("itsm", c4("ITSM Sync", "Container: Go", "Обмен КЕ, бизнес-услугами и инцидентами с корпоративной CMDB и ITSM"), R3X[4] + 20, r4, 176, R3H, CONT)
    p.node("itsmx", c4("Корпоративная CMDB и ITSM", "External System", ""), rx, 1040, 240, 90, EXT)
    p.edge("hist", "ch", "SQL", exit=(0, 0.5), entry=(1, 0.5))
    p.edge("norm", "hist", "events.norm", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
    p.edge("alert", "corr", "alerts.<br>changed", exit=(0.3, 1), entry=(0.3, 0))
    p.edge("corr", "alert", "incidents.<br>changed", EDGE_DASH, exit=(0.7, 0), entry=(0.7, 1))
    p.edge("cmdb", "itsm", "cmdb.changed", EDGE_DASH, exit=(0.6, 1), entry=(round((R3X[4] + 0.6 * R3W - R3X[4] - 20) / 176, 4), 0))
    p.edge("itsm", "itsmx", "REST", exit=(1, 0.5), entry=(0, 0.5))
p.text("Все движки читают конфигурацию и пишут состояние в PostgreSQL; на схеме показаны основные связи. "
       "Источники подтверждаются только после записи события в шину, поэтому потеря связи не даёт ни потерь, ни дублей. "
       "Секреты хранятся только в OpenBao, в PostgreSQL лежат ссылки. NATS, PostgreSQL" + ("" if MVP else ", ClickHouse") + " и OpenBao — готовые продукты, "
       "на компоненты не детализируются. " + ("Нагрузка: до 5000 событий в минуту (≈85 в секунду)." if MVP else "Нагрузка: до 20 000 событий в минуту (≈330 в секунду), история 13 месяцев в ClickHouse."),
       BX, 1090 if MVP else CMY + 40, 1700, 44)

# =====================================================================
# 3. C4 L3 — Connector Runtime
# =====================================================================
p = Page("C4-3 Connector Runtime", "crt"); pages.append(p)
p.text("<b>C4 · Уровень 3 · Компоненты Connector Runtime: исполнение low-code коннекторов</b>", 40, 20, 1300, 30, TITLE)
CW = 190
XS = [380 + i * 220 for i in range(6)]
Y1, Y2, Y3 = 240, 430, 640
p.node("nats", c4("NATS JetStream", "Container", "ingest.in, events.raw, events.dlq, connector.query, connector.command"), XS[0], 90, XS[5] + CW - XS[0], 70, BUS)
p.node("bnd", "<b>Connector Runtime</b> [Container: Go]", XS[0] - 40, 210, XS[5] + CW - XS[0] + 80, 600, BOUND + "align=right;spacingRight=10;")
p.node("src", c4("Источник", "External System", "Система мониторинга, её БД, облако, очередь"), 40, Y2, 200, 310, EXT)
p.node("trig", c4("Trigger", "Component", "Расписание, webhook из ingest.in, подписка на очередь, событие облака"), XS[0], Y1, CW, 120, COMP)
p.node("sysl", c4("Syslog Listener", "Component", "Приём syslog TLS 6514 за балансировщиком"), XS[3], Y1, 150, 100, COMP)
p.node("dev", c4("Сетевые устройства", "External System", "syslog TLS 6514; 514 только в изолированном сегменте"), XS[5] + CW + 90, Y1 - 10, 200, 120, EXT)
p.edge("dev", "sysl", "syslog TLS 6514", exit=(0, round(60 / 120, 4)), entry=(1, 0.5))
p.edge("sysl", "parse", "", exit=(0.5, 1), entry=(round(75 / CW, 4), 0))
p.node("qry", c4("Query Responder", "Component", "connector.query: метрики для Rule Engine по запросу"), XS[1], Y1, CW, 100, COMP)
p.node("cmd", c4("Command Executor", "Component", "connector.command: ack, закрытие, комментарий в источнике"), XS[2], Y1, CW, 100, COMP)
p.node("fetch", c4("Fetch Blocks", "Component", "HTTP REST / GraphQL / JSON-RPC, SQL, очереди, облака; секреты из OpenBao"), XS[0], Y2, CW, 120, COMP)
p.node("page", c4("Pagination & Cursor", "Component", "Страницы, since / offset / id > курсор; курсор ещё не сдвинут"), XS[1], Y2, CW, 120, COMP)
p.node("parse", c4("Parser", "Component", "JSON, XML, CSV, regex, Grok, key=value, syslog; разбиение пачки; ошибки → events.dlq"), XS[3], Y2, CW, 120, COMP)
p.node("inbox", c4("Inbox Dedup", "Component", "Идемпотентность: источник + external_id уже видели?"), XS[4], Y2, CW, 120, COMP)
p.node("ack", c4("Source Acknowledger", "Component", "ack по API, UPDATE в БД, ack сообщения; потом сдвиг курсора"), XS[0], Y3, CW, 110, COMP)
p.node("pub", c4("Publisher", "Component", "Пишет в events.raw и ждёт подтверждения JetStream"), XS[4], Y3, CW, 110, COMP)
p.node("pg", c4("PostgreSQL", "Container", "версии коннекторов, курсоры, inbox"), XS[0] - 15, 940, 220, 80, DB)
BIDIR = EDGE + "startArrow=block;startFill=1;startSize=6;"
p.edge("nats", "trig", "ingest.in", exit=(round((XS[0] + CW / 2 - XS[0]) / (XS[5] + CW - XS[0]), 4), 1), entry=(0.5, 0))
NW = XS[5] + CW - XS[0]
p.edge("qry", "nats", "connector.query", BIDIR, exit=(0.5, 0), entry=(round((XS[1] + CW / 2 - XS[0]) / NW, 4), 1))
p.edge("nats", "cmd", "connector.command", exit=(round((XS[2] + CW / 2 - XS[0]) / NW, 4), 1), entry=(0.5, 0))
p.edge("parse", "nats", "events.dlq", exit=(round(175 / CW, 4), 0), entry=(round((XS[3] + 175 - XS[0]) / NW, 4), 1), lpos=-0.55)
p.edge("trig", "fetch", "запуск", exit=(0.5, 1), entry=(0.5, 0))
p.edge("qry", "fetch", "запрос метрик", EDGE_ORTHO, points=[(XS[1] + CW / 2, 375), (XS[0] + 0.75 * CW, 375)], exit=(0.5, 1), entry=(0.75, 0))
p.edge("cmd", "fetch", "команда", EDGE_ORTHO, points=[(XS[2] + CW / 2, 400), (XS[0] + 0.92 * CW, 400)], exit=(0.5, 1), entry=(0.92, 0))
p.edge("fetch", "src", "запрос", exit=(0, 0.5), entry=(1, round(60 / 310, 4)))
p.edge("fetch", "page")
p.edge("page", "parse")
p.edge("parse", "inbox")
p.edge("inbox", "pub", "новые", exit=(0.5, 1), entry=(0.5, 0))
p.edge("pub", "nats", "events.raw", EDGE_ORTHO, points=[(XS[5] + CW / 2, Y3 + 55)], exit=(1, 0.5), entry=(round((XS[5] + CW / 2 - XS[0]) / NW, 4), 1))
p.edge("pub", "ack", "записано", exit=(0, 0.5), entry=(1, 0.5))
p.edge("ack", "src", "подтверждение", lpos=0.3, exit=(0, 0.5), entry=(1, round((Y3 + 55 - Y2) / 310, 4)))
p.edge("ack", "pg", "курсор", EDGE_DASH, exit=(0.5, 1), entry=(round((XS[0] + CW / 2 - XS[0] + 15) / 220, 4), 0))
p.text("Push-источники обслуживает Ingest Gateway: он отвечает 2xx только после записи в ingest.in, дальше тот же конвейер. "
       "Сетевые устройства шлют syslog в Syslog Listener за балансировщиком (6514, syslog TLS; 514 только в изолированном сегменте по согласованию с ИБ). "
       "Ack источнику и сдвиг курсора выполняются строго после подтверждения записи в шину. Если ack потерялся и источник отдал событие повторно, "
       "его отбрасывает Inbox Dedup. Итог: at-least-once доставка без дублей. Неразобранные события уходят в events.dlq. "
       "Query Responder и Command Executor обращаются к источнику теми же блоками Fetch Blocks.", XS[0] - 40, 1050, XS[5] + CW - XS[0] + 80, 70)

# =====================================================================
# 3b. C4 L3 — Alert Engine и PagerDuty Gateway
# =====================================================================
p = Page("C4-3b Alert Engine и PagerDuty", "cmp"); pages.append(p)
p.text("<b>C4 · Уровень 3 · Компоненты Alert Engine и PagerDuty Gateway</b>", 40, 20, 1200, 30, TITLE)
p.node("bA", "<b>Alert Engine</b> [Container: Go]", 40, 90, 740, 580, BOUND)
p.node("bB", "<b>PagerDuty Gateway</b> [Container: Go]", 1000, 90, 740, 580, BOUND)
p.node("nats", c4("NATS JetStream", "Container", "events.norm, alerts.changed, pd.outbox, pd.inbound"), 820, 130, 140, 500, BUS)
CW, CH = 200, 100
a0, a1, a2 = 60, 300, 540
y1, y2, y3 = 130, 320, 510
p.node("cons", c4("Event Consumer", "Component", "Читает events.norm и статусы из pd.inbound"), a2, y1, CW, CH, COMP)
p.node("cires", c4("CI Resolver", "Component", "Находит КЕ в карте CMDB по меткам и облачным ID"), a1, y1, CW, CH, COMP)
p.node("dedup", c4("Cross-source Dedup", "Component", "Ключ = КЕ + сигнал: одно событие из любого числа источников"), a0, y1, CW, CH, COMP)
p.node("supp", c4("Suppression", "Component", "Окна обслуживания, silences, правила игнора"), a0, y2, CW, CH, COMP)
p.node("impact", c4("Service Impact", "Component", "Влияние на ИТ-сервисы и бизнес-услуги по РСМ"), a1, y2, CW, CH, COMP)
p.node("corr", c4("Correlation", "Component", "RED сервиса + USE его КЕ = один инцидент"), a2, y2, CW, CH, COMP)
p.node("sync", c4("Source Sync-back", "Component", "ack и silence обратно в источники блоками коннектора"), a0, y3, CW, CH, COMP)
p.node("life", c4("Lifecycle", "Component", "open → ack → resolved; resolve, когда все источники в норме"), a1, y3, CW, CH, COMP)
p.node("pub", c4("Change Publisher", "Component", "Публикует alerts.changed"), a2, y3, CW, CH, COMP)
p.edge("nats", "cons", "events.norm,<br>pd.inbound", exit=(0, 0.1), entry=(1, 0.5))
p.edge("cons", "cires", "")
p.edge("cires", "dedup", "")
p.edge("dedup", "supp", "")
p.edge("supp", "impact", "")
p.edge("impact", "corr", "")
p.edge("corr", "life", "")
p.edge("life", "pub", "")
p.edge("life", "sync", "")
p.edge("pub", "nats", "alerts.changed", exit=(1, 0.5), entry=(0, 0.9))
b0, b1, b2 = 1020, 1260, 1500
p.node("listen", c4("Alert Listener", "Component", "Читает alerts.changed"), b0, y1, CW, CH, COMP)
p.node("router", c4("Service Router", "Component", "routing_key сервиса PagerDuty по РСМ, severity, меткам"), b1, y1, CW, CH, COMP)
p.node("mapper", c4("Event Mapper", "Component", "Шаблон → payload: summary, severity, dedup_key, links"), b2, y1, CW, CH, COMP)
p.node("sender", c4("Sender", "Component", "Outbox, повторы с backoff, учёт лимитов API"), b2, y2, CW, CH, COMP)
p.node("recv", c4("Webhook Receiver", "Component", "Проверка подписи, acknowledged, resolved, reassigned"), b1, y2, CW, CH, COMP)
p.node("stpub", c4("Status Publisher", "Component", "Находит тревогу по dedup_key, публикует pd.inbound"), b0, y2, CW, CH, COMP)
p.edge("nats", "listen", "alerts.changed", exit=(1, 0.1), entry=(0, 0.5))
p.edge("listen", "router", "")
p.edge("router", "mapper", "")
p.edge("mapper", "sender", "")
p.edge("recv", "stpub", "")
p.edge("stpub", "nats", "pd.inbound", exit=(0, 0.5), entry=(1, 0.48))
p.node("pg", c4("PostgreSQL", "Container", ""), 300, 700, 200, 80, DB)
p.node("src", c4("Системы-источники", "External System", "через коннекторы"), 40, 700, 230, 80, EXT)
p.node("pdx", c4("PagerDuty", "External System", "Events API v2, Webhooks v3"), 1500, 700, 200, 80, EXT)
p.edge("life", "pg", "SQL")
p.edge("sync", "src", "", exit=(0.5, 1), entry=(0.4348, 0))
p.edge("sender", "pdx", "trigger, resolve", exit=(0.5, 1), entry=(0.5, 0))
p.edge("pdx", "recv", "webhook", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1360, 740)], exit=(0, 0.5), entry=(0.5, 1))

# =====================================================================
# 3c. C4 L3 — CMDB Discovery
# =====================================================================
p = Page("C4-3 CMDB Discovery", "cdb"); pages.append(p)
p.text("<b>C4 · Уровень 3 · Компоненты CMDB Discovery: карта КЕ из данных мониторинга и облаков</b>", 40, 20, 1300, 30, TITLE)
SW_, SH_ = 210, 76
for i, (k, n, d) in enumerate([("mon", "Системы мониторинга", "хосты, таргеты, метки, группы"),
                               ("db", "БД систем мониторинга", "таблицы хостов и объектов"),
                               ("cloud", "Облачные провайдеры", "ресурсы, теги, группы, ID"),
                               ("stream", "Потоки событий", "имена хостов и сервисов в событиях")]):
    p.node(k, c4(n, "External System", d), 40, 110 + i * 92, SW_, SH_, EXT)
p.node("bnd", "<b>CMDB Discovery</b> [Container: Go, 2 реплики, leader election]", 290, 90, 1000, 510, BOUND + "align=right;spacingRight=10;")
CW, CH = 200, 100
p.node("scan", c4("Inventory Connectors", "Component", "Коннекторы из того же конструктора: забирают инвентарь по расписанию и по событию"), 310, 110, CW, 360, COMP)
p.node("lead", c4("Leader Election", "Component", "Lease в NATS KV: обнаружение ведёт одна реплика, вторая в резерве"), 310, 495, CW, 90, COMP)
p.node("ident", c4("Identity Resolver", "Component", "FQDN, IP, теги → одна КЕ; плавающие ID облака → устойчивая КЕ"), 560, 130, CW, CH, COMP)
p.node("rel", c4("Relation Builder", "Component", "Связи: сервис → КЕ по меткам; инстанс → группа; БД → хост"), 800, 130, CW, CH, COMP)
p.node("diff", c4("Change Detector", "Component", "Сравнивает с текущей картой: новые, изменённые, пропавшие КЕ"), 1040, 130, CW, CH, COMP)
p.node("rules", c4("Discovery Rules", "Component", "Low-code правила: что считать КЕ, тип, какие метки дают связи"), 560, 330, CW, CH, COMP)
p.node("mapapi", c4("Map API", "Component", "Граф КЕ и сервисов для UI, Alert Engine и Context Builder"), 800, 330, CW, CH, COMP)
p.node("apply", c4("Change Applier", "Component", "Автоприменение или черновик на ревью; версии карты"), 1040, 330, CW, CH, COMP)
p.node("pg", c4("PostgreSQL", "Container", "КЕ, связи, история ID, версии карты"), 1400, 330, 210, 100, DB)
p.node("nats", c4("NATS JetStream", "Container", "cmdb.changed"), 1400, 150, 210, 80, BUS)
p.node("api", c4("Core API и веб-интерфейс", "Container", "карта CMDB, ревью изменений"), 1400, 490, 210, 90, CONT)
for i, k in enumerate(("mon", "db", "cloud", "stream")):
    y = 110 + i * 92 + SH_ / 2
    p.edge(k, "scan", "", exit=(1, 0.5), entry=(0, round((y - 110) / 360, 4)))
p.edge("scan", "ident", "", exit=(1, 0.1944), entry=(0, 0.5))
p.edge("ident", "rel", "")
p.edge("rel", "diff", "")
p.edge("diff", "apply", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("rules", "ident", "", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1))
p.edge("rules", "rel", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(700, 280), (840, 280)], exit=(0.7, 0), entry=(0.2, 1))
p.edge("apply", "pg", "SQL")
p.edge("apply", "nats", "cmdb.changed", EDGE_ORTHO, points=[(1345, 350), (1345, 190)], exit=(1, 0.2), entry=(0, 0.5))
p.edge("mapapi", "pg", "SQL", EDGE_ORTHO, points=[(960, 455), (1505, 455)], exit=(0.8, 1), entry=(0.5, 1))
p.edge("api", "mapapi", "REST", EDGE_ORTHO, points=[(900, 535)], exit=(0, 0.5), entry=(0.5, 1))
p.edge("lead", "scan", "запуск на лидере", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1), lpos=0, loff=(52, 0))
p.edge("lead", "nats", "lease в NATS KV", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(410, 630), (1650, 630), (1650, 190)],
       exit=(0.5, 1), entry=(1, 0.5))
p.text("КЕ, которую ни один источник не видит дольше заданного срока, помечается устаревшей, а не удаляется. "
       "Ручные правки инженера в карте имеют приоритет над обнаружением и не перезаписываются. "
       "Облачный инстанс, пересозданный с новым ID, не порождает новую КЕ: он привязывается к той же логической группе.", 40, 670, 1620, 60)

# =====================================================================
# 4. C4 — развёртывание
# =====================================================================
p = Page("C4-4 Развёртывание", "dep"); pages.append(p)
p.text("<b>C4 · Развёртывание · Kubernetes (пилот: 3 VM и Docker Compose)</b>" if MVP else "<b>C4 · Развёртывание · Kubernetes, основная площадка и тёплый резерв</b>", 40, 20, 1200, 30, TITLE)
items = [
    ("web", "Веб-интерфейс", "nginx, ×2"), ("api", "Core API", "×2" if MVP else "×3"), ("ingest", "Ingest Gateway", "×2" if MVP else "×3"),
    ("conn", "Connector Runtime", ("×2" if MVP else "×4") + ", коннекторы шардируются"), ("norm", "Normalizer", "×2" if MVP else "×4"),
    ("rule", "Rule Engine", "×2, шардирование правил"),
    ("alert", "Alert Engine", "×2" if MVP else "×3"), ("pdg", "PagerDuty Gateway", "×2"), ("fbn", "Fallback Notifier", "×2, независим от PagerDuty Gateway"),
]
if MVP:
    items += [("cmdb", "CMDB Discovery", "×2, leader election"), ("ctxb", "Context Builder", "×2")]
else:
    items += [("cmdb", "CMDB Discovery", "×2, leader election"), ("hist", "History Service", "×2"), ("corr", "Correlation Engine", "×2"),
              ("itsm", "ITSM Sync", "×2, leader election"), ("ctxb", "Context Builder", "×2")]
items += [("graf", "Grafana OSS", "×2, только если нет корпоративной Grafana; HTTPS 443 от Context Builder"),
          ("wd", "Watchdog", "×2, leader election; heartbeat наружу")]
CW, CH = 215, 90
cols = [100, 350, 600]
RY0, RS = 290, 105
nrows = (len(items) + 2) // 3
KB = RY0 + nrows * RS + 20
p.node("k8s", "<b>Kubernetes-кластер</b> [3 app-узла 8 vCPU / 16 ГБ; 3 узла данных 4 vCPU / 16 ГБ, SSD 800 ГБ; 3 control-plane 2 vCPU / 4 ГБ или managed Kubernetes; 5000 событий/мин]" if MVP else
       "<b>Kubernetes-кластер, основная площадка</b> [5 app-узлов 12 vCPU / 24 ГБ; 3 узла данных 8 vCPU / 32 ГБ, SSD 2 ТБ; 3 узла ClickHouse 8 vCPU / 32 ГБ, SSD 4 ТБ; 3 control-plane 2 vCPU / 4 ГБ; 20 000 событий/мин]",
       40, 190, 1210, KB + 75 - 190, NODE_BOUND + "verticalAlign=bottom;spacingBottom=6;")
p.node("nsapp", "<b>namespace: umbrella-app</b> [Deployment, HPA]", 70, 240, 780, KB - 240, NODE_BOUND)
p.node("nsdata", "<b>namespace: umbrella-data</b> [StatefulSet]", 880, 240, 340, KB - 240, NODE_BOUND)
for i, (k, n, d) in enumerate(items):
    st = EXT_DASHED if k == "graf" else CONT
    p.node(k, c4(n, "Pod", d), cols[i % 3], RY0 + (i // 3) * RS, CW, CH, st)
p.edge("ctxb", "graf", "", exit=(1, 0.5), entry=(0, 0.5))
DXX, DWW = 905, 290
p.node("nats", c4("NATS JetStream", "StatefulSet ×3", "R3; ingest.in, events.raw, events.norm 24 ч; events.dlq 14 дней; notify.log 30 дней; остальные 7 дней; " + ("100 ГБ" if MVP else "250 ГБ")), DXX, 290, DWW, 100, BUS)
p.node("pg", c4("PostgreSQL", "CloudNativePG / Patroni",
                "Primary + синхронная реплика (без строгого режима), PITR, archive_timeout ≤ 60 с; 500 ГБ" if MVP else
                "Primary + синхронная реплика, PITR; асинхронная реплика на резервной площадке; горячие данные 7 дней"), DXX, 420, DWW, 120, DB)
if TGT:
    p.node("ch", c4("ClickHouse", "Container: ClickHouse, 3 узла", "Keeper ×3; история 13 месяцев, SSD 4 ТБ; ежедневный инкрементальный BACKUP в S3"), DXX, 570, DWW, 120, DB)
p.node("sec", c4("Хранилище секретов", "Container: OpenBao ×3, Raft", "auto-unseal или Шамир 3 из 5; секреты подам через agent injector; снапшоты в S3"), DXX, 570 if MVP else 720, DWW, 120, DB)
# справа: внешнее
RX, RW = 1400, 280
p.node("corp", c4("Корпоративные системы", "External System", "Исходящие из зоны приложений: IdP и каталог: LDAPS 636, SCIM 443; SIEM: syslog TLS 6514; корпоративная Grafana: HTTPS 443"), 70, 70, 560, 90, EXT)
p.edge("nsapp", "corp", "", exit=(round(150 / 780, 4), 0), entry=(round(150 / 560, 4), 1))
p.node("lb", c4("Балансировщик и DMZ", "2 VM 2 vCPU / 4 ГБ", "Reverse proxy + WAF, egress-прокси; в кластер через Ingress-контроллер ×2 на разных узлах"), RX, 60, RW, 120, EXT)
p.edge("lb", "nsapp", "HTTPS 8443 (mTLS)", EDGE_ORTHO, points=[(760, 120)], exit=(0, 0.5), entry=(round(690 / 780, 4), 0))
p.node("s3", c4("S3-совместимое хранилище", "другая площадка или регион", "WAL-архив и бэкапы PostgreSQL, снапшоты OpenBao" + ("" if MVP else ", бэкапы ClickHouse")), RX, 480, RW, 120, EXT)
p.edge("pg", "s3", "HTTPS 443", exit=(1, round(100 / 120, 4)), entry=(0, round(40 / 120, 4)))
if TGT:
    p.node("dr", c4("Резервная площадка", "Kubernetes, тёплый резерв",
                    "3 app-узла 12 vCPU / 24 ГБ, 3 узла данных 8 vCPU / 32 ГБ SSD 2 ТБ, control plane ×3, 2 VM DMZ; сервисы по 1 реплике, реплика PostgreSQL, "
                    "зеркало NATS, снапшоты OpenBao каждые 15 мин; RTO ≤ 15 мин"), RX, 280, RW, 180, REPL)
    p.edge("nats", "dr", "NATS 7422", EDGE_DASH, exit=(1, 0.5), entry=(0, round(60 / 180, 4)))
    p.edge("pg", "dr", "WAL 5432", EDGE_DASH, exit=(1, round(20 / 120, 4)), entry=(0, round(160 / 180, 4)))
HBY = KB - 140
p.node("hb", c4("Внешний heartbeat", "External System", "Второй контур: поднимает тревогу, если Umbrella молчит"), RX, HBY, RW, 110, EXT)
wdx, wdy = cols[(len(items) - 1) % 3], RY0 + ((len(items) - 1) // 3) * RS
p.edge("wd", "hb", "ping каждые 60 с", EDGE_ORTHO, points=[(wdx + CW / 2, KB + 30), (1350, KB + 30), (1350, HBY + 55)], exit=(0.5, 1), entry=(0, 0.5))
p.text("Поды без состояния масштабируются горизонтально; NATS распределяет работу через queue group (общий durable consumer). "
       "Watchdog раз в минуту прогоняет тестовое событие до PagerDuty Gateway, раз в сутки проверяет резервные каналы и шлёт heartbeat во внешний сервис. "
       "Если Umbrella перестала работать, внешний сервис сам поднимает тревогу (например, через свой сервис в PagerDuty): зонтичный мониторинг не может следить сам за собой. "
       "Grafana: используется корпоративная; если её нет, Grafana OSS ×2 разворачивается в зоне приложений, её база — в PostgreSQL. Все секреты — только в OpenBao.",
       40, KB + 100, 1600, 60)

# =====================================================================
# 4b. C4 — инфраструктура и сети
# =====================================================================
p = Page("C4-5 Инфраструктура", "inf"); pages.append(p)
p.text("<b>C4 · Инфраструктура: сетевые зоны, порты и внешние доступы</b>", 40, 20, 1200, 30, TITLE)
SMALL = NOTE + "fontSize=10;fontColor=#404040;"
# зоны
p.node("z_net", "<b>Интернет</b>", 40, 80, 310, 760, NODE_BOUND)
p.node("z_dmz", "<b>DMZ</b>", 450, 80, 250, 620, NODE_BOUND)
p.node("z_app", "<b>Зона приложений</b> [Kubernetes, namespace umbrella-app]", 820, 80, 1240, 620, NODE_BOUND)
p.node("z_data", "<b>Зона данных</b> [umbrella-data]", 2140, 80, 300, 440 if TGT else 360, NODE_BOUND)
p.node("z_corp", "<b>Внутренняя сеть компании</b>", 820, 780, 1620, 260, NODE_BOUND)
p.node("z_s3", "<b>Другая площадка или регион</b>", 2560, 80, 260, 200, NODE_BOUND)
# интернет
IX, IW = 110, 220
p.node("hb", c4("Внешний heartbeat", "независимый сервис", ""), IX, 115, IW, 60, EXT)
p.node("fbx", c4("Резервные каналы в интернете", "Облачный мессенджер (webhook)" if MVP else
                 "SMS- и голосовой шлюз (основной и резервный провайдер), облачные мессенджеры, запасная платформа", ""), IX, 195, IW, 80, EXT)
p.node("pd", c4("PagerDuty", "events.pagerduty.com, webhooks", ""), IX, 300, IW, 80, EXT)
p.node("cloud", c4("Облачные провайдеры", "API облака, события; AWS и др.", ""), IX, 410, IW, 70, EXT)
p.node("mob", c4("Телефоны дежурных", "звонок, push, SMS от PagerDuty; сообщения резервных каналов", ""), 45, 740, 285, 80, PERSON)
# DMZ
p.node("egress", c4("Egress-прокси", "Infrastructure", "Исходящий трафик только по списку разрешённых адресов"), 480, 110, 170, 390, CONT)
p.node("cbgw", c4("Reverse proxy + WAF", "Infrastructure", "Приём webhooks PagerDuty и событий облаков" + ("" if MVP else ", ответов SMS-шлюза и кнопок мессенджеров") + ": IP-списки, проверка подписи"), 480, 545, 170, 140, CONT)
# приложения
SVX, SVW = 860, 1160
p.node("svc", c4("Сервисы Umbrella", "Pods ×1–4, см. сайзинг",
                 "Веб-интерфейс, Core API, Ingest Gateway, Connector Runtime, Rule Engine, Normalizer, Alert Engine, CMDB Discovery, "
                 "PagerDuty Gateway, Fallback Notifier, Watchdog, Context Builder" + ("" if MVP else ", History Service, Correlation Engine, ITSM Sync")
                 + ". Между собой — TLS, секреты из OpenBao"), SVX, 120, SVW, 350, CONT)
p.node("ingress", c4("Ingress-контроллер ×2", "разные узлы", "TLS 443, syslog TLS 6514"), SVX, 570, 240, 100, CONT)
# данные
DX, DW = 2170, 240
if MVP:
    DATA = [("nats", c4("NATS JetStream", "3 узла", "R3, 4222/TLS"), BUS, 110, 70, "4222 TLS"),
            ("pg", c4("PostgreSQL", "primary + реплика", "Patroni, 5432/TLS"), DB, 200, 100, "5432 TLS"),
            ("sec", c4("Хранилище секретов", "OpenBao ×3, Raft", "8200/TLS"), DB, 330, 100, "8200 TLS")]
else:
    DATA = [("nats", c4("NATS JetStream", "3 узла", "R3, 4222/TLS"), BUS, 110, 70, "4222 TLS"),
            ("pg", c4("PostgreSQL", "primary + реплика", "Patroni, 5432/TLS"), DB, 200, 90, "5432 TLS"),
            ("ch", c4("ClickHouse", "3 узла", "9440/TLS (native)"), DB, 310, 90, "9440 TLS"),
            ("sec", c4("Хранилище секретов", "OpenBao ×3, Raft", "8200/TLS"), DB, 420, 90, "8200 TLS")]
for k, lab, st, y, h, port in DATA:
    p.node(k, lab, DX, y, DW, h, st)
    cy = y + h / 2
    p.edge("svc", k, port, exit=(1, round((cy - 120) / 350, 4)), entry=(0, 0.5))
p.node("s3", c4("S3-хранилище", "Infrastructure", "WAL-архив, бэкапы PostgreSQL" + ("" if MVP else " и ClickHouse") + ", снапшоты OpenBao"), 2590, 130, 200, 110, EXT)
p.edge("z_data", "s3", "HTTPS 443", exit=(1, round(105 / (440 if TGT else 360), 4)), entry=(0, 0.5))
if TGT:
    p.node("z_dr", "<b>Резервная площадка</b>", 2560, 340, 260, 200, NODE_BOUND)
    p.node("dr", c4("Umbrella в тёплом резерве", "тёплый резерв: DMZ, приложения, данные", "Реплика PostgreSQL, зеркало NATS, OpenBao из снапшотов"), 2590, 380, 200, 130, REPL)
    p.edge("dr", "z_data", "5432 (WAL),<br>7422 (NATS) TLS", exit=(0, 0.4), entry=(1, round((432 - 80) / 440, 4)))
    p.edge("dr", "s3", "443: снапшоты", exit=(0.85, 0), entry=(0.85, 1))
# внутренняя сеть
CY2, BW2 = 840, 140
CXS = [1150 + i * 155 for i in range(6)]
p.node("users", c4("Инженеры и дежурные", "браузер, VPN", ""), SVX, CY2, 200, 100, PERSON)
INT = [("mon", "Системы мониторинга и их БД", "API, SQL read-only", "HTTPS / API,<br>СУБД TLS"),
       ("stream", "Брокеры, почта, syslog-устройства", "очереди, IMAPS, syslog", "брокер,<br>IMAPS"),
       ("idp", "IdP и каталог (AD)", "OIDC, LDAPS, SCIM", "LDAPS 636,<br>OIDC 443"),
       ("siem", "SIEM", "аудит", "syslog TLS<br>6514"),
       ("relay", "SMTP-релей и внутренний мессенджер", "резервные каналы", "SMTP 587,<br>HTTPS 443"),
       ("graf", "Grafana", "корпоративная или Grafana OSS ×2 в зоне приложений", "HTTPS 443<br>(Context Builder)")]
for (k, n, d, lab), x in zip(INT, CXS):
    p.node(k, c4(n, d, ""), x, CY2, BW2, 100, EXT + "fontSize=11;")
    p.edge("svc", k, lab, exit=(round((x + BW2 / 2 - SVX) / SVW, 4), 1), entry=(0.5, 0))
p.node("stores", c4("Хранилища метрик, логов и трейсов", "источники данных Grafana", ""), 2190, CY2, 200, 100, EXT + "fontSize=11;")
p.edge("graf", "stores", "запросы", exit=(1, 0.5), entry=(0, 0.5))
p.edge("users", "graf", "HTTPS 443 (Grafana)", EDGE_ORTHO, points=[(SVX + 100, 1000), (CXS[5] + BW2 / 2, 1000)], exit=(0.5, 1), entry=(0.5, 1))
p.edge("z_corp", "ingress", "HTTPS 443: веб-UI, webhooks, SCIM;<br>syslog TLS 6514 от устройств", exit=(round((1080 - 820) / 1620, 4), 0), entry=(round((1080 - SVX) / 240, 4), 1))
p.edge("ingress", "svc", "HTTPS 8443 (mTLS)", exit=(0.5, 0), entry=(round(120 / SVW, 4), 1))
p.edge("cbgw", "ingress", "HTTPS 443", exit=(1, round((640 - 545) / 140, 4)), entry=(0, 0.7))
p.edge("svc", "egress", "443 через прокси", exit=(0, round((185 - 120) / 350, 4)), entry=(1, round((185 - 110) / 390, 4)))
# egress → интернет
for k, (y, h), lab in [("hb", (115, 60), "HTTPS 443"), ("fbx", (195, 80), "HTTPS 443"),
                       ("pd", (300, 80), "Events API 443" if MVP else "Events API,<br>REST 443"), ("cloud", (410, 70), "API облака 443")]:
    cy = y + h / 2
    p.edge("egress", k, lab, exit=(0, round((cy - 110) / 390, 4)), entry=(1, 0.5))
# входящие (по левому коридору)
p.edge("pd", "cbgw", "Webhooks v3", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(85, 365), (85, 655)], exit=(0, 0.8125), entry=(0, round((655 - 545) / 140, 4)))
p.edge("cloud", "cbgw", "события облака", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(100, 445), (100, 585)], exit=(0, 0.5), entry=(0, round((585 - 545) / 140, 4)))
p.edge("pd", "mob", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(70, 325), (70, 740)], exit=(0, round(25 / 80, 4)), entry=(round(25 / 285, 4), 0))
p.edge("fbx", "mob", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(55, 235), (55, 740)], exit=(0, 0.5), entry=(round(10 / 285, 4), 0))
p.text("Сплошные стрелки: исходящие соединения (от инициатора), пунктир: входящие webhooks и доставка дежурным. "
       "Весь трафик шифруется TLS; наружу Umbrella ходит только через egress-прокси. Внутри кластера Ingress передаёт запросы сервисам по HTTPS 8443 с mTLS. "
       "Syslog-устройства шлют события на балансировщик 6514 (syslog TLS; 514 только в изолированном сегменте по согласованию с ИБ). "
       "Порты источников зависят от системы и задаются в коннекторе. S3 находится вне площадки (другая площадка или регион)."
       + ("" if MVP else " Резервная площадка повторяет эту сегментацию, включая DMZ; межплощадочный канал только для репликации: 5432, 7422 и 443 к S3, TLS."),
       40, 1070, 2780, 50)

# =====================================================================
# 5. Конструктор коннекторов
# =====================================================================
p = Page("Конструктор коннекторов", "bld"); pages.append(p)
p.text("<b>Low-code конструктор коннекторов: подключение к любому источнику из блоков</b>", 40, 20, 1400, 30, TITLE)
p.text("<b>Палитра блоков</b>", 40, 60, 300, 24, NOTE + "fontSize=14;")
CATS = [
    ("Триггеры", "расписание, webhook из ingest.in, подписка на очередь, событие облака, ручной запуск", L_BIZ),
    ("Получение", "HTTP REST / GraphQL / JSON-RPC / SOAP, SQL-запрос, очередь (Kafka, AMQP, NATS), syslog и файлы, почта, SNMP trap, API облака", L_SVC),
    ("Авторизация", "токен, Basic, OAuth2, mTLS, IAM-роль облака, логин БД; секреты из OpenBao", L_TEAM),
    ("Управление", "пагинация, цикл по элементам, условие, задержка, повтор с backoff", L_MON),
    ("Парсинг", "JSON, XML, CSV, regex, Grok, key=value, syslog, разбиение пачки", L_CI),
    ("Преобразование", "маппинг полей (JSONata / CEL), шаблон, обогащение из CMDB и API облака, фильтр", L_CI),
    ("Подтверждение", "ack по API, UPDATE в БД, ack сообщения очереди или ingest.in, сдвиг курсора", L_ALERT),
    ("Выход", "событие, метрика, объект инвентаря", L_ALERT),
]
for i, (n, d, st) in enumerate(CATS):
    p.node(f"cat{i}", f"<b>{escape(n)}</b><br><font style='font-size:10px'>{escape(d)}</font>", 40, 95 + i * 72, 320, 64, st + LEFT)
BX, BS, BW, BH = 400, 170, 150, 70
FLOWS = [
    ("Пример 1 · API системы мониторинга (pull)", [
        ("Расписание<br>30 с", L_BIZ), ("HTTP-запрос<br>since={{cursor}}<br>+ OAuth2", L_SVC), ("Пагинация", L_MON),
        ("Парсер JSON", L_CI), ("Маппинг<br>и шаблон", L_CI), ("Событие<br>в шину", L_ALERT), ("ack в источник<br>+ курсор", L_ALERT)]),
    ("Пример 2 · база данных системы мониторинга", [
        ("Расписание<br>1 мин", L_BIZ), ("SQL: WHERE id &gt;<br>:cursor LIMIT 500", L_SVC), ("Цикл<br>по строкам", L_MON),
        ("Парсер regex<br>поля message", L_CI), ("Маппинг<br>и шаблон", L_CI), ("Событие<br>в шину", L_ALERT), ("UPDATE<br>exported = true", L_ALERT)]),
    ("Пример 3 · облачный провайдер (push)", [
        ("Событие облака<br>из ingest.in", L_BIZ), ("Парсер JSON", L_CI),
        ("Обогащение:<br>теги по ID<br>через API облака", L_CI), ("Маппинг<br>и шаблон", L_CI), ("Событие<br>в шину", L_ALERT), ("ack сообщения<br>ingest.in", L_ALERT)]),
]
for fi, (title, blocks) in enumerate(FLOWS):
    fy = 60 + fi * 165
    p.text(f"<b>{escape(title)}</b>", BX, fy, 900, 24, NOTE + "fontSize=13;")
    for bi, (lab, st) in enumerate(blocks):
        p.node(f"f{fi}b{bi}", lab, BX + bi * BS, fy + 40, BW, BH + 10, st + "fontSize=11;")
        if bi:
            p.edge(f"f{fi}b{bi - 1}", f"f{fi}b{bi}")
p.node("guar", "<b>Гарантия без дублей</b><br><font style='font-size:11px'>"
               "1. Событие записывается в шину, коннектор ждёт подтверждения записи.<br>"
               "2. Только после этого источник получает подтверждение (ack, UPDATE, ack сообщения) и сдвигается курсор. Для push-источников ответ 2xx уже дал Ingest Gateway после записи в ingest.in.<br>"
               "3. Если подтверждение потерялось, источник отдаст событие ещё раз; повтор отбрасывается по ключу источник + external_id.</font>",
       400, 560, 1170, 110, L_TEAM + LEFT + "verticalAlign=top;spacingTop=8;")
p.text("Коннектор хранится как версия: черновик проверяется dry-run на живых данных, публикуется, при росте ошибок откатывается на прошлую версию. "
       "Новые типы блоков добавляются как плагины Connector Runtime, не меняя конвейер. Тем же блочным редактором собираются «Связи контекста» для Grafana: условие → обход карты CMDB → панели → раскладка.", 40, 700, 1530, 40)

# =====================================================================
# 6. Облачные ресурсы и плавающие ID
# =====================================================================
p = Page("Облачные ресурсы и ID", "cld"); pages.append(p)
p.text("<b>Облачные провайдеры: привязка событий с плавающими ID к устойчивой КЕ</b>", 40, 20, 1400, 30, TITLE)
p.node("ev1", "<b>Событие облака · 13:58</b><br><font style='" + MONO + "'>resource: i-0a1b2c3d<br>metric: CPUUtilization 94 %</font>", 40, 100, 320, 80, L_MON + LEFT)
p.node("ev2", "<b>Событие облака · 14:20</b><br><font style='" + MONO + "'>resource: i-7f3c9e01 (после пересоздания)<br>metric: CPUUtilization 91 %</font>", 40, 230, 320, 80, L_MON + LEFT)
p.node("ev3", "<b>Событие без ресурса</b><br><font style='" + MONO + "'>id: 5c1e…-guid<br>text: disk latency high</font>", 40, 360, 320, 80, L_MON + LEFT)
p.node("res", "<b>Identity Resolver: порядок поиска КЕ</b><br><br><font style='font-size:11px'>"
              "1. Устойчивый тег ресурса (service, ci, host)<br><br>"
              "2. Логическая группа: autoscaling group, сервис контейнеров, deployment<br><br>"
              "3. Запрос по ID в API облака: теги и группа<br><br>"
              "4. DNS-имя или IP с проверкой по времени события<br><br>"
              "5. Не нашли: КЕ аккаунта, региона и облачного сервиса + очередь «без КЕ» для инженера</font>",
       420, 100, 420, 320, L_TEAM + LEFT + "verticalAlign=top;spacingTop=10;")
p.node("capi", c4("API облака", "External System", "описание ресурса по ID, теги, группа"), 520, 480, 220, 90, EXT)
p.edge("ev1", "res", "", exit=(1, 0.5), entry=(0, 0.125))
p.edge("ev2", "res", "", exit=(1, 0.5), entry=(0, 0.5313))
p.edge("ev3", "res", "", exit=(1, 0.5), entry=(0, 0.9375))
p.edge("res", "capi", "шаг 3", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.node("ci", "<b>КЕ payments-api</b><br><font style='font-size:11px'>логическая группа payments-asg<br>3 инстанса · теги service=payments</font>", 960, 100, 300, 80, L_CI)
p.node("hist", "<b>История ID</b><br><font style='" + MONO + "'>i-0a1b2c3d  terminated 14:02<br>i-7f3c9e01  active с 14:05</font>", 960, 220, 300, 80, L_MON + LEFT)
p.node("key", "<b>Ключ дедупликации</b><br><font style='" + MONO + "'>payments-api · use.cpu.utilization</font><br><font style='font-size:11px'>оба события → одна тревога</font>", 1320, 100, 290, 80, L_ALERT)
p.node("fb", "<b>Запасной вариант</b><br><font style='font-size:11px'>КЕ «аккаунт / регион / облачный сервис»<br>событие попадает в очередь «без КЕ»,<br>инженер дописывает правило или тег</font>", 960, 330, 300, 90, ST_WARN)
p.edge("res", "ci", "шаги 1–3", exit=(1, 0.125), entry=(0, 0.5))
p.edge("ci", "hist", "", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.edge("res", "fb", "шаг 5", exit=(1, 0.8594), entry=(0, 0.5))
p.edge("ci", "key")
p.text("ID инстанса или GUID ресурса — это атрибут и история КЕ, а не её идентичность: после перезагрузки или пересоздания ID меняется, КЕ остаётся той же. "
       "Устойчивой КЕ считается логическая единица (группа, сервис, deployment), инстансы видны внутри неё. "
       "Для облаков, где ресурс в событии не указан, рекомендуем правило: каждый ресурс обязан нести тег service.", 40, 610, 1570, 60)

# =====================================================================
# 7. Ресурсно-сервисная модель
# =====================================================================
p = Page("РСМ", "rsm"); pages.append(p)
p.text("<b>Ресурсно-сервисная модель (РСМ)</b>", 40, 20, 900, 30, TITLE)
p.text("<b>Метамодель</b>", 40, 60, 300, 24, NOTE + "fontSize=14;")
W, H = 300, 84
p.node("biz", "<b>Бизнес-услуга</b><br><font style='font-size:11px'>Интернет-банк, Колл-центр<br>критичность, SLA, владелец</font>", 40, 100, W, H, L_BIZ)
p.node("svc", "<b>ИТ-сервис</b><br><font style='font-size:11px'>API платежей, Авторизация<br>правило расчёта: худший / N из M<br><b>RED</b>: rate, errors, duration</font>", 40, 250, W, H, L_SVC)
p.node("ci", "<b>Конфигурационная единица (КЕ)</b><br><font style='font-size:11px'>приложение, БД, хост, под, облачная группа<br>создаётся обнаружением или вручную<br><b>USE</b>: utilization, saturation, errors</font>", 40, 400, W, H, L_CI)
p.node("mon", "<b>Объект мониторинга</b><br><font style='font-size:11px'>хост, триггер, метрика, проверка в любой системе;<br>облачный ресурс, чей ID может меняться</font>", 40, 550, W, H, L_MON)
p.node("pds", "<b>Сервис PagerDuty</b><br><font style='font-size:11px'>routing_key; расписания и escalation policy<br>живут в PagerDuty</font>", 440, 100, 280, 70, L_TEAM)
p.node("team", "<b>Команда</b><br><font style='font-size:11px'>участники, группа в каталоге (AD)</font>", 440, 257, 280, 70, L_TEAM)
p.node("bkc", "<b>Резервные контакты</b><br><font style='font-size:11px'>почта, мессенджер" + ("" if MVP else ", телефон для SMS и звонка") + ";<br>когда PagerDuty не принял алерт</font>", 440, 350, 280, 60, L_TEAM)
p.node("disc", "<b>Правило обнаружения</b><br><font style='font-size:11px'>low-code: какие объекты мониторинга<br>дают КЕ и связи</font>", 440, 432, 280, 60, L_TEAM)
p.node("alrt", "<b>Тревога</b><br><font style='font-size:11px'>ключ дедупликации, источники, влияние</font>", 440, 555, 280, 70, L_ALERT)
p.edge("biz", "svc", "зависит от 1..N")
p.edge("svc", "ci", "работает на 1..N")
p.edge("mon", "ci", "сопоставляется<br>по меткам и тегам", exit=(0.5, 0), entry=(0.5, 1))
p.edge("team", "svc", "владеет", exit=(0, 0.5), entry=(1, round((292 - 250) / 84, 4)))
p.edge("team", "pds", "дежурит через", exit=(0.5, 0), entry=(0.5, 1))
p.edge("team", "bkc", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("svc", "pds", "маршрутизируется в", exit=(1, 0.15), entry=(0, 0.8))
p.edge("disc", "ci", "создаёт КЕ и связи", exit=(0, 0.5), entry=(1, round((462 - 400) / 84, 4)))
p.edge("alrt", "mon", "порождена 1..N", exit=(0, 0.5), entry=(1, round((590 - 550) / 84, 4)))
p.edge("alrt", "ci", "влияет на", exit=(0, 0.15), entry=(1, 0.9))
p.node("sep", "", 790, 60, 2, 600, "line;strokeColor=#CCCCCC;html=1;direction=south;")
p.text("<b>Пример: как тревога на диске поднимается до бизнес-услуги</b>", 820, 60, 700, 24, NOTE + "fontSize=14;")
H = 70
p.node("x_bank", "<b>Интернет-банк</b><br>бизнес-услуга · деградация", 1030, 100, 220, H, ST_WARN)
p.node("x_pay", "<b>API платежей</b><br>ИТ-сервис · деградация", 900, 250, 210, H, ST_WARN)
p.node("x_auth", "<b>Авторизация</b><br>ИТ-сервис · норма", 1280, 250, 180, H, ST_OK)
p.node("x_app", "<b>payments-api</b><br>КЕ · облачная группа · норма", 820, 410, 160, H, ST_OK)
p.node("x_grp", "<b>Кластер БД: «N из M»</b>, 1 из 2 в аварии", 1000, 380, 260, 110, NODE_BOUND + "dashed=1;dashPattern=4 4;fontSize=11;")
p.node("x_db", "<b>db-01</b><br>PostgreSQL<br>КЕ · авария", 1010, 410, 115, H, ST_CRIT)
p.node("x_db2", "<b>db-02</b><br>PostgreSQL<br>КЕ · норма", 1135, 410, 115, H, ST_OK)
p.node("x_kc", "<b>Keycloak</b><br>КЕ · норма", 1280, 410, 180, H, ST_OK)
p.node("x_m1", "Облако<br>payments-asg", 820, 550, 160, 60, L_MON)
p.node("x_m2", "<b>Системы A и B: db-01</b><br>«Диск заполнен на 95%»", 1000, 550, 190, 60, ST_CRIT)
p.node("x_m3", "Система A<br>проверка keycloak", 1280, 550, 180, 60, L_MON)
p.edge("x_bank", "x_pay", "", exit=(0.25, 1), entry=(0.75, 0))
p.edge("x_bank", "x_auth", "", exit=(0.85, 1), entry=(0.3, 0))
p.edge("x_pay", "x_app", "", exit=(0.2, 1), entry=(0.6, 0))
p.edge("x_pay", "x_grp", "", exit=(0.75, 1), entry=(0.3, 0))
p.edge("x_auth", "x_kc", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("x_m1", "x_app", "", exit=(0.5, 0), entry=(0.5, 1))
p.edge("x_m2", "x_db", "", exit=(round(67.5 / 190, 4), 0), entry=(0.5, 1))
p.edge("x_m3", "x_kc", "", exit=(0.5, 0), entry=(0.5, 1))
p.text("Правило «худший»: авария КЕ даёт аварию ИТ-сервиса; для резервированных «N из M» — авария одной из M КЕ даёт деградацию. "
       "Здесь db-01 входит в кластер «N из M», поэтому API платежей и Интернет-банк в деградации, а не в аварии. "
       "Два источника про один диск дают одну тревогу; она уходит в сервис PagerDuty «API платежей», в её описании видно влияние на Интернет-банк.",
       820, 630, 700, 75)

# =====================================================================
# 8. RED и USE
# =====================================================================
p = Page("RED и USE", "sig"); pages.append(p)
p.text("<b>Формирование алертов по RED и USE</b>", 40, 20, 1200, 30, TITLE)
p.node("g_red", "<b>RED</b> · ИТ-сервисы: что чувствует пользователь", 40, 70, 700, 240, NODE_BOUND)
p.node("g_use", "<b>USE</b> · ресурсы и КЕ: почему это происходит", 780, 70, 700, 240, NODE_BOUND)
SW, SH = 206, 170


def sig(key, name, what, rule, src, x, style):
    lab = (f"<b>{escape(name)}</b><br><font style='font-size:11px'>{escape(what)}</font><br><br>"
           f"<font style='font-size:11px'><i>Алерт:</i> {escape(rule)}</font><br><br>"
           f"<font style='font-size:10px;color:#444444'>{escape(src)}</font>")
    p.node(key, lab, x, 120, SW, SH, style + "verticalAlign=top;spacingTop=10;")


sig("r", "Rate", "Интенсивность запросов, запросов в секунду",
    "падение ниже базовой линии на 50% за 10 мин", "счётчик запросов сервиса", 60, L_SVC)
sig("e", "Errors", "Доля ошибочных ответов",
    "расход бюджета ошибок SLO в 14,4 раза быстрее нормы за 1 ч", "ответы 5xx / все ответы; ошибки в логах доступа", 287, L_SVC)
sig("d", "Duration", "Время ответа p95 / p99",
    "p99 выше цели SLO дольше 5 мин", "гистограмма времени ответа", 514, L_SVC)
sig("u", "Utilization", "Занятость ресурса: CPU, память, диск, сеть",
    "выше 90% дольше 15 мин или прогноз заполнения диска меньше 24 ч", "метрики хоста, БД, облачного ресурса", 800, L_CI)
sig("s", "Saturation", "Очередь к ресурсу: load average, swap, пул соединений, лаг репликации",
    "load больше 2 × ядер или очередь растёт 10 мин", "load, активные соединения, лаг очереди", 1027, L_CI)
sig("ue", "Errors", "Ошибки ресурса: I/O, сетевые, OOM, рестарты",
    "любой рост счётчика за 5 мин", "счётчики ошибок I/O, рестарты, ERROR в логах КЕ", 1254, L_CI)
FY = 390
if MVP:
    XT, XE, XV, XA, XP = (60, 240), (350, 250), (650, 230), (930, 280), (1260, 200)
else:
    XT, XE, XV, XA, XC, XP = (60, 220), (320, 230), (600, 200), (850, 220), (1120, 200), (1370, 110)
p.node("tpl", "<b>Шаблон RED или USE</b><br><font style='font-size:11px'>low-code: сигнал, запрос, порог, окно</font>", XT[0], FY, XT[1], 80, L_TEAM)
p.node("eng", "<b>Rule Engine</b><br><font style='font-size:11px'>метрики у коннекторов (connector.query)<br>каждые 30 с: порог, базовая линия, SLO</font>", XE[0], FY, XE[1], 80, L_SVC)
p.node("ev", "<b>Событие</b><br><font style='font-size:11px'>method: red | use | other,<br>signal, сервис или КЕ, severity</font>", XV[0], FY, XV[1], 80, L_ALERT)
p.node("ae", "<b>Alert Engine</b><br><font style='font-size:11px'>дедупликация между источниками,<br>привязка к КЕ, влияние по РСМ"
       + ("; связь RED и USE" if MVP else "") + "</font>", XA[0], FY, XA[1], 80, L_SVC)
if TGT:
    p.node("corr", "<b>Correlation Engine</b><br><font style='font-size:11px'>RED сервиса + USE его КЕ<br>= инцидент, USE — вероятная причина</font>", XC[0], FY, XC[1], 80, L_SVC)
p.node("pd", c4("PagerDuty", "", "trigger / resolve"), XP[0], FY, XP[1], 80, EXT)
p.node("ext", "<b>Готовые алерты</b><br><font style='font-size:11px'>из систем мониторинга и облаков</font>", XT[0], FY + 130, XT[1], 70, L_MON)
p.node("cls", "<b>Шаблон события</b><br><font style='font-size:11px'>ставит method и signal по меткам,<br>сигнал входит в ключ дедупликации</font>", XE[0], FY + 130, XE[1], 70, L_TEAM)
p.edge("g_red", "tpl", "шаблон на сервис", exit=(round((XT[0] + 0.3 * XT[1] - 40) / 700, 4), 1), entry=(0.3, 0))
p.edge("g_use", "tpl", "шаблон на тип КЕ", EDGE_ORTHO, points=[(1130, 350), (XT[0] + 0.8 * XT[1], 350)], exit=(0.5, 1), entry=(0.8, 0))
p.edge("tpl", "eng")
p.edge("eng", "ev")
p.edge("ev", "ae")
if MVP:
    p.edge("ae", "pd")
else:
    p.edge("ae", "corr")
    p.edge("corr", "pd")
p.edge("ext", "cls")
p.edge("cls", "ev", "", EDGE_ORTHO, points=[(XV[0] + XV[1] / 2, FY + 165)], exit=(1, 0.5), entry=(0.5, 1))
p.text("RED-шаблон привязывается к ИТ-сервису, USE-шаблон к типу КЕ (хост, БД, под, облачная группа). "
       + ("Когда одновременно горят RED сервиса и USE одной из его КЕ, Alert Engine (RED/USE Link) связывает тревоги в окне 10 мин, "
          "USE-тревога помечается как вероятная причина; дежурный видит обе тревоги рядом в дашборде инцидентов. "
          if MVP else
          "Correlation Engine объединяет RED сервиса и USE его КЕ в инцидент и помечает USE-тревогу как вероятную причину; в PagerDuty уходит один алерт на инцидент. ")
       + "Пороги на схеме даны для иллюстрации, их задаёт инженер в шаблоне.", 40, FY + 230, 1440, 50)

# =====================================================================
# 9. Парсинг, шаблоны и дедупликация
# =====================================================================
p = Page("Парсинг, шаблоны, дедупликация", "dup"); pages.append(p)
p.text("<b>Парсинг, шаблоны событий и схлопывание дублей между источниками</b>", 40, 20, 1300, 30, TITLE)


def raw(key, src, body, y):
    p.node(key, f"<b>{escape(src)}</b><br><font style='{MONO}'>{body}</font>", 40, y, 360, 100, L_MON + LEFT)


raw("r1", "Система A · API, JSON", "{\"host\": \"db-01.bank.local\",<br> \"name\": \"High CPU\", \"value\": 93}", 90)
raw("r2", "Система B · таблица в БД, текст", "message: 'CPU on db-01 is 94% (crit)'<br>regex: CPU on (?P&lt;host&gt;\\S+) is (?P&lt;v&gt;\\d+)%", 220)
raw("r3", "Облако · событие", "resource: i-7f3c9e01 · CPU 92.7 %<br>теги по ID: host=db-01", 350)
NX = 520
labs = ["парсер JSON,<br>шаблон", "парсер regex,<br>шаблон", "ID → КЕ<br>по тегам"]
for i, k in enumerate(("n1", "n2", "n3")):
    p.node(k, "<b>Нормализованное событие</b><br>"
              f"<font style='{MONO}'>ci: db-01 · signal: use.cpu.utilization<br>severity: critical · value: {['93', '94', '92.7'][i]} %</font>",
           NX, 90 + i * 130 + 15, 320, 70, L_SVC + LEFT)
    p.edge(f"r{i + 1}", k, labs[i])
p.node("key", "<b>Ключ дедупликации</b><br><font style='font-size:11px'>КЕ + сигнал (+ метки по выбору)</font><br><br>"
              f"<font style='{MONO}'>db-01 · use.cpu.utilization</font>", 930, 215, 250, 110, L_TEAM)
for k in ("n1", "n2", "n3"):
    p.edge(k, "key", "")
p.node("alrt", "<b>Одна тревога Umbrella #1842</b><br><font style='font-size:11px'>db-01: CPU 94 % · severity critical<br>источники: Система A, Система B, облако<br>событий: 3 · влияние: API платежей" + ("" if MVP else "<br>входит в инцидент #731") + "</font>",
       1290, 215, 280, 110, L_ALERT)
p.edge("key", "alrt", "схлопнуть")
p.node("pd", "<b>PagerDuty · одно событие</b><br>"
             f"<font style='{MONO}'>event_action: trigger<br>dedup_key: " + ("umb-1842" if MVP else "umb-inc-731") + "<br>routing_key: сервис «API платежей»</font>",
       1290, 400, 280, 90, L_MON + LEFT + "strokeWidth=2;")
p.edge("alrt", "pd", "Events API v2", exit=(0.5, 1), entry=(0.5, 0))
p.node("tpl", "<b>Шаблон события (low-code)</b><br><br><font style='" + MONO + "'>"
              "summary: {{ ci.name }}: {{ signal.title }} {{ value }} %<br>"
              "severity: max(sources.severity)<br>"
              "details: сервисы {{ impact.services }}, источники {{ sources }}<br>"
              "links: дашборд инцидента в Grafana, карта CMDB, источники</font>",
       40, 560, 700, 120, L_TEAM + LEFT + "verticalAlign=top;spacingTop=8;")
p.node("rules", "<b>Правила схлопывания</b><br><font style='font-size:11px'>"
                "• окно склейки 10 мин, дальше та же тревога, пока открыта<br>"
                "• severity тревоги = максимум по источникам<br>"
                "• resolve в PagerDuty, когда в норму вернулись все источники<br>"
                "• повторы не создают новых событий в PagerDuty: тот же dedup_key</font>",
       800, 560, 770, 120, L_ALERT + LEFT + "verticalAlign=top;spacingTop=8;")
p.text("Парсер разбирает сырое тело (JSON, XML, CSV, regex, Grok, key=value, syslog) в поля; шаблон источника переводит поля в единую модель "
       "(КЕ через карту CMDB, сигнал, severity). Шаблон события задаёт текст, который увидит дежурный в PagerDuty. Значения на схеме — пример.",
       40, 700, 1400, 40)

# =====================================================================
# BPMN helpers
# =====================================================================
def lanes(p, names, x0, y0, width, heights):
    ys = []
    y = y0
    for i, (n, h) in enumerate(zip(names, heights)):
        p.node(f"lane{i}", n, x0, y, width, h, LANE)
        ys.append((y, h))
        y += h
    return ys


def mid(ys, i, h):
    y, lh = ys[i]
    return y + (lh - h) / 2


TW, TH = 150, 64
GW = 70
EW = 36

# =====================================================================
# БП-1 Обработка тревоги
# =====================================================================
p = Page("БП-1 Обработка тревоги", "bp1"); pages.append(p)
p.text("<b>БП-1 · Обработка тревоги: от события до закрытия</b>", 40, 20, 1200, 30, TITLE)
DX = 0 if MVP else 220
ys = lanes(p, ["Источник", "Umbrella", "PagerDuty", "Дежурный инженер"],
           40, 70, 2130 + DX, [170, 280, 150, 150])
L0, L1, L2, L3 = 0, 1, 2, 3
uy = ys[L1][0] + 45
ry = uy + 140
p.node("s", "", 110, mid(ys, L0, EW), EW, EW, START)
p.text("Сработал триггер или правило", 70, mid(ys, L0, EW) + 40, 120, 30, EVLABEL)
p.node("t1", "Отдать событие<br>по API, из БД<br>или webhook", 200, mid(ys, L0, TH), TW, TH, TASK)
p.node("t2", "Забрать и распарсить<br>событие, записать<br>в шину", 200, uy, TW, TH + 6, TASK)
p.node("t0", "Отметить событие<br>как переданное", 390, mid(ys, L0, TH), TW, TH, TASK)
p.node("t3", "Привязать к КЕ<br>(облачный ID → КЕ),<br>посчитать ключ", 390, uy, TW, TH + 6, TASK)
p.node("g1", "Окно<br>обслуживания?", 575, uy - 10, GW + 20, GW + 20, GATE)
p.node("tsup", "Записать без<br>отправки", 545, ry, TW, TH, TASK)
p.node("e1", "", 720, ry + 14, EW, EW, END)
p.node("g2", "Есть тревога<br>с этим<br>ключом?", 770, uy - 5, GW + 10, GW + 10, GATE)
p.node("tmrg", "Схлопнуть: источник,<br>счётчик, severity = max", 770, ry, TW + 20, TH, TASK)
p.node("e2", "", 960, ry + 14, EW, EW, END)
p.node("t5", "Создать тревогу,<br>влияние по РСМ,<br>текст по шаблону", 900, uy, TW + 10, TH + 6, TASK)
if TGT:
    p.node("tcor", "Correlation Engine:<br>привязать<br>к инциденту", 1110, uy, TW + 10, TH + 6, TASK)
    p.node("titsm", "ITSM Sync: создать<br>инцидент в ITSM", 1110, ry, TW + 10, TH, TASK_HOT)
    p.node("e4", "", 1300, ry + 14, EW, EW, END)
p.node("t6", "Отправить trigger<br>в PagerDuty<br>(" + ("dedup_key" if MVP else "dedup_key umb-inc-‹id›") + ")", 1110 + DX, uy, TW + 10, TH + 6, TASK_HOT)
p.node("p1", "Открыть инцидент,<br>уведомить по<br>escalation policy", 1110 + DX, mid(ys, L2, TH + 6), TW + 10, TH + 6, TASK_HOT)
p.node("d1", "Получить звонок,<br>push или SMS;<br>подтвердить (ack)", 1110 + DX, mid(ys, L3, TH + 6), TW + 10, TH + 6, TASK)
p.node("p2", "Webhook<br>acknowledged", 1330 + DX, mid(ys, L2, TH), TW, TH, TASK)
p.node("t7", "Отметить ack,<br>передать ack<br>в источники", 1330 + DX, uy, TW, TH + 6, TASK)
p.node("d2", "Устранить<br>причину", 1520 + DX, mid(ys, L3, TH + 6), TW, TH + 6, TASK)
p.node("t10", "Триггер вернулся<br>в норму (resolved)", 1760 + DX, mid(ys, L0, TH), TW, TH, TASK)
p.node("g3", "Все<br>источники<br>в норме?", 1795 + DX, uy - 8, GW + 10, GW + 10, GATE)
p.node("twait", "Ждать resolve<br>остальных источников", 1760 + DX, ry, TW, TH, TASK)
p.node("t12", "Отправить resolve<br>в PagerDuty", 1960 + DX, uy, TW, TH, TASK_HOT)
p.node("p3", "Закрыть<br>инцидент", 1960 + DX, mid(ys, L2, TH), TW, TH, TASK)
p.node("e3", "", 2120 + DX, mid(ys, L2, EW), EW, EW, END)
p.edge("s", "t1")
p.edge("t1", "t2", "")
p.edge("t2", "t0", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(320, 210), (465, 210)], exit=(0.8, 0), entry=(0.5, 1))
p.text("ack после записи", 472, 196, 110, 20, EVLABEL + "align=left;")
p.edge("t2", "t3")
p.edge("t3", "g1")
p.edge("g1", "tsup", "да", exit=(0.5, 1), entry=(0.5, 0))
p.edge("tsup", "e1")
p.edge("g1", "g2", "нет")
p.edge("g2", "tmrg", "да", exit=(0.5, 1), entry=(0.2353, 0))
p.edge("tmrg", "e2")
p.edge("g2", "t5", "нет")
if MVP:
    p.edge("t5", "t6")
else:
    p.edge("t5", "tcor")
    p.edge("tcor", "t6")
    p.edge("tcor", "titsm", "критичная<br>бизнес-услуга", exit=(0.5, 1), entry=(0.5, 0))
    p.edge("titsm", "e4")
p.edge("t6", "p1", "Events API v2", exit=(0.5, 1), entry=(0.5, 0))
p.edge("p1", "d1", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("d1", "p2", "ack", EDGE_ORTHO, exit=(1, 0.3), entry=(0.5, 1))
p.edge("p2", "t7", "webhook", exit=(0.5, 0), entry=(0.5, 1))
p.edge("d1", "d2", "", exit=(1, 0.75), entry=(0, 0.75))
p.edge("d2", "t10", "исправлено", EDGE_ORTHO, points=[(1720 + DX, mid(ys, L3, TH + 6) + 35), (1720 + DX, mid(ys, L0, TH) + 32)], exit=(1, 0.5), entry=(0, 0.5))
p.edge("t10", "g3", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("g3", "t12", "да")
p.edge("g3", "twait", "нет", exit=(0.5, 1), entry=(0.5, 0))
p.edge("twait", "t12", "все в норме", EDGE_ORTHO, exit=(1, 0.5), entry=(0.2, 1))
p.edge("t12", "p3", "", exit=(0.7, 1), entry=(0.7, 0))
p.edge("p3", "e3")
p.text("Источник получает подтверждение только после записи события в шину, поэтому повторная отдача не создаёт дублей. "
       "Расписания, эскалации и каналы (звонок с обходом беззвучного режима, push, SMS, email) настраиваются в PagerDuty. "
       "Umbrella отвечает за то, чтобы в PagerDuty пришёл один алерт на " + ("проблему" if MVP else "инцидент (из одной или нескольких тревог)")
       + ", с понятным текстом, влиянием на сервисы и ссылкой на дашборд инцидента в Grafana, и чтобы ack и resolve вернулись в источники. "
       "Если PagerDuty не принял trigger тревоги error или critical за 2 минуты, запускается БП-5 «Резервное оповещение».", 40, 840, 1800 + DX, 40)

# =====================================================================
# БП-2 Сборка коннектора
# =====================================================================
p = Page("БП-2 Сборка коннектора", "bp2"); pages.append(p)
p.text("<b>БП-2 · Сборка коннектора в low-code конструкторе</b>", 40, 20, 1200, 30, TITLE)
ys = lanes(p, ["Инженер мониторинга", "Umbrella", "Система-источник"], 40, 70, 1960, [170, 250, 130])
p.node("s", "", 100, mid(ys, 0, EW), EW, EW, START)
p.node("a1", "Собрать коннектор<br>из блоков: API, БД,<br>webhook, очередь,<br>облако", 170, mid(ys, 0, TH + 16), TW + 10, TH + 16, TASK)
p.node("a2", "Авторизация (секрет<br>в OpenBao),<br>курсор, интервал", 370, mid(ys, 0, TH + 16), TW + 20, TH + 16, TASK)
p.node("b1", "Проверить<br>соединение", 380, mid(ys, 1, TH), TW, TH, TASK)
p.node("g1", "Успешно?", 580, mid(ys, 1, GW), GW, GW, GATE)
p.node("c1", "Отдать пример<br>событий", 700, mid(ys, 2, TH), TW, TH, TASK)
p.node("b2", "Показать пример<br>сырого payload", 700, mid(ys, 1, TH), TW, TH, TASK)
p.node("a3", "Настроить парсер,<br>маппинг, шаблон<br>и ключ дедупликации", 900, mid(ys, 0, TH + 6), TW + 10, TH + 6, TASK)
p.node("b3", "Dry-run: парсинг,<br>шаблон, ack,<br>склейка с другими", 900, mid(ys, 1, TH + 6), TW + 10, TH + 6, TASK)
p.node("g2", "Результат<br>верный?", 1110, mid(ys, 1, GW), GW, GW, GATE)
p.node("a4", "Опубликовать<br>версию коннектора", 1230, mid(ys, 0, TH), TW, TH, TASK)
p.node("b4", "Включить приём,<br>запустить обнаружение<br>КЕ (БП-4)", 1430, mid(ys, 1, TH + 6), TW + 10, TH + 6, TASK)
p.node("g3", "Ошибок<br>разбора<br>больше<br>порога?", 1635, mid(ys, 1, 100), 100, 100, GATE)
p.node("b5", "Откатить версию,<br>показать ошибку<br>инженеру", 1790, ys[1][0] + 15, TW, TH + 6, TASK_HOT)
p.node("e", "", 1810, ys[1][0] + ys[1][1] - 60, EW, EW, END)
p.text("Источник подключён", 1855, ys[1][0] + ys[1][1] - 58, 100, 30, EVLABEL + "align=left;")
p.edge("s", "a1"); p.edge("a1", "a2"); p.edge("a2", "b1")
p.edge("b1", "g1")
p.edge("g1", "a2", "нет", EDGE_ORTHO, exit=(0.5, 0), entry=(1, 0.5))
p.edge("g1", "b2", "да")
p.edge("c1", "b2", "", EDGE_DASH)
p.edge("b2", "a3", "", EDGE_ORTHO, points=[(870, mid(ys, 1, TH) + 0.3 * TH), (870, mid(ys, 0, TH + 6) + 35)], exit=(1, 0.3), entry=(0, 0.5))
p.edge("a3", "b3", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("b3", "g2")
p.edge("g2", "a3", "нет", EDGE_ORTHO, exit=(0.5, 0), entry=(1, 0.5))
p.edge("g2", "a4", "да", EDGE_ORTHO, exit=(1, 0.5), entry=(0.5, 1))
p.edge("a4", "b4", "", EDGE_ORTHO, exit=(1, 0.5), entry=(0.5, 0))
p.edge("b4", "g3")
p.edge("g3", "b5", "да", EDGE_ORTHO, exit=(0.5, 0), entry=(0, 0.5))
p.edge("g3", "e", "нет", EDGE_ORTHO, exit=(0.5, 1), entry=(0, 0.5))
p.edge("b5", "a3", "исправить", EDGE_ORTHO, points=[(1865, 95), (980, 95)], exit=(0.5, 0), entry=(0.5, 0))

# =====================================================================
# БП-3 Плановые работы
# =====================================================================
p = Page("БП-3 Плановые работы", "bp3"); pages.append(p)
p.text("<b>БП-3 · Плановые работы (окно обслуживания)</b>", 40, 20, 1200, 30, TITLE)
ys = lanes(p, ["Владелец сервиса", "Второй инженер", "Umbrella", "PagerDuty и дежурный"], 40, 70, 1720, [150, 130, 190, 140])
p.node("s", "", 100, mid(ys, 0, EW), EW, EW, START)
p.node("a1", "Создать окно:<br>сервисы или КЕ,<br>начало и конец", 180, mid(ys, 0, TH + 6), TW, TH + 6, TASK)
p.node("ap", "Подтвердить окно<br>(второй ключ)", 380, mid(ys, 1, TH), TW, TH, TASK_HOT)
p.node("g0", "Подтверж-<br>дено?", 580, mid(ys, 1, GW), GW, GW, GATE)
p.node("b1", "Найти все КЕ<br>под сервисами по РСМ", 700, mid(ys, 2, TH), TW, TH, TASK)
p.node("b2", "Во время окна:<br>тревоги пишутся,<br>в PagerDuty не уходят", 900, mid(ys, 2, TH + 6), TW + 10, TH + 6, TASK)
p.node("tm", "", 1120, mid(ys, 2, 40), 40, 40, TIMER)
p.text("Окно закончилось", 1090, mid(ys, 2, 40) + 44, 100, 30, EVLABEL)
p.node("g1", "Тревоги ещё<br>активны?", 1210, mid(ys, 2, GW + 20), GW + 20, GW + 20, GATE)
p.node("b3", "Отправить активные<br>тревоги в PagerDuty", 1370, mid(ys, 2, TH), TW, TH, TASK_HOT)
p.node("c1", "Уведомить дежурного<br>по escalation policy", 1570, mid(ys, 3, TH), TW, TH, TASK)
p.node("a2", "Получить отчёт:<br>что сработало в окне", 1370, mid(ys, 0, TH), TW, TH, TASK)
p.node("e", "", 1620, mid(ys, 0, EW), EW, EW, END)
p.edge("s", "a1")
p.edge("a1", "ap", "", EDGE_ORTHO, exit=(1, 0.5), entry=(0.5, 0))
p.edge("ap", "g0")
p.edge("g0", "a1", "нет", EDGE_ORTHO, points=[(615, 85), (255, 85)], exit=(0.5, 0), entry=(0.5, 0))
p.edge("g0", "b1", "да", EDGE_ORTHO, exit=(1, 0.5), entry=(0.5, 0))
p.edge("b1", "b2"); p.edge("b2", "tm"); p.edge("tm", "g1")
p.edge("g1", "b3", "да")
p.edge("g1", "a2", "", EDGE_ORTHO, exit=(0.5, 0), entry=(0, 0.5))
p.text("нет", 1263, mid(ys, 2, GW + 20) - 26, 30, 18, EVLABEL + "align=left;")
p.edge("b3", "c1", "", EDGE_ORTHO, exit=(1, 0.5), entry=(0.5, 0))
p.edge("b3", "a2", "", exit=(0.5, 0), entry=(0.5, 1))
p.edge("a2", "e")
p.text("Окно обслуживания действует только после подтверждения вторым инженером (изменения в два ключа); автор окна не может подтвердить его сам.",
       40, 700, 1720, 30)

# =====================================================================
# БП-4 Построение карты CMDB
# =====================================================================
p = Page("БП-4 Карта CMDB", "bp4"); pages.append(p)
p.text("<b>БП-4 · Построение карты CMDB из данных мониторинга и облаков</b>", 40, 20, 1300, 30, TITLE)
ys = lanes(p, ["Источники", "Umbrella · CMDB Discovery" if MVP else "Umbrella", "Инженер мониторинга"], 40, 70, 1760 if MVP else 1980, [140, 200, 150])
p.node("tm", "", 100, mid(ys, 1, 40), 40, 40, TIMER)
p.text("Каждые 15 мин или<br>новый источник", 70, mid(ys, 1, 40) + 44, 100, 30, EVLABEL)
p.node("b1", "Запросить инвентарь<br>коннекторами", 190, mid(ys, 1, TH), TW + 10, TH, TASK)
p.node("c1", "Отдать объекты: хосты,<br>ресурсы облака,<br>теги, метки", 180, mid(ys, 0, TH + 10), TW + 30, TH + 10, TASK)
p.node("b2", "Применить правила<br>обнаружения (low-code):<br>тип и атрибуты КЕ", 400, mid(ys, 1, TH + 6), TW + 10, TH + 6, TASK)
p.node("b3", "Склеить объекты;<br>облачные ID → устойчивая<br>КЕ по тегам и группе", 610, mid(ys, 1, TH + 6), TW + 10, TH + 6, TASK)
p.node("b4", "Построить связи:<br>сервис → КЕ по меткам,<br>инстанс → группа, БД → хост", 820, mid(ys, 1, TH + 6), TW + 20, TH + 6, TASK)
p.node("b5", "Сравнить с картой;<br>пропавшие дольше срока<br>пометить устаревшими", 1040, mid(ys, 1, TH + 6), TW + 20, TH + 6, TASK)
p.node("g1", "Есть<br>изменения?", 1245, mid(ys, 1, GW + 14), GW + 14, GW + 14, GATE)
p.node("e1", "", 1269, mid(ys, 0, EW), EW, EW, END)
p.text("Карта без изменений", 1310, mid(ys, 0, EW) + 8, 120, 30, EVLABEL)
p.node("g2", "Можно<br>применить<br>автоматически?", 1370, mid(ys, 1, GW + 20), GW + 20, GW + 20, GATE)
p.node("a1", "Проверить черновик:<br>принять, поправить<br>или отклонить", 1340, mid(ys, 2, TH + 6), TW, TH + 6, TASK_HOT)
p.node("b6", "Применить, записать<br>версию карты,<br>cmdb.changed", 1520, mid(ys, 1, TH + 6), TW, TH + 6, TASK)
if TGT:
    p.node("b7", "ITSM Sync: выгрузить<br>КЕ в корпоративную<br>CMDB, забрать<br>бизнес-услуги", 1720, mid(ys, 1, TH + 16), TW + 10, TH + 16, TASK_HOT)
EX2 = 1720 if MVP else 1930
p.node("e2", "", EX2, mid(ys, 1, EW), EW, EW, END)
p.text("Карта обновлена", EX2 - 30, mid(ys, 1, EW) + 40, 100, 30, EVLABEL)
p.edge("tm", "b1")
p.edge("b1", "c1", "API, SQL", exit=(0.3, 0), entry=(0.3222, 1))
p.edge("c1", "b2", "данные", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(480, mid(ys, 0, TH + 10) + 37)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("b2", "b3"); p.edge("b3", "b4"); p.edge("b4", "b5"); p.edge("b5", "g1")
p.edge("g1", "e1", "нет", exit=(0.5, 0), entry=(0.5, 1), lpos=-0.5)
p.edge("g1", "g2", "да")
p.edge("g2", "b6", "да")
p.edge("g2", "a1", "нет", exit=(0.5, 1), entry=(0.5, 0), lpos=0.45)
p.edge("a1", "b6", "принято", EDGE_ORTHO, exit=(1, 0.5), entry=(0.5, 1))
if MVP:
    p.edge("b6", "e2")
else:
    p.edge("b6", "b7")
    p.edge("b7", "e2")
p.text("Автоматически применяются только добавления по доверенным правилам; удаление связей, смена типа КЕ и конфликты идут на ревью. "
       "Ручные правки инженера не перезаписываются обнаружением. Новый ID облачного инстанса не создаёт новую КЕ, а пишется в её историю.",
       40, 580, 1760 if MVP else 1980, 40)
