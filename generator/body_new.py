import re

# =====================================================================
# Новые страницы
# =====================================================================
SMALL = NOTE + "fontSize=10;"


def link(a, b):
    """Линия без стрелки (ножка интерфейса)."""
    return "endArrow=none;html=1;strokeColor=#3C7FC0;strokeWidth=2;"


# ---------------------------------------------------------------------
# Контекст потоков
# ---------------------------------------------------------------------
LBL = "text;html=1;fillColor=#FFFFFF;strokeColor=none;align=center;verticalAlign=middle;fontSize=10;fontColor=#404040;whiteSpace=wrap;"


def lbl(t, cx, cy, w=160, h=28):
    """Подпись связи отдельным блоком на белом фоне (точное положение)."""
    return p.text(t, cx - w / 2, cy - h / 2, w, h, LBL)


p = Page("Контекст потоков", "flc"); pages.append(p)
p.text("<b>Контекст потоков данных: 10 потоков П-01…П-10 (внешние + внутренний П-03)</b>" if MVP else
       "<b>Контекст потоков данных: 12 потоков П-01…П-12 (внешние + внутренний П-03 + межплощадочный П-12)</b>", 40, 20, 1560, 30, TITLE)
p.node("eng", c4("Инженер мониторинга", "Person", "Конструктор, карта CMDB, дашборд инцидентов"), 670, 60, 300, 90, PERSON)
p.node("src_bnd", "<b>Источники</b>", 40, 220, 300, 430, NODE_BOUND)
for i, (k, n, d) in enumerate([("mon", "Системы мониторинга", "API, webhook"), ("db", "БД систем мониторинга", "SQL"),
                               ("cloud", "Облачные провайдеры", "API, события, теги"), ("stream", "Потоки событий", "очереди, syslog, почта")]):
    p.node(k, c4(n, "External System", d), 60, 260 + i * 92, 260, 76, EXT)
p.node("umb", c4("Umbrella", "Software System",
                 ("П-03 внутри: Ingest Gateway и Connector Runtime → Normalizer → Alert Engine через NATS JetStream (ingest.in, events.raw, events.norm), "
                  if MVP else
                  "П-03 внутри: Ingest Gateway и Connector Runtime → Normalizer → Alert Engine → Correlation Engine через NATS JetStream "
                  "(ingest.in, events.raw, events.norm, alerts.changed, incidents.changed), ")
                 + "at-least-once, идемпотентные потребители"),
       600, 260, 440, 380, SYSTEM)
p.node("duty", c4("Дежурный инженер", "Person", "Звонок, push, SMS; дашборд в Grafana"), 1240, 60, 260, 90, PERSON)
p.node("pd", c4("PagerDuty", "External System", "Инциденты, эскалации, доставка"), 1240, 250, 260, 130, EXT)
p.node("fbx", c4("Резервные каналы", "External System", FB_CHANNELS), 1240, 420, 260, 110, EXT)
p.node("grf", c4("Grafana", "External System", "корпоративная; если её нет — Grafana OSS в контуре"), 1240, 570, 260, 90, EXT)
p.node("tsdb", c4("Хранилища метрик, логов и трейсов", "External System, вне Umbrella", "источники данных Grafana"), 1240, 750, 260, 90, EXT_DASHED)
p.node("idp", c4("Корпоративный IdP", "External System", "OIDC, SCIM 2.0"), 260, 770, 240, 90, EXT)
p.node("dir", c4("Каталог (AD)", "External System", "группы UMB-*"), 720, 770, 160, 90, EXT)


def hy(key, y):
    x, y0, w, h = p.geo[key]
    return round((y - y0) / h, 4)


def hx(key, x):
    x0, y, w, h = p.geo[key]
    return round((x - x0) / w, 4)


for y, lab, a, b, st in [
    (310, "П-01 · pull: HTTPS API, SQL, брокер, API облака;<br>ack или курсор после записи в шину", "src_bnd", "umb", EDGE),
    (380, "П-02 · push: HTTPS webhook;<br>ответ 2xx после записи в шину", "src_bnd", "umb", EDGE),
    (450, "П-05 · ack и silence обратно<br>в источники блоками коннектора", "umb", "src_bnd", EDGE_DASH),
    (515, "П-06 · инвентарь: объекты,<br>теги, группы ресурсов", "src_bnd", "umb", EDGE),
]:
    p.edge(a, b, lab, st, exit=(1 if a == "src_bnd" else 0, hy(a, y)), entry=(0 if b == "umb" else 1, hy(b, y)))
p.edge("umb", "pd", "П-04 · Events API v2:<br>trigger, resolve, dedup_key", exit=(1, hy("umb", 275)), entry=(0, hy("pd", 275)))
p.edge("pd", "umb", "П-05 · Webhooks v3:<br>ack, resolve, reassign", EDGE_DASH, exit=(0, hy("pd", 320)), entry=(1, hy("umb", 320)))
p.edge("umb", "pd", "П-09 · REST API: дежурства,<br>сверка" + ("" if MVP else " (только чтение)"), exit=(1, hy("umb", 365)), entry=(0, hy("pd", 365)))
p.edge("umb", "fbx", "П-08 · резервное оповещение<br>" + ("SMTP, HTTPS webhook" if MVP else "HTTPS, SMTP; ответы с ack"),
       exit=(1, hy("umb", 475)), entry=(0, hy("fbx", 475)))
p.edge("umb", "grf", "П-10 · Grafana HTTP API: дашборды,<br>аннотации, команды и права папок", exit=(1, hy("umb", 615)), entry=(0, hy("grf", 615)))
p.edge("grf", "tsdb", "", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
lbl("запросы панелей<br>(вне Umbrella)", 1370, 705, 130, 28)
# пользователи открывают дашборд Grafana по ссылке
p.edge("duty", "grf", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1580, 105), (1580, 615)], exit=(1, 0.5), entry=(1, 0.5))
p.text("открывает дашборд<br>инцидента по ссылке<br>/go/incidents/{id}/grafana,<br>HTTPS 443", 1590, 330, 170, 64, SMALL)
# резервные каналы доставляют уведомления дежурному
p.edge("fbx", "duty", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1545, 475), (1545, 205), (1460, 205)],
       exit=(1, 0.5), entry=(hx("duty", 1460), 1))
p.edge("eng", "umb", "П-07 · HTTPS, WebSocket:<br>конфигурация, дашборд инцидентов", exit=(0.5, 1), entry=(hx("umb", 820), 0))
p.edge("pd", "duty", "уведомления<br>(вне Umbrella)", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1))
# П-07: вход и синхронизация с каталогом
p.edge("umb", "idp", "", EDGE_ORTHO, points=[(640, 795)], exit=(hx("umb", 640), 1), entry=(1, hy("idp", 795)))
lbl("П-07 · OIDC 443: вход,<br>группы в claims", 570, 795, 130, 28)
p.edge("idp", "umb", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(690, 840)], exit=(1, hy("idp", 840)), entry=(hx("umb", 690), 1))
lbl("П-07 · SCIM 2.0, 443:<br>push групп", 595, 840, 130, 28)
p.edge("umb", "dir", "", exit=(hx("umb", 800), 1), entry=(hx("dir", 800), 0))
lbl("П-07 · LDAPS 636:<br>группы раз в 15 мин", 800, 678, 140, 28)
if TGT:
    p.node("itsm", c4("Корпоративная CMDB и ITSM", "External System", "КЕ, сервисы, инциденты"), 900, 770, 160, 90, EXT)
    p.edge("umb", "itsm", "", exit=(hx("umb", 960), 1), entry=(hx("itsm", 960), 0))
    lbl("П-11 · REST: КЕ,<br>связи, инциденты", 960, 730, 120, 28)
    p.node("dr", c4("Резервная площадка", "Umbrella, тёплый резерв", "реплика PostgreSQL, зеркало NATS, снапшоты OpenBao"), 1080, 870, 260, 90, REPL)
    p.edge("umb", "dr", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1020, 690), (1200, 690), (1200, 870)],
           exit=(hx("umb", 1020), 1), entry=(hx("dr", 1200), 0))
    lbl("П-12 · репликация, TLS:<br>5432, 7422, 443", 1110, 690, 150, 28)
p.text("Все внешние потоки идут по TLS. Входящие потоки (П-02, П-05" + ("" if MVP else ", ответы П-08") + ", SCIM в П-07) принимаются только с проверкой подписи или токена; "
       "исходящие к источникам и PagerDuty (П-01, П-04, П-05) выполняются с повторами и не создают дублей: подтверждение источнику "
       "отправляется после записи в шину, а в PagerDuty уходит один dedup_key на " + ("тревогу" if MVP else "инцидент") +
       ". Резервные каналы (П-08) включаются, только если тревогу error или critical PagerDuty не принял за 2 минуты. "
       "Umbrella не хранит метрики, логи и трейсы: Grafana читает их из существующих хранилищ (пунктир — вне Umbrella). "
       "Подробности каждого потока — в таблице потоков.",
       40, 900 if MVP else 990, 1720, 50)
p.legend(labels={EXT_DASHED: "Внешняя система вне Umbrella (данные не хранятся в Umbrella)", NODE_BOUND: "Группа источников"}
         | ({} if MVP else {REPL: "Резервная площадка (тёплый резерв)"}),
         edges=[(EDGE, "Поток данных П-NN (направление по стрелке)"),
                (EDGE_DASH, "Обратный или входящий поток; уведомления и запросы Grafana вне Umbrella")])

# ---------------------------------------------------------------------
# Функции и потоки
# ---------------------------------------------------------------------
p = Page("Функции и потоки", "fnf"); pages.append(p)
p.text("<b>Модель функций и потоков: функции Ф-01…Ф-15, контейнеры и потоки</b>" if MVP else "<b>Модель функций и потоков: функции Ф-01…Ф-18, контейнеры и потоки</b>", 40, 20, 1400, 30, TITLE)
HDR = BASE + "rounded=0;fillColor=#438DD5;strokeColor=#FFFFFF;fontColor=#FFFFFF;fontSize=10;fontStyle=1;"
FN = BASE + "rounded=0;fillColor=#FFF2CC;strokeColor=#D6B656;fontSize=11;align=left;spacingLeft=6;"
CELL = BASE + "rounded=0;fillColor=#FFFFFF;strokeColor=#DDDDDD;fontSize=10;"
CELL_ON = BASE + "rounded=0;fillColor=#85BBF0;strokeColor=#DDDDDD;fontSize=14;fontColor=#08427B;"
CELL_TXT = BASE + "rounded=0;fillColor=#F5F5F5;strokeColor=#DDDDDD;fontSize=10;"
CONTS = ["Веб-интерфейс", "Core API", "Ingest Gateway", "Connector Runtime", "Rule Engine", "Normalizer",
         "Alert Engine", "CMDB Discovery", "PagerDuty Gateway", "Watchdog", "Fallback Notifier", "Context Builder"] + \
        ([] if MVP else ["History Service", "Correlation Engine", "ITSM Sync"])
FUNCS = [
    ("Ф-01", "Сборка коннекторов в low-code конструкторе", [0, 1, 3], "П-07, П-01", "БП-2"),
    ("Ф-02", "Приём событий и ack источнику", [2, 3], "П-01, П-02", "БП-1"),
    ("Ф-03", "Парсинг и нормализация по шаблону", [3, 5], "П-03", "БП-1"),
    ("Ф-04", "Привязка к КЕ, облачные плавающие ID", [5, 6, 7], "П-03, П-06", "БП-1, БП-4"),
    ("Ф-05", "Схлопывание дублей, жизненный цикл тревоги", [6], "П-03", "БП-1"),
    ("Ф-06", "Алерты по RED и USE", [3, 4], "П-01, П-03", "БП-1"),
    ("Ф-07", "Карта CMDB и РСМ", [0, 1, 7], "П-06, П-07", "БП-4"),
    ("Ф-08", "Шаблонизация событий", [5, 8, 10], "П-03, П-04, П-08", "БП-1, БП-2"),
    ("Ф-09", "Передача в PagerDuty и возврат статусов", [3, 6, 8] + ([] if MVP else [13]), "П-04, П-05", "БП-1"),
    ("Ф-10", "Окна обслуживания и подавление", [0, 1, 6], "П-07", "БП-3"),
    ("Ф-11", "Администрирование, аудит, самоконтроль", [0, 1, 9], "П-07", "—"),
    ("Ф-12", "Резервное оповещение, когда PagerDuty недоступен", [1, 8, 9, 10], "П-08, П-09", "БП-5"),
    ("Ф-13", "Дашборд инцидентов (поиск, фильтры)", [0, 1] + ([] if MVP else [12]), "П-07", "БП-6"),
    ("Ф-14", "Контекст инцидента в Grafana", [0, 1, 11], "П-07, П-10", "БП-6"),
    ("Ф-15", "Ролевая модель и синхронизация с каталогом", [1, 11], "П-07, П-10", "БП-7"),
]
if TGT:
    FUNCS += [("Ф-16", "История и аналитика", [0, 1, 12], "П-07", "—"),
              ("Ф-17", "Топологическая корреляция", [6, 13], "П-03", "БП-1"),
              ("Ф-18", "Синхронизация с корпоративной CMDB и ITSM", [7, 14], "П-11", "БП-1, БП-4")]
FX, FW, CWc, RHt, HY = 40, 330, 80, 32, 70
FLW, BPW = 120, 90
FLX = FX + FW + len(CONTS) * CWc
p.node("h0", "Функция", FX, HY, FW, 50, HDR)
for j, n in enumerate(CONTS):
    p.node(f"hc{j}", n.replace(" ", "<br>").replace("Веб-", "Веб-<br>"), FX + FW + j * CWc, HY, CWc, 50, HDR)
p.node("hfl", "Потоки", FLX, HY, FLW, 50, HDR)
p.node("hbp", "Процесс", FLX + FLW, HY, BPW, 50, HDR)
for i, (fid, name, cs, fl, bp) in enumerate(FUNCS):
    y = HY + 50 + i * RHt
    p.node(f"f{i}", f"<b>{fid}</b> {escape(name)}", FX, y, FW, RHt, FN)
    for j in range(len(CONTS)):
        p.node(f"c{i}_{j}", "●" if j in cs else "", FX + FW + j * CWc, y, CWc, RHt, CELL_ON if j in cs else CELL)
    p.node(f"fl{i}", fl, FLX, y, FLW, RHt, CELL_TXT)
    p.node(f"bp{i}", bp, FLX + FLW, y, BPW, RHt, CELL_TXT)
TY = HY + 50 + len(FUNCS) * RHt + 40
p.text("<b>Потоки между функциями</b>", 40, TY, 400, 24, NOTE + "fontSize=14;")
BWf, BHf = 150, 60
R1, R2, R3, R4 = TY + 70, TY + 180, TY + 290, TY + 390


def fcx(i):
    return 40 + i * 250


def fbox(k, n, f, col, y, st=CONT):
    lab = f"<b>{escape(n)}</b>" + (f"<br><font style='font-size:10px'>{f}</font>" if f else "")
    p.node(k, lab, fcx(col), y, BWf, BHf, st)


CH_ = [("src", "Источник", EXT, "Ф-02"), ("cr", "Приём и ack", CONT, "Ф-02"), ("nm", "Парсинг и КЕ", CONT, "Ф-03, Ф-04, Ф-08"),
       ("ae", "Дедуп, окна, влияние", CONT, "Ф-05, Ф-10")] + \
      ([] if MVP else [("corr", "Корреляция", CONT, "Ф-17")]) + \
      [("pg", "Передача", CONT, "Ф-08, Ф-09"), ("pd", "PagerDuty", EXT, "")]
COL = {}
for i, (k, n, st, f) in enumerate(CH_):
    fbox(k, n, f, i, R1, st)
    COL[k] = i
CHAIN = [("src", "cr", "П-01,<br>П-02"), ("cr", "nm", "П-03<br>events.raw"), ("nm", "ae", "П-03<br>events.norm"),
         ("ae", "corr" if TGT else "pg", "alerts.<br>changed")] + ([("corr", "pg", "incidents.<br>changed")] if TGT else []) + \
        [("pg", "pd", "П-04")]
for (a, b, l) in CHAIN:
    p.edge(a, b, l)
# П-05: статусы PagerDuty возвращаются в Alert Engine (поверх ряда)
p.edge("pd", "ae", "П-05: статусы → ack и silence в источники", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;",
       points=[(fcx(COL["pd"]) + 75, TY + 45), (fcx(COL["ae"]) + 75, TY + 45)], exit=(0.5, 0), entry=(0.5, 0))
# второй ряд: RED/USE, карта CMDB, резервное оповещение
fbox("rule", "RED и USE", "Ф-06", 1, R2)
p.edge("rule", "nm", "events.raw", EDGE_ORTHO, points=[(fcx(1) + 75, R2 - 25), (fcx(2) + 35, R2 - 25)], exit=(0.5, 0), entry=(round(35 / BWf, 4), 1))
fbox("cmdb", "Карта CMDB", "Ф-07 · П-06", 2, R2)
p.edge("cmdb", "ae", "cmdb.changed", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(fcx(3) + 35, R2 + 30)], exit=(1, 0.5), entry=(round(35 / BWf, 4), 1))
fbox("fbn", "Резервное оповещение", "Ф-08, Ф-12", COL["pg"], R2)
p.edge("pg", "fbn", "не принято<br>за 2 мин", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
fbox("fbx", "Резервные каналы", "", COL["pd"], R2, EXT)
p.edge("fbn", "fbx", "П-08")
# третий ряд: каталог → роли → дашборд → контекст → Grafana
c0 = 1 if MVP else 2
fbox("dir", "Каталог (AD)", "", c0, R3, EXT)
fbox("rbac", "Ролевая модель", "Ф-15", c0 + 1, R3)
fbox("dash", "Дашборд инцидентов", "Ф-13", c0 + 2, R3)
fbox("ctxb", "Контекст в Grafana", "Ф-14", c0 + 3, R3)
fbox("grf", "Grafana", "", c0 + 4, R3, EXT)
p.edge("dir", "rbac", "П-07<br>LDAPS, SCIM")
p.edge("rbac", "dash", "роли, области,<br>предустановки")
p.edge("dash", "ctxb", "клик:<br>context.request")
p.edge("ctxb", "grf", "П-10")
src_dash = "ae" if MVP else "corr"
p.edge(src_dash, "dash", "alerts.changed<br>(WebSocket)" if MVP else "incidents.changed<br>(WebSocket)", EDGE_DASH, exit=(0.75, 1), entry=(0.75, 0))
p.edge("rbac", "ctxb", "команды и права папок Grafana", EDGE_ORTHO,
       points=[(fcx(c0 + 1) + 75, R4), (fcx(c0 + 3) + 75, R4)], exit=(0.5, 1), entry=(0.5, 1))
p.text("Ф-01 (сборка коннекторов) и Ф-11 (администрирование, аудит, самоконтроль)" + ("" if MVP else ", Ф-16 (история и аналитика) и Ф-18 (обмен с CMDB и ITSM)") +
       " на нижней схеме не показаны: они работают с конфигурацией и данными, но не лежат на пути события от источника до дежурного.",
       40, R4 + 30, 1600, 40)
p.legend(labels={CONT: "Функциональный блок Umbrella", EXT: "Внешняя система"},
         extra=[(FN, "Функция Ф-NN"), (CELL_ON, "● функция выполняется в контейнере"), (CELL_TXT, "Поток П-NN и бизнес-процесс БП-N")],
         skip=())

# ---------------------------------------------------------------------
# Проведение сеанса (последовательность)
# ---------------------------------------------------------------------
p = Page("Проведение сеанса", "seq"); pages.append(p)
p.text("<b>Проведение сеанса: одно событие от источника до ack и resolve (pull-источник)</b>", 40, 20, 1500, 30, TITLE)
PART = [("src", "Источник", LIFE_EXT), ("cr", "Connector Runtime", LIFE), ("nats", "NATS JetStream", LIFE),
        ("nm", "Normalizer", LIFE), ("ae", "Alert Engine", LIFE)] + ([] if MVP else [("corr", "Correlation Engine", LIFE)]) + \
       [("pdg", "PagerDuty Gateway", LIFE), ("pd", "PagerDuty", LIFE_EXT), ("duty", "Дежурный", LIFE_EXT)]
LX0, LSP, LW = 40, 190, 150
LXc = {}
Y0, YEND = 70, 1150
for i, (k, n, st) in enumerate(PART):
    x = LX0 + i * LSP + (70 if k in ("pd", "duty") else 0)
    p.node(k, f"<b>{escape(n)}</b>", x, Y0, LW, 44, st)
    LXc[k] = x + LW / 2
I_LIFE = len(p.cells)  # линии жизни вставляются под сообщения и блоки
MSG = [
    ("cr", "src", "1. запрос событий since = курсор<br>(HTTPS API или SQL)", EDGE),
    ("src", "cr", "2. пачка событий", EDGE_DASH),
    ("cr", "cr", "3. парсинг, проверка inbox (source + external_id)", None),
    ("cr", "nats", "4. publish events.raw", EDGE),
    ("nats", "cr", "5. PubAck: записано в 3 репликах", EDGE_DASH),
    ("cr", "src", "6. ack / UPDATE, сдвиг курсора", EDGE),
    ("nats", "nm", "7. events.raw", EDGE),
    ("nm", "nm", "8. маппинг, шаблон, кандидат КЕ, сигнал, ключ", None),
    ("nm", "nats", "9. events.norm", EDGE),
    ("nats", "ae", "10. events.norm", EDGE),
    ("ae", "ae", "11. привязка к КЕ, схлопывание, окна обслуживания, РСМ", None),
    ("ae", "nats", "12. alerts.changed (тревога #1842 открыта)", EDGE),
]
if MVP:
    MSG += [
        ("nats", "pdg", "13. alerts.changed", EDGE),
        ("pdg", "pd", "14. Events API v2: trigger,<br>dedup_key umb-1842,<br>links: /go/incidents/1842/grafana", EDGE),
    ]
else:
    MSG += [
        ("nats", "corr", "13. alerts.changed", EDGE),
        ("corr", "corr", "14. тревога входит в инцидент #77 по топологии, вероятная причина", None),
        ("corr", "nats", "15. incidents.changed (инцидент #77)", EDGE),
        ("nats", "pdg", "16. incidents.changed", EDGE),
        ("pdg", "pd", "17. Events API v2: trigger,<br>dedup_key umb-inc-77,<br>links: /go/incidents/77/grafana", EDGE),
    ]
n0 = len(MSG) + 1
INC = "тревога #1842" if MVP else "инцидент #77"
MSG += [
    ("pd", "pdg", f"{n0}. 202 Accepted", EDGE_DASH),
    ("pd", "duty", f"{n0 + 1}. звонок, push, SMS", EDGE),
    ("duty", "pd", f"{n0 + 2}. acknowledge", EDGE),
    ("pd", "pdg", f"{n0 + 3}. Webhook v3: incident.acknowledged", EDGE_DASH),
    ("pdg", "nats", f"{n0 + 4}. pd.inbound", EDGE),
    ("nats", "ae", f"{n0 + 5}. pd.inbound: {INC} → ack" + ("" if MVP else " тревог"), EDGE),
    ("ae", "cr", f"{n0 + 6}. sync-back: connector.command (ack в источниках)", EDGE),
    ("cr", "src", f"{n0 + 7}. ack или silence в источнике", EDGE),
]
NLAST = n0 + 7
y = 150
for a, b, lab, st in MSG:
    if st is None:
        p.node(f"self{y}", lab, LXc[a] - 110, y - 12, 220, 36, TASK + "fontSize=10;")
        y += 52
        continue
    # подпись — в первом промежутке от отправителя, чтобы не пересекать чужие линии жизни
    right = LXc[b] > LXc[a]
    nxt = sorted(v for v in LXc.values() if (v > LXc[a] if right else v < LXc[a]))
    gap = abs((nxt[0] if right else nxt[-1]) - LXc[a])
    lw = gap - 16
    nl = sum(max(1, -(-len(re.sub("<[^>]+>", "", seg)) * 5.6 // lw)) for seg in lab.split("<br>"))
    y += 14 * (nl - 1)
    p.line(LXc[a], y, LXc[b], y, "", st)
    lx_ = LXc[a] + 8 if right else LXc[a] - 8 - lw
    p.text(lab, lx_, y - 14 * nl - 3, lw, 14 * nl, SMALL + ("align=left;" if right else "align=right;") + "verticalAlign=bottom;spacing=0;")
    y += 40
n_before = len(p.cells)
for k in LXc:
    p.line(LXc[k], Y0 + 44, LXc[k], y - 10, "", LLINE)
life = p.cells[n_before:]
del p.cells[n_before:]
p.cells[I_LIFE:I_LIFE] = life
p.node("res", f"<b>{NLAST + 1}–{NLAST + 4}. Resolve</b><br><font style='font-size:10px'>источник вернулся в норму → тот же путь через шину; "
               + ("Alert Engine закрывает тревогу, только когда в норме все её источники; PagerDuty Gateway шлёт resolve с тем же dedup_key</font>" if MVP else
                  "Correlation Engine закрывает инцидент, когда закрыты все его тревоги; PagerDuty Gateway шлёт resolve с тем же dedup_key</font>"),
       LXc["src"] - 60, y, LXc["pd"] - LXc["src"] + 120, 50, TASK_HOT + LEFT)
y += 80
p.node("fail", "<b>Сбои внутри сеанса</b><br><font style='font-size:11px'>"
                "• нет PubAck (шаг 5) → нет ack источнику, повторный запрос на следующем цикле, повтор отсеет inbox<br>"
                f"• PagerDuty не отвечает (шаг {n0}) → запись остаётся в outbox, повторы с backoff; если тревогу error или critical PagerDuty не принял за 2 мин — "
                "резервное оповещение (схема «Резервное оповещение»)<br>"
                f"• webhook потерян (шаг {n0 + 3}) → PagerDuty Gateway сверяет статус инцидента по REST API раз в 5 мин</font>",
       40, y, LXc["duty"] + 75 - 40, 80, L_TEAM + LEFT + "verticalAlign=top;spacingTop=6;")
p.legend(labels={L_TEAM: "Обработка сбоев", TASK_HOT: "Свёрнутые шаги: resolve"},
         extra=[(TASK + "fontSize=10;", "Внутренняя обработка участника")],
         skip=(TASK,),
         edges=[(EDGE, "Вызов или сообщение"), (EDGE_DASH, "Ответ, подтверждение или webhook"), (LLINE, "Линия жизни участника")])

# ---------------------------------------------------------------------
# Модель данных
# ---------------------------------------------------------------------
p = Page("Модель данных", "erd"); pages.append(p)
p.text("<b>Логическая модель данных Umbrella (PostgreSQL)</b>", 40, 20, 1300, 30, TITLE)
ENT_RBAC = ENT.replace("#DAE8FC", "#E1D5E7").replace("#6C8EBF", "#9673A6")
GRP_E = NODE_BOUND + "fontStyle=1;fontSize=13;"
ENT_W, EGAP, ECOL = 230, 40, 270


def ent(key, name, attrs, x, y, st, w=ENT_W):
    lines = attrs.split("|")
    h = 30 + 14 * len(lines)
    body = "<br>".join(escape(a.strip()) for a in lines)
    p.node(key, f"<div style='font-size:12px;text-align:center'><b>{escape(name)}</b></div><hr size='1'>{body}", x, y, w, h, st)
    return h


def ecol(x, y0, items):
    """Колонка сущностей сверху вниз; возвращает нижнюю границу."""
    y = y0
    for key, name, attrs, st in items:
        y += ent(key, name, attrs, x, y, st) + EGAP
    return y - EGAP


REL_ = EDGE + "endArrow=ERmany;startArrow=ERone;endFill=0;startFill=0;"
REL1 = EDGE + "endArrow=ERmandOne;startArrow=ERone;endFill=0;startFill=0;"
RELM = EDGE + "endArrow=ERmany;startArrow=ERmany;endFill=0;startFill=0;"


def rel(a, b, lab, ex, en, st=REL_, pts=None):
    p.edge(a, b, lab, st + ("edgeStyle=orthogonalEdgeStyle;" if pts else ""), points=pts, exit=ex, entry=en)


def ey(key, frac):
    x, y, w, h = p.geo[key]
    return y + h * frac


# --- группа 1: конфигурация ---
GX1, GY1 = 40, 70
CX = [GX1 + 20 + i * ECOL for i in range(4)]
Y1 = GY1 + 40
b1 = max(
    ecol(CX[0], Y1, [("source", "Source · источник", "id PK | name | kind: api, db, queue, cloud, push | ack_mode", ENT_CFG),
                     ("conn", "Connector · коннектор", "id PK | source_id FK | name | current_version_id FK | secret_ref → хранилище секретов", ENT_CFG),
                     ("ver", "ConnectorVersion", "id PK | connector_id FK | n | graph: блоки (JSON) | status: draft, published | author, created_at", ENT_CFG)]),
    ecol(CX[1], Y1, [("tpl", "EventTemplate · шаблон", "id PK | source_id FK | mapping: JSONata, CEL | summary, severity | dedup_labels", ENT_CFG),
                     ("rule", "Rule · правило RED/USE", "id PK | method: red, use | target: сервис, тип КЕ | expr, threshold, window", ENT_CFG),
                     ("disc", "DiscoveryRule · обнаружение", "id PK | connector_id FK → Connector | object_types | match: теги, группы ресурсов | schedule | apply: авто или ревью", ENT_CFG)]),
    ecol(CX[2], Y1, [("crule", "ContextRule · связь контекста", "id PK | condition: тип КЕ, сигнал, сервис | traversal: тип связи, направление, глубина ≤ 3 | layout (JSON) | time_window: −10 / +10 мин | version, status", ENT_CFG),
                     ("ptpl", "PanelTemplate · шаблон панели", "id PK | kind: метрики, логи, трейсы, сводка | datasource_uid (Grafana) | query_template: ${ci.host}, ${service} | allowed_vars", ENT_CFG),
                     ("mw", "MaintenanceWindow · окно", "id PK | scope: сервисы, КЕ | starts_at, ends_at | author, approver (два ключа)", ENT_CFG)]),
    ecol(CX[3], Y1, [("bch", "BackupChannel · резервный канал", "id PK | kind: " + ("email, webhook" if MVP else "email, sms, voice, | messenger, webhook, alt_platform") +
                      " | config | secret_ref → хранилище секретов | priority | last_test_at, last_test_ok", ENT_CFG),
                     ("bct", "BackupContact · резервный контакт", "id PK | team_id FK → Team | name | contact_ref (e-mail, телефон) | priority", ENT_CFG)]),
)
GW1 = 3 * ECOL + ENT_W + 40
p.node("g_cfg", "Конфигурация: коннекторы, обработка, контекст, оповещение", GX1, GY1, GW1, b1 - GY1 + 20, GRP_E)
rel("source", "conn", "1 : N", (0.5, 1), (0.5, 0))
rel("conn", "ver", "1 : N", (0.5, 1), (0.5, 0))
rel("source", "tpl", "1 : N", (1, 0.5), (0, 0.43))
rel("crule", "ptpl", "N : M", (0.5, 1), (0.5, 0), RELM)

# --- группа 2: CMDB и РСМ ---
GX2 = GX1 + GW1 + 40
DX = [GX2 + 40 + i * ECOL for i in range(3)]
b2 = max(
    ecol(DX[0], Y1, [("ci", "CI · КЕ", "id PK | type | name | attrs (JSONB) | state: active, stale | origin: discovery, manual", ENT_CMDB),
                     ("cid", "CIIdentity · история ID", "ci_id FK | provider | external_id (ID, GUID) | valid_from, valid_to", ENT_CMDB),
                     ("rel", "Relation · связь", "from_ci FK | to_ci FK | type: runs_on, member_of | origin", ENT_CMDB)]),
    ecol(DX[1], Y1, [("its", "ITService · ИТ-сервис", "id PK | name | calc: худший, N из M | team_id FK | pd_service_id FK", ENT_CMDB),
                     ("bs", "BusinessService · бизнес-услуга", "id PK | name | criticality, SLA | owner", ENT_CMDB),
                     ("mapv", "MapVersion · версия карты", "id PK | changes | status: auto, review, applied | reviewer", ENT_CMDB)]),
    ecol(DX[2], Y1, [("team", "Team · команда", "id PK | name | directory_group_id FK", ENT_CMDB),
                     ("pds", "PDService · сервис PagerDuty", "id PK | pd_id | routing_key_ref → хранилище секретов", ENT_EXT)]),
)
GW2 = 2 * ECOL + ENT_W + 60
p.node("g_cmdb", "CMDB и ресурсно-сервисная модель", GX2, GY1, GW2, b2 - GY1 + 20, GRP_E)
rel("ci", "cid", "1 : N", (0.5, 1), (0.5, 0))
rel("ci", "rel", "1 : N", (0, 0.5), (0, 0.5), pts=[(DX[0] - 20, ey("ci", 0.5)), (DX[0] - 20, ey("rel", 0.5))])
rel("its", "ci", "N : M", (0, 0.3), (1, round((ey("its", 0.3) - p.geo["ci"][1]) / p.geo["ci"][3], 4)), RELM)
rel("its", "bs", "N : M", (0.5, 1), (0.5, 0), RELM)
rel("team", "its", "1 : N", (0, 0.5), (1, round((ey("team", 0.5) - p.geo["its"][1]) / p.geo["its"][3], 4)))
xg = DX[2] - 20
rel("pds", "its", "1 : N", (0, 0.5), (1, 0.8), pts=[(xg, ey("pds", 0.5)), (xg, ey("its", 0.8))])

# --- группа 3: операционные данные ---
GY3 = max(b1, b2) + 60
Y3 = GY3 + 40
INC_FK = "" if MVP else " | incident_id FK → Incident"
OBJ = "alert_id" if MVP else "incident_id"
OX = CX
cols3 = [
    [("event", "Event · событие", "id PK | source_id FK → Source | external_id | ci_id FK → CI | signal, method | severity, value, ts | dedup_key | alert_id FK | raw (JSONB, 7 дней)", ENT),
     ("cursor", "Cursor · курсор", "connector_id PK, FK → Connector | position | updated_at", ENT),
     ("inbox", "Inbox", "source_id + external_id PK | received_at | expires_at (7 дней)", ENT)],
    [("alert", "Alert · тревога", "id PK | dedup_key UQ | ci_id FK → CI | signal, method: red, use, other | severity = max: critical, error, warning, info | "
      "status: open, acknowledged, resolved, closed | first_seen, last_seen, count" + INC_FK, ENT),
     ("pdl", "PagerDutyLink", f"{OBJ} PK, FK | dedup_key: " + ("umb-‹id тревоги›" if MVP else "umb-inc-‹id›") + " | pd_incident_id | pd_status, synced_at", ENT),
     ("outbox", "Outbox", f"id PK | {OBJ} FK | action: trigger, acknowledge, resolve | attempts, next_try_at", ENT)],
    [("notif", "Notification · уведомление", "id PK | alert_id FK | channel_id FK → BackupChannel | recipient_ref | status: sent, delivered, acked, failed | ack_token_hash, expires_at | sent_at", ENT),
     ("sil", "Silence · заглушение", "id PK | scope: тревоги, КЕ, сервис | until | reason | author", ENT),
     ("audit", "AuditLog · аудит", "id PK | actor | action | object_type, object_id | before, after | ts", ENT)],
    [("gdash", "GrafanaDashboard · дашборд Grafana", f"{OBJ} PK, FK | uid: umb-‹id› | folder_uid (папка команды) | context_rule_versions | generated_at | delete_after: закрытие + 30 дней", ENT),
     ("onc", "OnCallCache · кеш дежурств", "pd_schedule_id | level | user_ref | contact_ref (e-mail, телефон) | from, to | synced_at"
      + ("" if MVP else " | escalation_timeout"), ENT_EXT)] +
    ([] if MVP else [("inc", "Incident · инцидент", "id PK | root_alert_id FK | probable_cause_ci FK → CI | status: open, acknowledged, resolved, closed | itsm_ref", ENT),
                     ("itl", "ITSMLink · связь с ITSM", "object_type: ci, service, incident | local_id | itsm_id | direction | synced_at", ENT_EXT)]),
]
b3 = max(ecol(OX[i], Y3, c) for i, c in enumerate(cols3))
p.node("g_ops", "Операционные данные", GX1, GY3, GW1, b3 - GY3 + 20, GRP_E)
rel("alert", "event", "1 : N", (0, 0.2), (1, round((ey("alert", 0.2) - p.geo["event"][1]) / p.geo["event"][3], 4)))
rel("alert", "notif", "1 : N", (1, 0.2), (0, round((ey("alert", 0.2) - p.geo["notif"][1]) / p.geo["notif"][3], 4)))
if MVP:
    rel("alert", "pdl", "1 : 1", (0.5, 1), (0.5, 0), REL1)
    xo = OX[1] + ENT_W + 20
    rel("alert", "outbox", "1 : N", (1, 0.85), (1, 0.5), pts=[(xo, ey("alert", 0.85)), (xo, ey("outbox", 0.5))])
else:
    rel("pdl", "outbox", "1 : N", (0.5, 1), (0.5, 0))

# --- группа 4: ролевая модель и доступ ---
RXc = DX
b4 = max(
    ecol(RXc[0], Y3, [("user", "User · пользователь", "id PK | login (из IdP) | name, e-mail | origin: каталог, break-glass | last_login, disabled_at", ENT_RBAC),
                      ("view", "SavedView · личный вид", "id PK | user_id FK | preset_id FK → DashboardPreset | filters (не шире области) | share_url", ENT_RBAC)]),
    ecol(RXc[1], Y3, [("dgrp", "DirectoryGroup · группа каталога", "id PK | dn или scim_id | name (UMB-*) | synced_at", ENT_RBAC),
                      ("gbind", "GroupBinding · привязка", "id PK | group_id FK | role_id FK | scope_id FK | preset_id FK | priority | approved_by (два ключа)", ENT_RBAC),
                      ("scope", "Scope · область", "id PK | business_services | it_services | teams | connectors, sources", ENT_RBAC)]),
    ecol(RXc[2], Y3, [("role", "Role · роль", "id PK | name | builtin", ENT_RBAC),
                      ("perm", "Permission · разрешение", "code PK, напр. incident.ack | description", ENT_RBAC),
                      ("preset", "DashboardPreset · предустановка", "id PK | filters | columns, grouping, sorting | widgets, auto_refresh | context_rules, grafana_folder | version", ENT_RBAC)]),
)
p.node("g_rbac", "Ролевая модель и доступ", GX2, GY3, GW2, b4 - GY3 + 20, GRP_E)
rel("user", "dgrp", "N : M", (1, 0.3), (0, round((ey("user", 0.3) - p.geo["dgrp"][1]) / p.geo["dgrp"][3], 4)), RELM)
rel("user", "view", "1 : N", (0.5, 1), (0.5, 0))
rel("dgrp", "gbind", "1 : N", (0.5, 1), (0.5, 0))
rel("scope", "gbind", "1 : N", (0.5, 0), (0.5, 1))
rel("role", "perm", "N : M", (0.5, 1), (0.5, 0), RELM)
xr = RXc[2] - 20
rel("role", "gbind", "1 : N", (0, 0.6), (1, 0.25), pts=[(xr, ey("role", 0.6)), (xr, ey("gbind", 0.25))])
rel("preset", "gbind", "1 : N", (0, 0.3), (1, 0.75), pts=[(xr, ey("preset", 0.3)), (xr, ey("gbind", 0.75))])

p.text(("Горячие данные (нормализованные события 30 дней, сырые 7 дней, тревоги, аудит) лежат в PostgreSQL; ClickHouse в MVP не используется. " if MVP else
        "Горячие данные (события 7 дней, сырые 3 дня, тревоги, инциденты, аудит) лежат в PostgreSQL; история событий и тревог 13 месяцев — в ClickHouse (таблицы events, alerts, notifications). "
        "Каждая тревога входит в инцидент (Alert.incident_id); в PagerDuty уходит один алерт на инцидент. ") +
       "Линиями показаны связи внутри групп; связи между группами — внешними ключами «FK → Сущность». "
       "Все секреты (логины, пароли, токены, ключи API, routing_key) хранятся только в хранилище секретов (OpenBao), в БД лежат ссылки. Inbox чистится по expires_at. "
       "Контакты дежурных в кеше хранятся ссылками и обновляются из PagerDuty каждые 5 минут" + ("" if MVP else ", расписания — на 7 дней вперёд") +
       "; резервные контакты команды используются, если кеш пуст или старше 24 часов" + ("" if MVP else " и после последнего уровня эскалации") + ".",
       40, max(b3, b4) + 40, GW1 + GW2 + 40, 56)
p.legend(labels={NODE_BOUND: "Группа сущностей"}, extra=[(ENT_RBAC, "Сущность ролевой модели и доступа")],
         edges=[(REL_, "Связь сущностей; подпись — кардинальность: 1 : 1, 1 : N, N : M")])

# ---------------------------------------------------------------------
# Слои ИТ-модели
# ---------------------------------------------------------------------
p = Page("Слои ИТ-модели", "lay"); pages.append(p)
p.text("<b>Слои ИТ-модели Umbrella: от бизнеса до инфраструктуры</b>", 40, 20, 1300, 30, TITLE)
LAY_T = BASE + "rounded=0;fontStyle=1;fontSize=12;"
LAYERS = [
    ("Бизнес-слой", L_BIZ, ["Бизнес-услуги компании<br>(Интернет-банк и др.)", "Процессы БП-1…БП-7",
                            "Роли: инженер мониторинга, дежурный,<br>владелец сервиса, аудитор",
                            "Ценность: один алерт на проблему,<br>быстрый MTTA, контекст в Grafana"]),
    ("Прикладной слой", CONT, ["Веб-интерфейс, Core API,<br>Context Builder", "Connector Runtime,<br>Ingest Gateway",
                               "Normalizer, Rule Engine,<br>Alert Engine" + ("" if MVP else ", Correlation Engine"),
                               "CMDB Discovery, PagerDuty Gateway,<br>Fallback Notifier, Watchdog" + ("" if MVP else "<br>History Service, ITSM Sync")]),
    ("Слой данных", DB, ["PostgreSQL: конфигурация,<br>CMDB и РСМ, тревоги, аудит", "NATS JetStream:<br>потоки событий",
                         ("S3 (другая площадка): бэкапы<br>PostgreSQL, снапшоты OpenBao" if MVP else "ClickHouse:<br>история 13 месяцев"),
                         ("Хранилище секретов:<br>OpenBao ×3, Raft" if MVP else "Секреты: OpenBao ×3, Raft;<br>S3 (другой регион): бэкапы")]),
    ("Технологический слой", L_TEAM, ["Go; React, TypeScript,<br>React Flow", "CEL, JSONata, Grok, RE2", "Helm, GitOps; метрики<br>OpenMetrics и OpenTelemetry",
                                      "TLS, mTLS, OIDC, LDAPS, SCIM,<br>RBAC, SBOM, подпись образов"]),
    ("Инфраструктурный слой", L_MON, ["Kubernetes: 3 app-, 3 data-узла,<br>3 узла control plane" if MVP else "Kubernetes: 5 app-, 3 data-,<br>3 ClickHouse-узла",
                                      "Сетевые зоны: DMZ, приложения,<br>данные, внутренняя сеть", "Reverse proxy + WAF,<br>egress-прокси",
                                      "Облако или свой ЦОД;<br>пилот: 3 VM и Docker Compose" if MVP else "Две площадки:<br>основная и тёплый резерв"]),
]
LY0, LH_, LGAP = 70, 90, 30
for i, (n, st, items) in enumerate(LAYERS):
    y = LY0 + i * (LH_ + LGAP)
    p.node(f"lt{i}", n, 40, y, 190, LH_, st.replace("fontSize=12;", "") + "fontStyle=1;fontSize=13;")
    for j, it in enumerate(items):
        p.node(f"l{i}_{j}", it, 260 + j * 300, y + 10, 280, LH_ - 20, st.replace("fontSize=12;", "") + "fontSize=11;")
    if i:
        p.edge(f"lt{i}", f"lt{i - 1}", "обслуживает", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1))
# внешние приложения — боковая колонка «интеграция»
EXX, EXW = 1510, 300
EXTS = ["Системы мониторинга и их БД,<br>брокеры, syslog", "Облачные провайдеры", "PagerDuty (SaaS) и<br>резервные каналы",
        "Grafana и хранилища<br>метрик, логов и трейсов", "IdP (OIDC, SCIM) и<br>каталог AD (LDAPS)", "SIEM, внешний heartbeat"] + \
       ([] if MVP else ["Корпоративная CMDB и ITSM"])
EXH = (5 * (LH_ + LGAP) - LGAP - 44 - 10 * len(EXTS)) / len(EXTS)
p.node("extc", "<b>Внешние приложения · интеграция</b>", EXX, LY0, EXW, 5 * (LH_ + LGAP) - LGAP, NODE_BOUND)
for j, it in enumerate(EXTS):
    p.node(f"ex{j}", it, EXX + 15, LY0 + 38 + j * (EXH + 10), EXW - 30, EXH, EXT.replace("fontSize=12;", "") + "fontSize=11;")
INTEG = EDGE + "startArrow=block;startFill=1;"
p.edge("l1_3", "extc", "", INTEG, exit=(1, 0.5), entry=(0, round((LY0 + LH_ + LGAP + LH_ / 2 - LY0) / (5 * (LH_ + LGAP) - LGAP), 4)))
p.text("интеграция", 1440, LY0 + LH_ + LGAP + 18, 70, 16, SMALL + "align=center;")
p.text("Каждый слой обслуживает вышестоящий. Внешние приложения стоят сбоку, на уровне прикладного слоя: Umbrella их не заменяет, а интегрируется с ними — "
       "собирает события (системы мониторинга, облака), передаёт результат (PagerDuty, резервные каналы), строит контекст в Grafana, "
       "берёт пользователей и группы из IdP и каталога" + ("" if MVP else ", обменивается данными с CMDB и ITSM") +
       ". Платные российские продукты не используются; open source допускается.",
       40, LY0 + 5 * (LH_ + LGAP) + 10, EXX + EXW - 40, 40)
p.legend(auto=False, extra=[(st.replace("fontSize=12;", "") + "fontSize=11;", n) for n, st, _ in LAYERS] +
         [(EXT.replace("fontSize=12;", "") + "fontSize=11;", "Внешнее приложение"), (NODE_BOUND, "Колонка внешних приложений")],
         edges=[(EDGE_DASH, "Нижний слой обслуживает верхний"), (INTEG, "Интеграция: обмен в обе стороны")])

# ---------------------------------------------------------------------
# Компоненты и интерфейсы
# ---------------------------------------------------------------------
p = Page("Компоненты и интерфейсы", "ifc"); pages.append(p)
p.text("<b>Компоненты и интерфейсы: что каждый контейнер предоставляет и что использует</b>", 40, 20, 1500, 30, TITLE)
IST = CONT + LEFT + "verticalAlign=top;spacingTop=6;fontSize=11;"
IXT = EXT.replace("fontSize=12;", "fontSize=11;")
LEG_ST = "endArrow=none;html=1;strokeColor=#3C7FC0;strokeWidth=2;"


def box(key, name, prov, req, x, y, w=220, h=160, st=IST):
    lab = f"<b>{escape(name)}</b><br><font style='font-size:10px'>" + (f"● {prov}<br>" if prov else "") + f"○ {req}</font>"
    p.node(key, lab, x, y, w, h, st)


def lolli(key, owner, side, frac, label="", lpos="right", lw=140):
    """Предоставляемый интерфейс: кружок на ножке у стороны владельца; подпись слева или справа от кружка."""
    x, y, w, h = p.geo[owner]
    if side == "left":
        cx, cy, lx1, ly1 = x - 34, y + h * frac - 9, x, y + h * frac
    elif side == "right":
        cx, cy, lx1, ly1 = x + w + 16, y + h * frac - 9, x + w, y + h * frac
    elif side == "top":
        cx, cy, lx1, ly1 = x + w * frac - 9, y - 34, x + w * frac, y
    else:
        cx, cy, lx1, ly1 = x + w * frac - 9, y + h + 16, x + w * frac, y + h
    p.node(key, "", cx, cy, 18, 18, IFACE)
    p.line(lx1, ly1, cx + 9, cy + 9, "", LEG_ST)
    if label:
        lh = 14 * (label.count("<br>") + 1)
        if lpos == "right":
            p.text(label, cx + 24, cy + 9 - lh / 2 - 1, lw, lh + 2, SMALL + "fontStyle=1;fontColor=#1F4E79;spacing=0;verticalAlign=middle;")
        elif lpos == "left":
            p.text(label, cx - 6 - lw, cy + 9 - lh / 2 - 1, lw, lh + 2, SMALL + "fontStyle=1;fontColor=#1F4E79;spacing=0;verticalAlign=middle;align=right;")
        elif lpos == "aboveleft":
            p.text(label, cx + 9 - lw, cy - lh - 4, lw, lh, SMALL + "fontStyle=1;fontColor=#1F4E79;spacing=0;align=right;")
        else:
            p.text(label, cx + 9 - lw / 2, cy - lh - 4, lw, lh, SMALL + "fontStyle=1;fontColor=#1F4E79;spacing=0;align=center;")
    return cx + 9, cy + 9


def gx(i):
    return 40 + i * 250


TOPY, R1Y, BUSY, R3Y, BOTY = 70, 230, 450, 570, 820
R1H, R3H = 140, 140
# --- внешние системы сверху ---
p.node("eng", c4("Инженер", "Person", ""), gx(0), TOPY, 220, 70, PERSON)
p.node("idp", c4("IdP", "External System", "● OIDC 443; SCIM-клиент"), 290, TOPY, 140, 70, IXT)
p.node("ad", c4("Каталог (AD)", "External System", "● LDAPS 636"), 455, TOPY, 140, 70, IXT)
p.node("siem", c4("SIEM", "External System", "● syslog TLS 6514"), 620, TOPY, 140, 70, IXT)
p.node("grf", c4("Grafana", "External System", "● HTTP API 443"), gx(3), TOPY, 220, 70, IXT)
p.node("bao", c4("Хранилище секретов", "Container: OpenBao ×3, Raft", "● 8200 TLS"), gx(4), TOPY - 10, 220, 90, DB + "fontSize=11;")
p.node("fbx", c4("Резервные каналы", "External System", ""), gx(5), TOPY, 220, 70, IXT)
p.node("pd", c4("PagerDuty", "External System", ""), gx(6), TOPY, 220, 70, IXT)
p.node("hb", c4("Внешний heartbeat", "External System", ""), gx(7), TOPY, 220, 70, IXT)
# --- контейнеры над шиной ---
box("web", "Веб-интерфейс", "HTTPS 443: UI, конструктор, дашборд инцидентов", "REST, WebSocket Core API", gx(0), R1Y, 220, R1H)
box("api", "Core API", "REST /api/v1 (OpenAPI), WebSocket /ws, GET /go/incidents/{id}/grafana, SCIM 2.0 /scim/v2",
    "SQL; OIDC; LDAPS 636 (каталог); syslog TLS 6514 в SIEM; NATS: pub alerts.commands; request connector.query, context.request; "
    "sub alerts.changed, pd.delivery, notify.log, events.dlq" + ("" if MVP else ", incidents.changed"), 290, R1Y, 470, R1H)
box("cb", "Context Builder", "NATS: context.request (request/reply)",
    "Grafana HTTP API 443: дашборды, аннотации, команды и права папок; NATS: sub alerts.changed" + ("" if MVP else ", incidents.changed") +
    "; SQL: связи контекста, шаблоны панелей, КЕ, тревоги; "
    "OpenBao: токен сервисного аккаунта Grafana", gx(3), R1Y, 470, R1H)
box("fbn", "Fallback Notifier", "метрики /metrics" + ("" if MVP else "; ответы SMS и кнопки мессенджеров (Ack Handler)"),
    "SMTP 587 (релей), HTTPS webhook" + ("" if MVP else ", SMS- и голосовой шлюз") + "; NATS: sub pd.delivery, alerts.changed; pub notify.log" + ("" if MVP else ", alerts.commands") + "; SQL",
    gx(5), R1Y, 220, R1H)
box("pdg", "PagerDuty Gateway", "POST /pd/webhook (подпись)",
    "Events API v2, REST API PagerDuty; NATS: sub " + ("alerts.changed" if MVP else "incidents.changed") + "; pub pd.delivery, pd.inbound; SQL (outbox)",
    gx(6), R1Y, 220, R1H)
box("wd", "Watchdog", "метрики /metrics",
    "тестовое событие в ingest.in; проба до PagerDuty Gateway; sub pd.delivery, notify.log, alerts.changed; ping heartbeat",
    gx(7), R1Y, 220, R1H)
# --- контейнеры под шиной ---
ROW3 = [("ingest", "Ingest Gateway", "POST /ingest/{connector} (HMAC или токен)", "NATS: pub ingest.in"),
        ("cr", "Connector Runtime", "syslog TLS 6514 (приём); NATS: connector.query, connector.command",
         "API, SQL, брокеры источников; sub ingest.in; pub events.raw, events.dlq"),
        ("rule", "Rule Engine", "", "NATS: request connector.query; pub events.raw; SQL: правила"),
        ("norm", "Normalizer", "", "sub events.raw, cmdb.changed; pub events.norm, events.dlq; SQL: шаблоны, CMDB"),
        ("alert", "Alert Engine", "", "sub events.norm, pd.inbound, alerts.commands, cmdb.changed" + ("" if MVP else ", incidents.changed") +
         "; pub alerts.changed (outbox), connector.command; SQL")]
if TGT:
    ROW3 += [("corr", "Correlation Engine", "pub incidents.changed", "sub alerts.changed, cmdb.changed; SQL; ClickHouse 9440 (Pattern Learner)")]
ROW3 += [("cmdb", "CMDB Discovery", "Map API (через Core API); pub cmdb.changed", "инвентарь источников и облаков (API); SQL")]
if TGT:
    ROW3 += [("hist", "History Service", "REST /api/v1/history", "sub events.norm, alerts.changed, incidents.changed, notify.log; SQL ClickHouse 9440 TLS"),
             ("itsm", "ITSM Sync", "outbox в PostgreSQL", "REST API ITSM; sub cmdb.changed, incidents.changed; SQL")]
for i, (k, n, pr, rq) in enumerate(ROW3):
    box(k, n, pr, rq, gx(i), R3Y, 220, R3H)
BUSR = max(gx(7) + 220, gx(len(ROW3) - 1) + 220)
p.node("nats", "<b>NATS JetStream</b> · ● 4222 TLS<br><font style='font-size:10px'>ingest.in · events.raw · events.dlq · events.norm · alerts.changed · "
       "alerts.commands · pd.delivery · pd.inbound · connector.query · connector.command · cmdb.changed · notify.log" +
       ("" if MVP else " · incidents.changed") + " · request/reply context.request</font>", 40, BUSY, BUSR - 40, 64, BUS)
for k in ("api", "cb", "fbn", "pdg", "wd"):
    p.vert(k, "nats", "")
for k, *_ in ROW3:
    p.vert(k, "nats", "", down=False)
# --- связи с внешними системами сверху ---
lolli("i_ui", "web", "top", 0.5, "HTTPS 443", "right", 100)
p.edge("eng", "i_ui", "", exit=(0.5, 1))
lolli("i_oidc", "idp", "bottom", 0.4)
p.edge("api", "i_oidc", "", exit=(round((290 + 140 * 0.4 - 290) / 470, 4), 0))
lolli("i_scim", "api", "top", round((290 + 140 * 0.85 - 290) / 470, 4))
p.edge("idp", "i_scim", "", exit=(0.85, 1))
lolli("i_ldap", "ad", "bottom", 0.5)
p.edge("api", "i_ldap", "", exit=(round((455 + 70 - 290) / 470, 4), 0))
lolli("i_siem", "siem", "bottom", 0.5)
p.edge("api", "i_siem", "", exit=(round((620 + 70 - 290) / 470, 4), 0))
lolli("i_grf", "grf", "bottom", 0.5)
p.edge("cb", "i_grf", "", exit=(round(110 / 470, 4), 0))
lolli("i_bao", "bao", "bottom", 0.5)
p.edge("cb", "i_bao", "", exit=(round((gx(4) + 110 - gx(3)) / 470, 4), 0))
lolli("i_fbx", "fbx", "bottom", 0.3, "SMTP · HTTPS", "left", 90)
p.edge("fbn", "i_fbx", "", exit=(0.3, 0))
if TGT:
    lolli("i_ack", "fbn", "top", 0.75, "ответы (через<br>reverse proxy)", "right", 95)
    p.edge("fbx", "i_ack", "", EDGE_DASH, exit=(0.75, 1))
lolli("i_ev", "pd", "bottom", 0.3, "Events API v2,<br>REST", "left", 85)
p.edge("pdg", "i_ev", "", exit=(0.3, 0))
lolli("i_pdw", "pdg", "top", 0.75, "Webhooks v3", "right", 80)
p.edge("pd", "i_pdw", "", EDGE_DASH, exit=(0.75, 1))
lolli("i_hb", "hb", "bottom", 0.5, "HTTPS ping", "right", 80)
p.edge("wd", "i_hb", "", exit=(0.5, 0))
# Веб-интерфейс → Core API
lolli("i_rest", "api", "bottom", 0.08, "REST ·<br>WebSocket", "right", 70)
p.edge("web", "i_rest", "", EDGE_ORTHO, points=[(gx(0) + 110, R1Y + R1H + 25)], exit=(0.5, 1), entry=(0, 0.5))
# --- источники снизу ---
p.node("src", c4("Источники", "External System", "● API, SQL, брокеры; инвентарь"), 40, BOTY, 470, 80, IXT)
x_api = gx(1) + 50
lolli("i_src", "src", "top", round((x_api - 40) / 470, 4), "API · SQL ·<br>брокер", "left", 70)
p.edge("cr", "i_src", "pull, ack", exit=(round(50 / 220, 4), 1))
lolli("i_ing", "ingest", "bottom", 0.5, "HTTPS webhook<br>HMAC / токен", "right", 95)
p.edge("src", "i_ing", "", EDGE_DASH, exit=(round((gx(0) + 110 - 40) / 470, 4), 0))
lolli("i_sys", "cr", "bottom", 0.75, "syslog<br>TLS 6514", "right", 60)
p.edge("src", "i_sys", "", EDGE_DASH, exit=(round((gx(1) + 165 - 40) / 470, 4), 0))
icm = [k for k, *_ in ROW3].index("cmdb")
p.edge("cmdb", "src", "инвентарь (API)", EDGE_ORTHO, points=[(gx(icm) + 110, BOTY + 55)], exit=(0.5, 1), entry=(1, round(55 / 80, 4)))
# --- хранилища ---
PGX = BUSR + 130
p.node("pg", c4("PostgreSQL", "Container", ""), PGX, R3Y + 20, 200, 110, DB)
lolli("i_sql", "pg", "left", 0.5, "SQL 5432 TLS", "aboveleft", 90)
last = ROW3[-1][0]
p.edge(last, "i_sql", "", exit=(1, round((R3Y + 75 - R3Y) / R3H, 4)))
if TGT:
    ih = [k for k, *_ in ROW3].index("hist")
    p.node("ch", c4("ClickHouse", "Container: 3 узла", ""), gx(ih), BOTY + 10, 220, 90, DB)
    lolli("i_ch", "ch", "top", 0.5, "SQL 9440 TLS", "right", 90)
    p.edge("hist", "i_ch", "", exit=(0.5, 1))
    p.node("itsmx", c4("CMDB и ITSM", "External System", "● REST 443"), gx(ih + 1), BOTY + 10, 220, 80, IXT)
    lolli("i_itsm", "itsmx", "top", 0.5, "REST 443", "right", 70)
    p.edge("itsm", "i_itsm", "", exit=(0.5, 1))
p.text("● — интерфейс, который контейнер предоставляет; ○ — интерфейсы, которые он использует. Все контейнеры ходят в PostgreSQL по SQL 5432 TLS "
       "и получают секреты из хранилища секретов (OpenBao) через agent injector; на схеме показано по одной связи. "
       "Пунктир — входящий поток, который начинает внешняя система. Протоколы и порты сетевых зон — на схеме «C4-5 Сегментация сети».",
       40, BOTY + 130, BUSR - 40, 40)
p.legend(labels={CONT: "Контейнер: ● предоставляет, ○ использует", PERSON: "Пользователь", DB: "Хранилище данных или секретов"},
         edges=[(EDGE, "Использует интерфейс"), (EDGE_DASH, "Входящий поток от внешней системы"), (LEG_ST, "Предоставляет интерфейс (ножка)")])

# ---------------------------------------------------------------------
# Отказоустойчивость и восстановление
# ---------------------------------------------------------------------
p = Page("Отказоустойчивость и восстановление", "ha"); pages.append(p)
p.text("<b>Отказоустойчивость и восстановление: RPO ≤ 1 мин, RTO ≤ 1 ч (холодный резерв)</b>" if MVP else "<b>Отказоустойчивость и восстановление: RPO ≤ 1 мин, RTO ≤ 15 мин (тёплый резерв)</b>", 40, 20, 1400, 30, TITLE)
p.node("site", "<b>Основная площадка</b> [Kubernetes, без единой точки отказа]", 40, 100, 1000, 380, NODE_BOUND + "align=right;spacingRight=10;")
p.node("ingr", c4("Ingress-контроллер ×2", "на разных узлах", "за reverse proxy + WAF ×2 в DMZ (VRRP)"), 70, 125, 260, 65, CONT)
p.node("apps", c4("Сервисы Umbrella", "Deployment ×2 и больше, HPA, PDB",
                  "anti-affinity; Watchdog и CMDB Discovery ×2 с выбором лидера" + ("" if MVP else ", ITSM Sync тоже")), 70, 210, 260, 120, CONT)
p.node("ob", c4("Outbox и Fallback Notifier", "таблица PostgreSQL, контейнер",
                "тревога error или critical, не принятая PagerDuty за 2 мин, уходит по резервным каналам"), 70, 350, 260, 115, CONT)
p.edge("ingr", "apps", "", exit=(0.5, 1), entry=(0.5, 0))
p.edge("apps", "ob", "", exit=(0.5, 1), entry=(0.5, 0))
for i in range(3):
    p.node(f"n{i}", c4(f"nats-{i}", "JetStream", ""), 380 + i * 180, 130, 150, 60, BUS)
p.edge("n0", "n1", "")
p.edge("n1", "n2", "")
p.text("Raft, потоки R3", 560, 194, 150, 16, SMALL + "align=center;")
p.node("pgr", c4("PostgreSQL реплика", "синхронная, без strict-режима", "автоматическое повышение реплики ≤ 30 с"), 380, 240, 260, 90, DB)
p.node("pgp", c4("PostgreSQL primary", "CloudNativePG или Patroni", ""), 380, 375, 220, 80, DB)
p.edge("pgp", "pgr", "синхронная<br>репликация", exit=(0.5, 0), entry=(round(110 / 260, 4), 1))
p.node("bao", c4("Хранилище секретов", "Container: OpenBao ×3, Raft", ""), 620, 375, 200 if TGT else 390, 80, DB)
if TGT:
    p.node("ch", c4("ClickHouse", "Container: 3 узла, Keeper ×3", ""), 840, 375, 180, 80, DB)
p.edge("apps", "n0", "at-least-once", EDGE_ORTHO, points=[(455, 220)], exit=(1, round(10 / 120, 4)), entry=(0.5, 1))
p.edge("apps", "pgp", "SQL", EDGE_ORTHO, points=[(355, 320), (355, 415)], exit=(1, round(110 / 120, 4)), entry=(0, 0.5))
# S3 вне площадки
p.node("s3", c4("S3-хранилище", "другая площадка или регион",
                "WAL-архив (archive_timeout ≤ 60 с), полный бэкап раз в сутки, 30 дней; снапшоты OpenBao" + ("" if MVP else "; BACKUP ClickHouse раз в сутки")),
       380, 550, 640, 80, EXT)
p.edge("pgp", "s3", "WAL, бэкапы", exit=(0.5, 1), entry=(round((490 - 380) / 640, 4), 0))
p.edge("bao", "s3", "снапшоты" if MVP else "снапшоты:<br>15 мин и после изменения", exit=(0.5, 1),
       entry=(round((620 + (100 if TGT else 195) - 380) / 640, 4), 0))
if TGT:
    p.edge("ch", "s3", "BACKUP", exit=(0.5, 1), entry=(round((930 - 380) / 640, 4), 0))
# резервная площадка
p.node("dr", "<b>Резервная площадка</b> [" + ("холодный резерв" if MVP else "тёплый резерв") + "]", 1100, 100, 440, 380, NODE_BOUND + "dashed=1;")
if MVP:
    p.node("drk", c4("Kubernetes-кластер", "GitOps: Helm-чарты", "разворачивается по регламенту"), 1130, 140, 380, 90, REPL)
    p.node("drp", c4("PostgreSQL", "PITR из S3", "на момент сбоя минус ≤ 1 мин"), 1130, 300, 180, 110, REPL)
    p.node("drb", c4("OpenBao", "из снапшота в S3", ""), 1330, 300, 180, 110, REPL)
    p.edge("s3", "drp", "восстановление", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1220, 570)], exit=(1, 0.25), entry=(0.5, 1))
    p.edge("s3", "drb", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1420, 610)], exit=(1, 0.75), entry=(0.5, 1))
else:
    p.node("drn", c4("NATS", "зеркало, leafnode 7422", ""), 1130, 130, 180, 60, REPL)
    p.node("drz", c4("DMZ", "reverse proxy + WAF ×2", ""), 1330, 130, 180, 60, REPL)
    p.node("drp", c4("PostgreSQL", "асинхронная реплика", "забирает WAL с primary по 5432"), 1130, 235, 180, 100, REPL)
    p.node("drk", c4("Сервисы Umbrella", "по 1 реплике", "масштабируются при переключении"), 1330, 235, 180, 100, REPL)
    p.node("drb", c4("OpenBao", "из снапшота в S3", "RPO секретов ≤ 15 мин"), 1130, 375, 380, 80, REPL)
    p.edge("pgr", "drp", "WAL 5432 (реплика резерва забирает)", EDGE_DASH, exit=(1, 0.5), entry=(0, round((285 - 235) / 100, 4)))
    p.edge("n2", "drn", "зеркало 7422", EDGE_DASH, exit=(1, 0.5), entry=(0, 0.5), lpos=-0.3)
    p.edge("s3", "drb", "снапшоты 443", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1320, 590)], exit=(1, 0.5), entry=(round((1320 - 1130) / 380, 4), 1))
p.node("hb", c4("Внешний heartbeat", "External System", "нет ping 3 мин → тревога"), 1600, 130, 200, 100, EXT)
p.node("pd", c4("PagerDuty", "External System", ""), 1600, 300, 200, 80, EXT)
p.edge("hb", "pd", "тревога напрямую", exit=(0.5, 1), entry=(0.5, 0))
p.edge("apps", "hb", "Watchdog: ping 60 с", EDGE_ORTHO, points=[(55, 270), (55, 75), (1700, 75)], exit=(0, 0.5), entry=(0.5, 0))
p.text("<b>Сценарии отказа</b>", 40, 670, 600, 24, NOTE + "fontSize=14;")
SC = [
    ("Упал под сервиса", "Kubernetes перезапускает под; вторая реплика держит нагрузку (минимум ×2, PDB); NATS передоставляет неподтверждённые сообщения", "0 потерь, до 1 мин"),
    ("Отказ узла приложений", "реплики разнесены по узлам (anti-affinity); ingress-контроллер ×2 на разных узлах; Watchdog и CMDB Discovery ×2 выбирают нового лидера", "без простоя"),
    ("Отказ узла NATS", "кворум 2 из 3 сохраняется, потоки R3 продолжают работу; узел догоняет после возврата", "0 потерь, без простоя"),
    ("Отказ PostgreSQL primary", "оператор выполняет автоматическое повышение реплики до ведущей (promote); сервисы переподключаются", "RPO 0, ≤ 30 с"),
    ("Отказ синхронной реплики PostgreSQL", "primary продолжает работу в асинхронном режиме (strict-режим выключен), Watchdog поднимает тревогу; реплика догоняет после возврата", "без простоя"),
    ("Недоступен PagerDuty", "алерты ждут в outbox с повторами; если тревогу error или critical PagerDuty не принял за 2 мин с открытия (отсчёт идёт и при открытом circuit breaker), "
     "Fallback Notifier оповещает дежурных по резервным каналам; после восстановления — trigger с тем же dedup_key", "оповещение ≤ 3 мин, без дублей"),
    ("Недоступен источник", "курсор не сдвигается, после восстановления коннектор догружает пропущенное; алерт «источник молчит»", "0 потерь"),
    ("Недоступно хранилище секретов", "OpenBao работает при 2 из 3 узлов; сервисы продолжают работу на полученных секретах до конца аренды; "
     "распечатывание: auto-unseal или Шамир 3 из 5", "без простоя"),
    ("Недоступна Grafana", "дашборд инцидентов работает, ссылка в Grafana показывает ошибку, оповещение не затронуто", "без влияния на оповещение"),
    ("Недоступен каталог (AD)", "действует кеш групп последней синхронизации; если недоступен и IdP — вход break-glass (OpenBao, MFA, аудит каждого входа)", "доступ сохраняется"),
]
if TGT:
    SC.append(("Отказ узла ClickHouse", "реплики ReplicatedMergeTree и Keeper 2 из 3; при полной недоступности Backfill догружает историю из NATS и горячих данных PostgreSQL; ежедневный BACKUP в S3", "без потерь истории"))
SC.append(("Потеря основной площадки",
           "развёртывание в резерве через GitOps, PostgreSQL (PITR) и OpenBao из S3; pull-источники перечитываются от курсоров; "
           "push-события, принятые в JetStream, но ещё не записанные в PostgreSQL, могут быть потеряны", "RPO ≤ 1 мин, RTO ≤ 1 ч") if MVP else
          ("Потеря основной площадки",
           "повышение реплики PostgreSQL до ведущей (promote), масштабирование сервисов резерва, переключение DNS и адресов webhook; "
           "секреты из снапшота (RPO ≤ 15 мин); история ClickHouse из BACKUP в S3", "RPO ≤ 1 мин, RTO ≤ 15 мин"))
SC.append(("Umbrella не работает целиком", "внешний heartbeat не получает ping и поднимает тревогу в PagerDuty сам", "дежурный узнаёт ≤ 3 мин"))
for i, (f, r, res) in enumerate(SC):
    y = 710 + i * 64
    p.node(f"sf{i}", f"<b>{escape(f)}</b>", 40, y, 280, 52, ST_CRIT)
    p.node(f"sr{i}", escape(r), 360, y, 1060, 52, TASK + LEFT)
    p.node(f"so{i}", f"<b>{escape(res)}</b>", 1460, y, 340, 52, ST_OK)
    p.edge(f"sf{i}", f"sr{i}")
    p.edge(f"sr{i}", f"so{i}")
p.legend(labels={ST_CRIT: "Отказ", TASK: "Реакция системы", ST_OK: "Результат: потери и время", NODE_BOUND: "Площадка",
                 DB: "Хранилище данных или секретов",
                 REPL: "Резервная площадка: восстановление из бэкапа" if MVP else "Резервная площадка (тёплый резерв)"})

# ---------------------------------------------------------------------
# Интеграционная схема
# ---------------------------------------------------------------------
p = Page("Интеграционная схема", "int"); pages.append(p)
p.text("<b>Интеграционная схема: все внешние интеграции, протоколы, направления и гарантии</b>", 40, 20, 1500, 30, TITLE)
IEXT = EXT.replace("fontSize=12;", "fontSize=11;")
p.node("bnd", "<b>Umbrella</b>", 470, 70, 760, 1050 if MVP else 1140, BOUND)
p.node("conn", c4("Connector Runtime", "Container", "pull, подписки, приём syslog, sync-back"), 500, 90, 230, 520, CONT)
p.node("ingest", c4("Ingest Gateway", "Container", "приём push"), 500, 680, 230, 110, CONT)
p.node("cmdb", c4("CMDB Discovery", "Container", "инвентарь"), 760, 590, 180, 80, CONT)
p.node("pdg", c4("PagerDuty Gateway", "Container", "Events API, Webhooks, REST"), 970, 90, 230, 240, CONT)
p.node("fbn", c4("Fallback Notifier", "Container", ""), 970, 360, 230, 70, CONT)
p.node("cb", c4("Context Builder", "Container", ""), 970, 460, 230, 70, CONT)
p.node("api", c4("Core API", "Container", "вход пользователей, каталог, аудит"), 970, 560, 230, 250, CONT)
p.node("wd", c4("Watchdog", "Container", ""), 970, 840, 230, 60, CONT)
p.node("pg", c4("PostgreSQL", "Container", ""), 760, 930, 180, 70, DB)
p.node("bao", c4("Хранилище секретов", "Container: OpenBao ×3, Raft", ""), 760, 1020, 180, 80, DB)
if TGT:
    p.node("ch", c4("ClickHouse", "Container: 3 узла", ""), 760, 1120, 180, 70, DB)
    p.node("itsm", c4("ITSM Sync", "Container", ""), 500, 830, 230, 70, CONT)
LEFTI = [
    ("i1", "API систем мониторинга", "П-01 · REST, GraphQL, JSON-RPC, SOAP; OAuth2, токен, mTLS; ack и silence (П-05)", 90, 70),
    ("i2", "БД систем мониторинга", "П-01 · SQL, только чтение; UPDATE флага — если разрешено", 180, 70),
    ("i3", "Брокеры сообщений", "П-01 · Kafka, AMQP, NATS; ack сообщения", 270, 70),
    ("i4", "Почтовые ящики", "П-01 · IMAPS 993", 360, 70),
    ("i6", "Устройства и syslog", "П-02 · syslog TLS 6514 → приёмник Connector Runtime за балансировщиком", 450, 70),
    ("i5", "Облачные провайдеры", "П-01 · API облака (IAM-роль); П-06 · инвентарь и теги; П-02 · события push", 550, 100),
    ("i7", "Webhooks источников", "П-02 · HTTPS push, HMAC или токен", 700, 70),
]
if TGT:
    LEFTI.append(("i8", "Корпоративная CMDB и ITSM", "П-11 · REST 443; КЕ, сервисы, инциденты; двусторонний обмен", 830, 70))
for k, n, d, y, h in LEFTI:
    p.node(k, c4(n, "", d), 40, y, 320, h, IEXT)


def ry(key, y):
    x0, y0, w, h = p.geo[key]
    return round((y - y0) / h, 4)


for k in ("i1", "i2", "i3", "i4"):
    yc = p.geo[k][1] + 35
    p.edge("conn", k, "", exit=(0, ry("conn", yc)), entry=(1, 0.5))
p.edge("i6", "conn", "", EDGE_DASH, exit=(1, 0.5), entry=(0, ry("conn", 485)))
p.edge("conn", "i5", "", exit=(0, ry("conn", 575)), entry=(1, ry("i5", 575)))
p.edge("cmdb", "i5", "", exit=(0, 0.375), entry=(1, ry("i5", 620)))
p.edge("i5", "ingest", "", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(410, 645), (410, 700)], exit=(1, 0.95), entry=(0, ry("ingest", 700)))
p.edge("i7", "ingest", "", EDGE_DASH, exit=(1, 0.7), entry=(0, ry("ingest", 749)))
if TGT:
    p.edge("itsm", "i8", "", exit=(0, 0.3), entry=(1, 0.3))
    p.edge("i8", "itsm", "", EDGE_DASH, exit=(1, 0.75), entry=(0, 0.75))
RIGHTI = [
    ("o1", "PagerDuty Events API v2", "П-04 · исходящий; trigger, acknowledge, resolve; dedup_key, links на Grafana; outbox, повторы", 90, 70),
    ("o2", "PagerDuty Webhooks v3", "П-05 · входящий; ack, resolve, reassign; проверка подписи", 175, 70),
    ("o3", "PagerDuty REST API", "П-09 · исходящий; дежурства, сверка статусов" + ("" if MVP else "; токен только на чтение"), 260, 70),
    ("o8", "Резервные каналы оповещения", ("П-08 · SMTP 587 через корпоративный релей, HTTPS webhook; только если тревогу error или critical PagerDuty не принял за 2 мин" if MVP else
      "П-08 · SMS- и голосовой шлюз, мессенджеры, почта, webhook, запасная платформа; входящие ответы с ack"), 355, 85),
    ("o10", "Grafana HTTP API", "П-10 · HTTPS 443; дашборды, аннотации, команды и права папок", 460, 70),
    ("o4", "IdP", "П-07 · OIDC 443 — вход; SCIM 2.0 443 — push групп в Core API", 560, 70),
    ("o11", "Каталог (AD)", "П-07 · LDAPS 636; группы UMB-* раз в 15 мин", 650, 70),
    ("o12", "SIEM", "syslog TLS 6514; журнал аудита", 740, 70),
    ("o6", "Внешний heartbeat", "исходящий ping 60 с, HTTPS 443", 840, 60),
    ("o7", "S3 (другая площадка или регион)", "HTTPS 443; WAL и бэкапы PostgreSQL, снапшоты OpenBao" + ("" if MVP else "; BACKUP ClickHouse"), 930, 170 if MVP else 260),
]
for k, n, d, y, h in RIGHTI:
    p.node(k, c4(n, "", d), 1300, y, 320, h, IEXT)
p.edge("pdg", "o1", "", exit=(1, ry("pdg", 125)), entry=(0, 0.5))
p.edge("o2", "pdg", "", EDGE_DASH, exit=(0, 0.5), entry=(1, ry("pdg", 210)))
p.edge("pdg", "o3", "", exit=(1, ry("pdg", 295)), entry=(0, 0.5))
p.edge("fbn", "o8", "", exit=(1, ry("fbn", 385)), entry=(0, ry("o8", 385)))
if TGT:
    p.edge("o8", "fbn", "", EDGE_DASH, exit=(0, ry("o8", 415)), entry=(1, ry("fbn", 415)))
p.edge("cb", "o10", "", exit=(1, 0.5), entry=(0, 0.5))
p.edge("api", "o4", "", exit=(1, ry("api", 580)), entry=(0, ry("o4", 580)))
p.edge("o4", "api", "", EDGE_DASH, exit=(0, ry("o4", 615)), entry=(1, ry("api", 615)))
p.edge("api", "o11", "", exit=(1, ry("api", 685)), entry=(0, 0.5))
p.edge("api", "o12", "", exit=(1, ry("api", 775)), entry=(0, 0.5))
p.edge("wd", "o6", "", exit=(1, 0.5), entry=(0, 0.5))
p.edge("pg", "o7", "", exit=(1, 0.5), entry=(0, ry("o7", 965)))
p.edge("bao", "o7", "", exit=(1, 0.5), entry=(0, ry("o7", 1060)))
if TGT:
    p.edge("ch", "o7", "", exit=(1, 0.5), entry=(0, ry("o7", 1155)))
    p.node("drs", c4("Резервная площадка", "Umbrella, тёплый резерв",
                     "П-12 · реплика забирает WAL по 5432; зеркало NATS 7422; снапшоты OpenBao через S3 (443)"), 700, 1260, 360, 90, REPL)
    p.edge("drs", "bnd", "", EDGE_DASH, exit=(0.5, 0), entry=(round((880 - 470) / 760, 4), 1))
YN = (1120 if MVP else 1210) + (30 if MVP else 160)
p.text("Гарантии: источники — at-least-once, ack после записи в шину, повтор отсекает inbox; PagerDuty — at-least-once с dedup_key, "
       "поэтому повтор не создаёт второй инцидент; webhooks — идемпотентно по id события. Учётные данные всех интеграций берутся только из хранилища секретов (OpenBao). "
       "Исходящие HTTPS-вызовы во внешние сети идут через egress-прокси. Интеграция с новым источником собирается в конструкторе без кода.",
       40, YN, 1580, 40)
p.legend(labels={DB: "Хранилище данных или секретов"} | ({} if MVP else {REPL: "Резервная площадка (тёплый резерв)"}),
         extra=[(IEXT, "Внешняя интеграция: поток, протокол, гарантия")],
         edges=[(EDGE, "Umbrella инициирует соединение"), (EDGE_DASH, "Внешняя система инициирует (push)")])

# ---------------------------------------------------------------------
# Размещение по узлам
# ---------------------------------------------------------------------
p = Page("Размещение по узлам", "nod"); pages.append(p)
p.text("<b>Размещение по узлам: промышленный контур на 5000 событий в минуту</b>" if MVP else "<b>Размещение по узлам: основная площадка на 20 000 событий в минуту</b>", 40, 20, 1400, 30, TITLE)
POD = CONT + "fontSize=10;"
POD_RES = CONT + "fontSize=10;dashed=1;"
p.node("dmz", "<b>DMZ</b> [2 VM, 2 vCPU / 4 ГБ]", 40, 70, 300, 330, NODE_BOUND)
p.node("rp1", "<b>proxy-1</b><br><font style='font-size:10px'>reverse proxy + WAF, egress-прокси</font>", 60, 110, 260, 110, EXT)
p.node("rp2", "<b>proxy-2</b><br><font style='font-size:10px'>reverse proxy + WAF, egress-прокси; VRRP</font>", 60, 250, 260, 110, EXT)
if MVP:
    APPN = [
        ("w1", "worker-1", ["ingress-controller-0", "web-0", "core-api-0", "ingest-0", "connector-0", "normalizer-0", "rule-0", "alert-0", "pd-gateway-0", "fallback-0"]),
        ("w2", "worker-2", ["ingress-controller-1", "web-1", "core-api-1", "ingest-1", "connector-1", "normalizer-1", "cmdb-0 (лидер)", "watchdog-0 (лидер)", "context-builder-0", "grafana-0 *"]),
        ("w3", "worker-3", ["rule-1", "alert-1", "pd-gateway-1", "fallback-1", "cmdb-1 (резерв)", "watchdog-1 (резерв)", "context-builder-1", "grafana-1 *", "резерв на отказ узла"]),
    ]
    WSPEC, DSPEC = "8 vCPU / 16 ГБ", "4 vCPU / 16 ГБ, SSD 800 ГБ"
else:
    APPN = [
        ("w1", "worker-1", ["ingress-controller-0", "web-0", "core-api-0", "ingest-0", "connector-0", "normalizer-0", "rule-0", "alert-0", "grafana-0 *"]),
        ("w2", "worker-2", ["ingress-controller-1", "web-1", "core-api-1", "ingest-1", "connector-1", "normalizer-1", "rule-1", "alert-1"]),
        ("w3", "worker-3", ["connector-2", "normalizer-2", "pd-gateway-0", "fallback-0", "history-0", "correlation-0", "cmdb-0 (лидер)", "watchdog-0 (лидер)", "core-api-2"]),
        ("w4", "worker-4", ["connector-3", "normalizer-3", "pd-gateway-1", "fallback-1", "history-1", "correlation-1", "itsm-sync-0 (лидер)", "context-builder-0", "ingest-2"]),
        ("w5", "worker-5", ["cmdb-1 (резерв)", "watchdog-1 (резерв)", "itsm-sync-1 (резерв)", "context-builder-1", "alert-2", "grafana-1 *", "резерв на отказ узла"]),
    ]
    WSPEC, DSPEC = "12 vCPU / 24 ГБ", "8 vCPU / 32 ГБ, SSD 2 ТБ"
NW = len(APPN)
DX0 = 400 + NW * 190 + 20
WH = 60 + max(len(x[2]) for x in APPN) * 36 + 10
K8B = (110 + WH + 70) if MVP else 800
p.node("k8s", "<b>Kubernetes-кластер</b>", 380, 70, DX0 + 3 * 190 - 380, K8B - 70, NODE_BOUND)
for i, (k, n, pods) in enumerate(APPN):
    x = 400 + i * 190
    p.node(k, f"<b>{n}</b><br><font style='font-size:10px'>{WSPEC}</font>", x, 110, 175, WH, NODE_BOUND)
    for j, pod in enumerate(pods):
        p.node(f"{k}p{j}", pod, x + 12, 160 + j * 36, 150, 28, POD_RES if pod.startswith("резерв") else POD)
p.node("cp", "<b>Control plane</b> ×3 · 2 vCPU / 4 ГБ" + ("" if MVP else " на каждой площадке") + " (или управляемый Kubernetes)", 400, 110 + WH + 20, NW * 190 - 15, 34,
       NODE_BOUND + "verticalAlign=middle;spacingTop=0;")
DATAN = [("d1", "data-1", [("nats-0", BUS), ("PostgreSQL primary", DB), ("openbao-0", DB)]),
         ("d2", "data-2", [("nats-1", BUS), ("PostgreSQL синхронная реплика", DB), ("openbao-1", DB)]),
         ("d3", "data-3", [("nats-2", BUS), ("PostgreSQL: бэкап-агент", DB + "dashed=1;"), ("openbao-2", DB)])]
for i, (k, n, pods) in enumerate(DATAN):
    x = DX0 + i * 190
    p.node(k, f"<b>{n}</b><br><font style='font-size:10px'>{DSPEC}</font>", x, 110, 175, 380, NODE_BOUND)
    for j, (pod, st) in enumerate(pods):
        p.node(f"{k}p{j}", pod, x + 12, 170 + j * 100, 150, 70, st + "fontSize=10;")
p.edge("d1p1", "d2p1", "")
p.edge("d1p0", "d2p0", "")
p.edge("d2p0", "d3p0", "")
p.edge("d1p2", "d2p2", "")
p.edge("d2p2", "d3p2", "")
p.node("s3", c4("S3-хранилище", "другая площадка или регион", "WAL-архив, бэкапы, снапшоты OpenBao")
       + ("" if MVP else "<br><font style='font-size:11px'>BACKUP ClickHouse</font>"),
       DX0 + 3 * 190 + 60, 300, 260, 110, EXT)
p.edge("d1", "s3", "WAL, бэкапы, снапшоты", EDGE_ORTHO, points=[(DX0 + 60, 520), (DX0 + 3 * 190 + 190, 520)], exit=(round(60 / 175, 4), 1), entry=(0.5, 1))
p.edge("rp1", "w1p0", "HTTPS<br>443", exit=(1, round((174 - 110) / 110, 4)), entry=(0, 0.5), lpos=-0.13)
if TGT:
    for i in range(3):
        x = 400 + i * 250
        p.node(f"ch{i}", f"<b>clickhouse-{i + 1}</b><br><font style='font-size:10px'>8 vCPU / 32 ГБ, SSD 4 ТБ</font>", x, 625, 230, 150, NODE_BOUND)
        p.node(f"ch{i}p0", f"clickhouse-{i}", x + 15, 670, 200, 40, DB + "fontSize=10;")
        p.node(f"ch{i}p1", f"keeper-{i}", x + 15, 722, 200, 36, BUS + "fontSize=10;")
    p.edge("ch0p0", "ch1p0", "")
    p.edge("ch1p0", "ch2p0", "")
    # резервная площадка
    p.node("drs", "<b>Резервная площадка</b> [тёплый резерв]", 380, 850, DX0 + 3 * 190 - 380, 150, NODE_BOUND + "dashed=1;")
    p.node("drz", c4("DMZ: 2 VM", "2 vCPU / 4 ГБ", "reverse proxy + WAF, egress-прокси"), 400, 890, 300, 90, REPL)
    p.node("dra", c4("3 app-узла", "12 vCPU / 24 ГБ; control plane ×3", "сервисы Umbrella по 1 реплике, масштабируются при переключении"), 730, 890, 440, 90, REPL)
    p.node("drd", c4("3 data-узла", "8 vCPU / 32 ГБ, SSD 2 ТБ", "асинхронная реплика PostgreSQL, зеркало NATS, OpenBao из снапшотов"), DX0 - 10, 890, 3 * 190 - 10, 90, REPL)
    p.edge("drd", "d1", "WAL 5432,<br>NATS 7422", EDGE_DASH, exit=(round(40 / (3 * 190 - 10), 4), 0), entry=(round(30 / 175, 4), 1))
p.text("Связи между узлами данных: NATS — потоки R3, PostgreSQL — синхронная репликация (без strict-режима), OpenBao — Raft. "
       "* Grafana разворачивается в кластере, только если нет корпоративной. Реплики одного сервиса разносятся по разным узлам (pod anti-affinity), "
       "ingress-controller ×2 на разных узлах; Watchdog и CMDB Discovery ×2 с выбором лидера. Запросы ресурсов всех подов помещаются на 2 app-узла при отказе третьего; "
       "узлы данных помечены taint и принимают только NATS, PostgreSQL и хранилище секретов (OpenBao). Нагрузка 85 событий в секунду (нагрузочный тест — 10 000 в минуту). "
       "Пилот: 3 VM и Docker Compose, по одному экземпляру сервиса.",
       40, K8B + 30, DX0 + 3 * 190 + 280, 60) if MVP else \
    p.text("Связи между узлами данных: NATS — потоки R3, PostgreSQL — синхронная репликация (без strict-режима), OpenBao — Raft. "
       "* Grafana разворачивается в кластере, только если нет корпоративной. Реплики одного сервиса разносятся по разным узлам, ingress-controller ×2 на разных узлах, "
           "Core API, Ingest Gateway и Alert Engine ×3, Connector Runtime и Normalizer ×4, остальные ×2; ITSM Sync, Watchdog и CMDB Discovery — с выбором лидера; worker-5 держит запас на отказ узла. Узлы данных и ClickHouse помечены taint; ClickHouse — ReplicatedMergeTree и Keeper ×3. "
           "Нагрузка 330 событий в секунду (нагрузочный тест — 40 000 в минуту). Резервная площадка повторяет схему в минимальном составе; "
           "ClickHouse там не резервируется — история восстанавливается из BACKUP в S3.",
           40, 1030, DX0 + 3 * 190 + 280, 60)
p.legend(labels={CONT: "Под сервиса Umbrella", NODE_BOUND: "Узел, кластер или площадка", EXT: "VM вне кластера или внешнее хранилище",
                 BUS: "Под NATS JetStream или Keeper", DB: "Под СУБД или хранилища секретов (OpenBao)"} | ({} if MVP else {REPL: "Резервная площадка (тёплый резерв)"}),
         extra=[(POD_RES, "Резервная ёмкость узла"), (DB + "dashed=1;", "Вспомогательный под без данных (бэкап-агент)")])

# ---------------------------------------------------------------------
# Модель угроз STRIDE
# ---------------------------------------------------------------------
p = Page("Модель угроз STRIDE", "thr"); pages.append(p)
TMAX = "У-22" if MVP else "У-26"
p.text(f"<b>Модель угроз STRIDE: потоки данных, границы доверия и угрозы У-01…{TMAX}</b>", 40, 20, 1500, 30, TITLE)
AX, BX, CX_, DZX = 630, 880, 1100, 1340         # колонки процессов и зона данных
RY = {1: 110, 2: 230, 3: 350, 4: 470, 5: 590, 6: 710}
PW, PH = 170, 70
IY = 700 if MVP else 820                         # граница внутренней сети
p.node("tb_net", "Интернет", 40, 70, 270, 600, TB)
p.node("tb_dmz", "DMZ", 330, 70, 200, 600, TB)
p.node("tb_app", "Зона приложений", 560, 70, 750, IY - 90, TB)
p.node("tb_data", "Зона данных", 1330, 70, 270, 600, TB)
p.node("tb_corp", "Внутренняя сеть", 560, IY, 1040, 160, TB)
if TGT:
    p.node("tb_dr", "Резервная площадка", 1680, 70, 230, 600, TB)
# внешние сущности: Интернет
p.node("pd", "PagerDuty", 60, 110, 180, 70, ENTITY)
p.node("cloud", "Облачные провайдеры", 60, 230, 180, 70, ENTITY)
p.node("fbx", "Резервные каналы" + ("" if MVP else "<br>(SMS-шлюз, мессенджеры,<br>почта)"), 60, 375, 180, 80, ENTITY)
p.node("hb", "Внешний heartbeat", 60, 520, 180, 60, ENTITY)
# DMZ
p.node("waf", "Reverse proxy + WAF", 350, 110, 160, 90, PROC)
p.node("egr", "Egress-прокси", 350, 330, 160, 130, PROC)
# процессы
P_ = [("ing", "Ingest Gateway", AX, 1), ("pdg", "PagerDuty Gateway", AX, 2), ("fbn", "Fallback Notifier", AX, 3), ("cr", "Connector Runtime", AX, 4),
      ("nm", "Normalizer, Rule Engine", BX, 1), ("ae", "Alert Engine, CMDB Discovery", BX, 2), ("api", "Core API, Web", BX, 4),
      ("cb", "Context Builder", CX_, 5)]
if TGT:
    P_ += [("corr", "Correlation Engine", BX, 3), ("hist", "History Service", CX_, 3), ("itsm", "ITSM Sync", CX_, 6)]
for k, n, x, r in P_:
    p.node(k, n, x, RY[r], PW, PH, PROC)
# хранилища
S_ = [("nats", "NATS JetStream", 1), ("pg", "PostgreSQL: конфигурация, CMDB, тревоги", 2), ("pga", "PostgreSQL: аудит", 4), ("sec", "Хранилище секретов (OpenBao)", 5)]
if TGT:
    S_.append(("ch", "ClickHouse: история", 3))
for k, n, r in S_:
    p.node(k, n, 1360, RY[r] + 10, 220, 50, STORE)
# внутренняя сеть
YB = IY + 40
p.node("srcs", "Системы мониторинга,<br>БД, брокеры", 610, YB, 190, 60, ENTITY)
p.node("eng", "Инженеры", 830, YB, 110, 60, ENTITY)
p.node("idp", "IdP и каталог (AD)", 960, YB, 130, 60, ENTITY)
if TGT:
    p.node("itsmx", "CMDB и ITSM", 1110, YB, 130, 60, ENTITY)
p.node("grf", "Grafana", 1260, YB, 120, 60, ENTITY)
p.node("tsdb", "Хранилища метрик,<br>логов и трейсов", 1420, YB, 160, 60, ENTITY + "dashed=1;")


def fy(key, y):
    x0, y0, w, h = p.geo[key]
    return round((y - y0) / h, 4)


def fx(key, x):
    x0, y0, w, h = p.geo[key]
    return round((x - x0) / w, 4)


SIDE = SMALL + "spacing=0;verticalAlign=middle;"
def fl(a, b, lab="", st=EDGE, pts=None, ex=None, en=None):
    p.edge(a, b, lab, st + ("edgeStyle=orthogonalEdgeStyle;" if pts else ""), points=pts, exit=ex, entry=en)


# Интернет ↔ DMZ
p.edge("pd", "waf", "webhook", EDGE_DASH, exit=(1, fy("pd", 150)), entry=(0, fy("waf", 150)), lpos=-0.5)
fl("cloud", "waf", "", EDGE_DASH, pts=[(285, 250), (285, 175)], ex=(1, fy("cloud", 250)), en=(0, fy("waf", 175)))
fl("egr", "pd", "", pts=[(270, 350), (270, 165)], ex=(0, fy("egr", 350)), en=(1, fy("pd", 165)))
fl("egr", "cloud", "", pts=[(255, 375), (255, 285)], ex=(0, fy("egr", 375)), en=(1, fy("cloud", 285)))
fl("egr", "fbx", "", ex=(0, 0.5), en=(1, fy("fbx", 395)))
fl("egr", "hb", "ping", pts=[(390, 550)], ex=(fx("egr", 390), 1), en=(1, fy("hb", 550)))
if TGT:
    fl("fbx", "waf", "ответы", EDGE_DASH, pts=[(50, 400), (50, 95), (390, 95)], ex=(0, fy("fbx", 400)), en=(fx("waf", 390), 0))
# DMZ ↔ зона приложений
fl("waf", "ing", "", ex=(1, fy("waf", 150)), en=(0, fy("ing", 150)))
fl("waf", "pdg", "", pts=[(580, 165), (580, 265)], ex=(1, fy("waf", 165)), en=(0, fy("pdg", 265)))
if TGT:
    fl("waf", "fbn", "", EDGE + "entryPerimeter=0;", pts=[(565, 180), (565, 372)], ex=(1, fy("waf", 180)), en=(0.035, fy("fbn", 372)))
fl("pdg", "egr", "Events API", EDGE + "entryPerimeter=0;", pts=[(595, 290), (595, 362)], ex=(0, fy("pdg", 290)), en=(0.93, fy("egr", 362)))
fl("fbn", "egr", "", EDGE + "exitPerimeter=0;entryPerimeter=0;", ex=(0, fy("fbn", 395)), en=(1, fy("egr", 395)))
p.edge("cr", "egr", "API облака", EDGE + "edgeStyle=orthogonalEdgeStyle;", points=[(470, 505)], exit=(0, fy("cr", 505)), entry=(fx("egr", 470), 1), lpos=-0.6)
# внутри зоны приложений
fl("ing", "nm", "через шину", ex=(1, 0.5), en=(0, 0.5))
fl("cr", "nm", "", pts=[(850, 495), (850, 160)], ex=(1, fy("cr", 495)), en=(0, fy("nm", 160)))
fl("nm", "ae", "", ex=(0.5, 1), en=(0.5, 0))
fl("ae", "pdg", "", ex=(0, fy("ae", 255)), en=(1, fy("pdg", 255)))
fl("pdg", "fbn", "pd.delivery", ex=(0.5, 1), en=(0.5, 0))
fl("nm", "nats", "", ex=(1, 0.5), en=(0, 0.5))
fl("ae", "pg", "", ex=(1, 0.5), en=(0, 0.5))
fl("api", "pga", "аудит", ex=(1, fy("api", 495)), en=(0, fy("pga", 495)))
fl("api", "cb", "context.request", pts=[(1185, 520)], ex=(1, fy("api", 520)), en=(0.5, 0))
fl("cb", "sec", "", EDGE_DASH, ex=(1, fy("cb", 615)), en=(0, fy("sec", 615)))
p.text("HTTP API", 1305, IY - 18, 56, 16, SIDE)
fl("cb", "grf", "", pts=[(1300, 645), (1300, YB - 10), (1320, YB - 10)], ex=(1, fy("cb", 645)), en=(fx("grf", 1320), 0))
if TGT:
    fl("ae", "corr", "", ex=(0.5, 1), en=(0.5, 0))
    fl("corr", "pdg", "", pts=[(910, 335), (790, 335)], ex=(fx("corr", 910), 0), en=(fx("pdg", 790), 1))
    fl("hist", "ch", "", ex=(1, 0.5), en=(0, 0.5))
    fl("api", "hist", "история", pts=[(1030, 450), (1185, 450)], ex=(fx("api", 1030), 0), en=(0.5, 1))
    fl("itsm", "itsmx", "", ex=(0.5, 1), en=(fx("itsmx", CX_ + PW / 2), 0))
    p.text("REST", 1191, IY - 18, 40, 16, SMALL + "spacing=0;verticalAlign=middle;")
# зона приложений ↔ внутренняя сеть
fl("cr", "srcs", "API, SQL, ack", ex=(0.5, 1), en=(fx("srcs", AX + PW / 2), 0))
fl("eng", "api", "", pts=[(925, YB - 30)], ex=(fx("eng", 925), 0), en=(fx("api", 925), 1))
p.text("HTTPS", 870, IY - 60, 50, 16, SIDE + "align=right;")
fl("api", "idp", "", pts=[(985, YB - 30)], ex=(fx("api", 985), 1), en=(fx("idp", 985), 0))
fl("idp", "api", "", EDGE_DASH, pts=[(1070, 590), (1030, 590)], ex=(fx("idp", 1070), 0), en=(fx("api", 1030), 1))
p.text("OIDC,<br>LDAPS", 990, IY - 75, 70, 30, SIDE)
p.text("SCIM", 1030, IY - 110, 36, 16, SIDE)
fl("eng", "grf", "просмотр дашборда", pts=[(885, YB + 95), (1320, YB + 95)], ex=(fx("eng", 885), 1), en=(fx("grf", 1320), 1))
fl("grf", "tsdb", "", ex=(1, 0.5), en=(0, 0.5))
# межплощадочный канал
if TGT:
    p.node("drpg", "Реплика PostgreSQL", 1700, RY[2] + 10, 190, 50, STORE)
    p.node("drn", "Зеркало NATS", 1700, RY[1] + 10, 190, 50, STORE)
    p.node("drs", "Резервные сервисы и DMZ", 1700, RY[4] + 10, 190, 50, PROC)
    fl("drpg", "pg", "WAL 5432", EDGE_DASH, ex=(0, 0.5), en=(1, 0.5))
    fl("nats", "drn", "7422", ex=(1, 0.5), en=(0, 0.5))
# угрозы
TH_ = [("У-01", "ing", 1.0, -0.15), ("У-02", "pdg", 0.95, -0.25), ("У-03", "api", 0.98, -0.1), ("У-04", "sec", 0.85, -0.45),
       ("У-05", "cr", 0.95, -0.25), ("У-06", "cr", 0.1, -0.25), ("У-07", "nm", 0.98, -0.15), ("У-08", "nats", 0.85, -0.45),
       ("У-09", "egr", 0.05, -0.2), ("У-10", "pga", 0.85, -0.45), ("У-11", "pdg", 0.1, 1.05), ("У-12", "eng", 0.0, -0.4),
       ("У-13", "tb_app", 0.04, 0.95 if MVP else 0.96), ("У-14", "ae", 0.98, -0.15), ("У-15", "api", -0.15, 1.0),
       ("У-16", "fbn", 0.95, -0.25), ("У-17", "fbn", 0.95, 1.0), ("У-18", "fbx", 0.85, -0.35),
       ("У-19", "grf", 0.85, -0.4), ("У-20", "cb", 0.98, -0.15), ("У-21", "sec", 0.85, 1.05), ("У-22", "idp", 0.3, 1.05)]
if TGT:
    TH_ += [("У-23", "ch", 0.85, -0.45), ("У-24", "itsm", 0.98, -0.15), ("У-25", "drpg", 0.75, 1.05), ("У-26", "drs", 0.75, 1.05)]
for tid, owner, ox, oy in TH_:
    x, y, w, h = p.geo[owner]
    p.node("t" + tid[-2:], tid, x + w * ox - 10, y + h * oy if oy >= 0 else y + oy * 60, 44, 22, THREAT)
p.text("<b>STRIDE:</b> S — подмена, T — изменение, R — отказ от действий, I — раскрытие информации, D — отказ в обслуживании, "
       f"E — повышение привилегий. Угрозы У-01…{TMAX}, их меры и остаточный риск описаны в документе «Информационная безопасность». "
       "У-13 (цепочка поставки) касается всех контейнеров зоны приложений; У-21 — начальные секреты OpenBao (ключи распечатывания); "
       "У-22 — повышение прав через привязки групп каталога." + ("" if MVP else " У-25 и У-26 относятся к межплощадочному каналу и переключению на резерв (DNS, адреса webhook)."),
       40, IY + 180, 1870 if TGT else 1560, 40)
p.legend()

# ---------------------------------------------------------------------
# Легенды для существующих страниц
# ---------------------------------------------------------------------
BP_EDGES = [(EDGE, "Поток управления"), (EDGE_DASH, "Поток сообщений или данных")]
LEG = {
    "ctx": dict(edges=[(EDGE, "Связь, вызов или поток данных (направление по стрелке)"),
                       (EDGE + "startArrow=block;startFill=1;startSize=6;", "Двусторонний обмен"),
                       (EDGE_DASH, "Обратный поток: статусы из PagerDuty, сообщения резервных каналов дежурному")]),
    "cnt": dict(labels={NODE_BOUND: "Группа внешних систем (источники)"},
                edges=[(EDGE, "Вызов или поток данных (направление по стрелке)"),
                       (EDGE + "startArrow=block;startFill=1;startSize=6;", "Публикация и чтение потоков шины"),
                       (EDGE_DASH, "Обратный или асинхронный поток: статусы, инвентарь, сообщения дежурному")]),
    "crt": {}, "cmp": {}, "cdb": {},
    "dep": dict(labels={CONT: "Pod сервиса Umbrella", EXT: "Инфраструктура или внешний сервис", NODE_BOUND: "Кластер или namespace",
                        EXT_DASHED: "Pod, который разворачивается по условию (Grafana OSS)"},
                edges=[(EDGE, "Соединение: протокол и порт")] + ([] if MVP else [(EDGE_DASH, "Репликация на резервную площадку")])),
    "inf": dict(labels={CONT: "Сетевой компонент или сервисы Umbrella", NODE_BOUND: "Сетевая зона"},
                edges=[(EDGE, "Исходящее соединение: протокол и порт"),
                       (EDGE_DASH, "Входящие webhooks и события облаков; доставка на телефоны от PagerDuty и резервных каналов")]),
    "bld": dict(labels={L_BIZ: "Блок-триггер", L_SVC: "Блок получения данных", L_TEAM: "Авторизация, проверка, гарантии",
                        L_MON: "Управление потоком", L_CI: "Парсинг и преобразование", L_ALERT: "Выход и подтверждение источнику"},
                edges=[(EDGE, "Порядок выполнения блоков")]),
    "cld": dict(labels={L_MON: "Событие облака или история ID", L_TEAM: "Логика сопоставления", L_CI: "Устойчивая КЕ",
                        L_ALERT: "Ключ дедупликации", ST_WARN: "Запасной вариант: очередь «без КЕ»"}),
    "rsm": dict(labels={L_TEAM: "Команда, сервис PagerDuty, правило", L_ALERT: "Тревога", L_MON: "Объект мониторинга", NODE_BOUND: "Кластер КЕ «N из M»"}),
    "sig": dict(labels={L_SVC: "RED-сигнал или обработчик", L_CI: "USE-сигнал", NODE_BOUND: "Группа сигналов",
                        L_TEAM: "Шаблон", L_ALERT: "Событие", L_MON: "Готовые алерты источников"}),
    "dup": dict(labels={L_MON: "Сырое событие или payload PagerDuty", L_SVC: "Нормализованное событие",
                        L_TEAM: "Ключ дедупликации и шаблон", L_ALERT: "Тревога и правила схлопывания"}),
    "bp1": dict(edges=BP_EDGES, labels={TASK_HOT: "Задача с внешним эффектом: PagerDuty" + ("" if MVP else ", ITSM")}),
    "bp2": dict(edges=BP_EDGES, labels={TASK_HOT: "Откат версии коннектора"}),
    "bp3": dict(edges=BP_EDGES[:1], labels={TASK_HOT: "Второй ключ или отправка в PagerDuty"}),
    "bp4": dict(edges=BP_EDGES, labels={TASK_HOT: "Ревью инженера" if MVP else "Ревью инженера; обмен с корпоративной CMDB"}),
}
for pg in pages:
    if pg.pid in LEG:
        pg.legend(**LEG[pg.pid])

# переименование C4-5 и порядок страниц
for pg in pages:
    if pg.pid == "inf":
        pg.name = "C4-5 Сегментация сети"
        for i, c in enumerate(pg.cells):
            if c[0] == "v" and "Инфраструктура: сетевые зоны" in c[2]:
                pg.cells[i] = (c[0], c[1], "<b>C4 · Сегментация сети: зоны, порты и внешние доступы</b>", c[3], c[4], c[5])

