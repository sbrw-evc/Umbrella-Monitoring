# =====================================================================
# v3: потоки шины, последовательности, БП-5…БП-7, дорожная карта, макеты дашбордов, ролевая модель и порядок
# =====================================================================
HDR2 = BASE + "rounded=0;fillColor=#438DD5;strokeColor=#FFFFFF;fontColor=#FFFFFF;fontSize=11;fontStyle=1;"
CELL2 = BASE + "rounded=0;fillColor=#FFFFFF;strokeColor=#DDDDDD;fontSize=10;align=left;spacingLeft=6;"
CELLN = BASE + "rounded=0;fillColor=#DAE8FC;strokeColor=#DDDDDD;fontSize=11;fontStyle=1;align=left;spacingLeft=6;fontFamily=Courier New;"
CELLT = CELLN.replace("#DAE8FC", "#FFE6CC")
CELLR = CELLN.replace("#DAE8FC", "#E1D5E7")

# схемы последовательности: фрагмент alt, подписи сообщений
FRAME = ("rounded=0;fillColor=none;strokeColor=#9673A6;strokeWidth=1.5;html=1;verticalAlign=top;align=left;"
         "spacingLeft=110;spacingTop=2;fontSize=10;fontStyle=1;fontColor=#5A3D73;")
FRAME_TAG = BASE + "rounded=0;fillColor=#E1D5E7;strokeColor=#9673A6;fontSize=10;fontStyle=1;fontColor=#5A3D73;"
SEP = "endArrow=none;dashed=1;dashPattern=6 4;html=1;strokeColor=#9673A6;"
SEQL = ("text;html=1;fillColor=none;strokeColor=none;align=left;verticalAlign=bottom;fontSize=10;fontColor=#333333;"
        "whiteSpace=wrap;spacing=0;")
SELF = TASK + "fontSize=10;"
SEQ_EDGES = [(EDGE, "Вызов или сообщение"), (EDGE_DASH, "Ответ, ошибка или асинхронное сообщение"), (LLINE, "Линия жизни участника")]


def seq(p, parts, steps, y0=70, dx=210, x0=40, bw=170):
    """Схема последовательности. steps: ("m", a, b, label, style) | ("s", a, label) | ("alt", guard, a, b) |
    ("else", guard) | ("end",). Подпись сообщения ставится над стрелкой в первом промежутке от отправителя."""
    lx = {}
    for i, (k, n, st) in enumerate(parts):
        x = x0 + i * dx
        p.node(k, f"<b>{n}</b>", x, y0, bw, 50, st)
        lx[k] = x + bw / 2
    i_life = len(p.cells)  # линии жизни вставляются сюда, чтобы лежать под блоками и подписями
    y = y0 + 50 + 26
    fr = None
    n = 0
    for s in steps:
        if s[0] == "m":
            _, a, b, lab, st = s
            n += 1
            nl = lab.count("<br>") + 1
            y += 13 * nl + 4
            lw = dx - 18
            if lx[b] > lx[a]:
                p.text(f"{n}. {lab}", lx[a] + 8, y - 13 * nl - 3, lw, 13 * nl, SEQL)
            else:
                p.text(f"{n}. {lab}", lx[a] - 8 - lw, y - 13 * nl - 3, lw, 13 * nl, SEQL + "align=right;")
            p.line(lx[a], y, lx[b], y, "", st)
            y += 16
        elif s[0] == "s":
            _, a, lab = s
            n += 1
            nl = lab.count("<br>") + 1
            h = 14 * nl + 14
            p.node(f"self{n}", f"{n}. {lab}", lx[a] - 95, y, 190, h, SELF)
            y += h + 18
        elif s[0] == "alt":
            _, guard, a, b = s
            fr = [y, lx[a] - max(dx / 2 - 6, 105), lx[b] + max(dx / 2 - 6, 105), guard, []]
            y += 30
        elif s[0] == "else":
            fr[4].append((y, s[1]))
            y += 30
        elif s[0] == "end":
            y0f, xa, xb, guard, seps = fr
            p.node(f"fr{y0f}", escape(guard), xa, y0f, xb - xa, y + 4 - y0f, FRAME)
            p.node(f"frt{y0f}", "alt", xa, y0f, 40, 18, FRAME_TAG)
            for ys, g in seps:
                p.line(xa, ys + 4, xb, ys + 4, "", SEP)
                p.text(escape(g), xa + 108, ys + 6, dx - 22, 16, SEQL + "verticalAlign=top;fontStyle=1;fontColor=#5A3D73;")
            fr = None
            y += 24
    n_before = len(p.cells)
    for k in lx:
        p.line(lx[k], y0 + 50, lx[k], y, "", LLINE)
    life = p.cells[n_before:]
    del p.cells[n_before:]
    p.cells[i_life:i_life] = life
    return y


# ---------------------------------------------------------------------
# Потоки шины NATS JetStream
# ---------------------------------------------------------------------
p = Page("Потоки шины", "bus"); pages.append(p)
p.text("<b>Потоки (subjects) NATS JetStream: кто публикует, кто читает, сколько хранится</b>", 40, 20, 1500, 30, TITLE)
H = "" if MVP else ", History Service"
STREAMS = [
    ("ingest.in", "Ingest Gateway, Watchdog", "Connector Runtime", "24 ч", "Сырые push-события после проверки подписи; пробы Watchdog"),
    ("events.raw", "Connector Runtime, Rule Engine", "Normalizer", "24 ч", "Сырые события; источник подтверждён после записи сюда"),
    ("events.dlq", "Connector Runtime, Normalizer", "Core API (раздел «Ошибки разбора»)", "14 дней", "События, не прошедшие парсер или шаблон, с исходным телом"),
    ("events.norm", "Normalizer", "Alert Engine" + H, "24 ч", "Нормализованные события без поля raw: КЕ-кандидат, сигнал, ключ"),
    ("alerts.changed", "Alert Engine (Outbox Relay)",
     ("PagerDuty Gateway, " if MVP else "Correlation Engine, ") + "Fallback Notifier, Core API (WebSocket Hub), Watchdog, Context Builder"
     + ("" if MVP else ", History Service"), "7 дней", "Открытие, рост severity, ack, resolve тревоги"),
    ("alerts.commands", "Core API" + ("" if MVP else ", Fallback Notifier (Ack Handler)"), "Alert Engine", "7 дней", "Команды людей: ack, resolve, silence"),
    ("pd.delivery", "PagerDuty Gateway", "Fallback Notifier, Watchdog, Core API (WebSocket Hub)", "7 дней", "Принято PagerDuty или нет; состояние канала (норма, деградация)"),
    ("pd.inbound", "PagerDuty Gateway", "Alert Engine", "7 дней", "Статусы инцидентов PagerDuty: ack, resolve, reassign"),
    ("connector.query", "Rule Engine, Core API", "Connector Runtime", "request/reply", "Запрос метрик у источника и dry-run коннектора"),
    ("connector.command", "Alert Engine", "Connector Runtime", "7 дней", "Sync-back: ack и silence в источники"),
    ("cmdb.changed", "CMDB Discovery", "Alert Engine, Normalizer" + ("" if MVP else ", Correlation Engine, ITSM Sync"), "7 дней", "Изменения карты КЕ и связей"),
    ("notify.log", "Fallback Notifier", "Core API, Watchdog" + H, "30 дней", "Журнал резервных отправок и подтверждений"),
]
if TGT:
    STREAMS.append(("incidents.changed", "Correlation Engine", "Alert Engine, PagerDuty Gateway, Core API (WebSocket Hub), ITSM Sync, History Service, Context Builder",
                    "7 дней", "Инциденты: состав тревог и вероятная причина"))
STREAMS.append(("context.request", "Core API", "Context Builder", "request/reply",
                "Построить или обновить дашборд инцидента в Grafana, вернуть URL"))
COLS = [("Поток", 190), ("Публикует", 270), ("Читает", 470), ("Хранение", 110), ("Назначение", 470)]
x = 40
for j, (n, w) in enumerate(COLS):
    p.node(f"h{j}", n, x, 70, w, 36, HDR2)
    x += w
for i, row in enumerate(STREAMS):
    x = 40
    y = 106 + i * 44
    for j, (v, (n, w)) in enumerate(zip(row, COLS)):
        if j == 0:
            st = CELLR if row[0] == "context.request" else (
                CELLT if row[0] in ("pd.delivery", "pd.inbound", "notify.log", "alerts.commands") else CELLN)
        else:
            st = CELL2
        p.node(f"c{i}_{j}", escape(v), x, y, w, 44, st)
        x += w
ty = 106 + len(STREAMS) * 44 + 20
NS = 12 if MVP else 13
p.text(f"{NS} потоков JetStream и subject context.request (request/reply без хранения). Потоки ingest.in, events.raw и events.norm "
       "хранятся 24 ч, events.dlq 14 дней, notify.log 30 дней, остальные 7 дней (connector.query работает как request/reply); все с репликацией R3 и подтверждением записи (PubAck); "
       "потребители идемпотентны, повтор доставки безопасен. Outbox в PagerDuty — таблица PostgreSQL, а не поток: Alert Engine "
       "пишет изменение тревоги и строку outbox в одной транзакции, Outbox Relay публикует alerts.changed. "
       "Доступ к subjects ограничен учётной записью NATS каждого контейнера; учётные данные — только в OpenBao.", 40, ty, 1500, 64)
p.legend(auto=False, extra=[(CELLN, "Поток конвейера событий и тревог"), (CELLT, "Поток оповещения и команд"),
                            (CELLR, "Запрос контекста Grafana (request/reply)"), (HDR2, "Заголовок таблицы")], edges=[])

# ---------------------------------------------------------------------
# Резервное оповещение: последовательность
# ---------------------------------------------------------------------
p = Page("Резервное оповещение", "fbs"); pages.append(p)
p.text("<b>Резервное оповещение: PagerDuty не принимает алерт, дежурный получает его по резервному каналу</b>", 40, 20, 1600, 30, TITLE)
if MVP:
    PART = [("ae", "Alert Engine", LIFE), ("pdg", "PagerDuty Gateway", LIFE), ("pd", "PagerDuty", LIFE_EXT),
            ("fbn", "Fallback Notifier", LIFE), ("pg", "PostgreSQL: кеш дежурств", LIFE), ("ch", "Резервный канал", LIFE_EXT),
            ("duty", "Дежурный", LIFE_EXT), ("ack", "Core API", LIFE)]
    head = [("m", "ae", "pdg", "alerts.changed: тревога<br>#2051 critical открыта", EDGE),
            ("m", "pdg", "pd", "Events API v2: trigger,<br>dedup_key umb-2051", EDGE)]
    send = "письмо или webhook:<br>текст тревоги, ссылка<br>подтверждения, ссылка на<br>контекст в Grafana"
    esc = "резервные контакты сервиса"
    ackm = [("m", "duty", "ack", "одноразовая ссылка,<br>вход через IdP", EDGE),
            ("m", "ack", "ae", "alerts.commands:<br>ack тревоги #2051", EDGE)]
    guard = "[нет ack за 10 мин]"
else:
    PART = [("ae", "Alert Engine", LIFE), ("cor", "Correlation Engine", LIFE), ("pdg", "PagerDuty Gateway", LIFE),
            ("pd", "PagerDuty", LIFE_EXT), ("fbn", "Fallback Notifier", LIFE), ("pg", "PostgreSQL: кеш дежурств", LIFE),
            ("ch", "Резервный канал", LIFE_EXT), ("duty", "Дежурный", LIFE_EXT),
            ("ack", "Fallback Notifier · Ack Handler<br>(через reverse proxy)", LIFE)]
    head = [("m", "ae", "cor", "alerts.changed: тревога<br>#2051 critical открыта", EDGE),
            ("m", "cor", "pdg", "incidents.changed:<br>инцидент #731", EDGE),
            ("m", "pdg", "pd", "Events API v2: trigger,<br>dedup_key umb-inc-731", EDGE)]
    send = "SMS, звонок, мессенджер:<br>текст, кнопка «Принять»,<br>ссылка на контекст<br>в Grafana"
    esc = "следующий уровень escalation<br>policy; после последнего —<br>резервные контакты"
    ackm = [("m", "duty", "ack", "кнопка мессенджера<br>или ответ на SMS", EDGE),
            ("s", "ack", "подпись проверена"),
            ("m", "ack", "ae", "alerts.commands: ack<br>тревог инцидента #731", EDGE)]
    guard = "[нет ack за таймаут уровня]"
STEPS = head + [
    ("m", "pd", "pdg", "таймаут или 5xx", EDGE_DASH),
    ("s", "pdg", "повторы с backoff; 5 ошибок<br>подряд → circuit breaker открыт"),
    ("m", "pdg", "fbn", "pd.delivery: не принято,<br>канал деградировал", EDGE),
    ("s", "fbn", "2 мин от открытия тревоги<br>без приёма, severity error или<br>critical → резервное оповещение"),
    ("m", "fbn", "pg", "дежурные сервиса<br>на сейчас", EDGE),
    ("m", "pg", "fbn", "дежурный L1<br>и его контакты", EDGE_DASH),
    ("m", "fbn", "ch", send, EDGE),
    ("m", "ch", "duty", "уведомление", EDGE),
    ("alt", guard, "ae", "ack"),
    ("s", "fbn", esc),
    ("m", "fbn", "ch", "повторная отправка", EDGE),
    ("else", "[ack получен]"),
] + ackm + [
    ("m", "ae", "fbn", "alerts.changed: ack,<br>эскалация остановлена", EDGE_DASH),
    ("end",),
    ("s", "pdg", "пробный запрос прошёл,<br>circuit breaker закрыт"),
    ("m", "pdg", "pd", "PagerDuty снова<br>принимает: trigger<br>с тем же dedup_key", EDGE),
    ("m", "pd", "pdg", "202 Accepted", EDGE_DASH),
    ("m", "pdg", "pd", "acknowledge: ack<br>из резервного канала", EDGE),
    ("m", "pdg", "fbn", "pd.delivery:<br>канал в норме", EDGE),
    ("m", "fbn", "ch", "«PagerDuty снова<br>работает», итог", EDGE),
]
y = seq(p, PART, STEPS)
RULES = ("• включается для тревог error и critical, которые PagerDuty не принял за 2 мин от открытия; warning и info ждут PagerDuty<br>"
         "• открытый circuit breaker прекращает попытки отправки в PagerDuty, но не останавливает отсчёт 2 мин<br>"
         + ("• кеш дежурств обновляется из PagerDuty каждые 5 мин; если он пуст или старше 24 ч, сообщение уходит резервным контактам сервиса из РСМ<br>"
            if MVP else
            "• кеш дежурств хранит расписания на 7 дней вперёд; если он пуст или старше 24 ч, а также после последнего уровня — резервные контакты сервиса<br>")
         + ("• ссылка подтверждения одноразовая и подписанная, ведёт в Core API (вход через IdP)<br>" if MVP else
            "• ответы SMS и кнопки мессенджеров приходят через reverse proxy и WAF в Ack Handler (подпись проверяется); ссылки ведут в Core API<br>")
         + "• в каждом сообщении есть ссылка /go/incidents/{id}/grafana на контекст инцидента в Grafana<br>"
         "• при шторме (более 20 тревог за 5 мин на получателя) уходит одна сводка вместо отдельных сообщений<br>"
         "• дублей в PagerDuty нет: при восстановлении отправляется тот же dedup_key, подтверждённые тревоги — как acknowledge")
p.node("note", "<b>Правила</b><br><font style='font-size:11px'>" + RULES + "</font>",
       40, y + 16, len(PART) * 210 - 40, 150, L_TEAM + LEFT + "verticalAlign=top;spacingTop=6;")
p.legend(labels={L_TEAM: "Правила резервного оповещения"},
         extra=[(SELF, "Внутренняя обработка участника"), (FRAME, "Фрагмент alt: альтернативные ветки")], skip=(TASK,),
         edges=SEQ_EDGES)

# ---------------------------------------------------------------------
# БП-5 Резервное оповещение
# ---------------------------------------------------------------------
EVGATE = BASE + "rhombus;shape=rhombus;perimeter=rhombusPerimeter;fillColor=#FFFFFF;strokeColor=#D6B656;strokeWidth=2;fontSize=10;"
MSGEV = "ellipse;shape=ellipse;perimeter=ellipsePerimeter;html=1;fillColor=#DAE8FC;strokeColor=#6C8EBF;strokeWidth=1.5;double=1;"
p = Page("БП-5 Резервное оповещение", "bp5"); pages.append(p)
p.text("<b>БП-5 · Резервное оповещение: PagerDuty недоступен</b>", 40, 20, 1200, 30, TITLE)
LN = ["PagerDuty Gateway", "Fallback Notifier", "Core API", "Дежурный", "PagerDuty"] if MVP else \
     ["PagerDuty Gateway", "Fallback Notifier", "Дежурный", "PagerDuty"]
LH = [170, 330, 130, 140, 140] if MVP else [170, 330, 140, 140]
ys = lanes(p, LN, 40, 70, 2460, LH)
iF = 1
iD = 3 if MVP else 2
iP = iD + 1
q0 = mid(ys, 0, TH)
qa = ys[iF][0] + 40
qb = ys[iF][0] + 190
qd = mid(ys, iD, TH)
qp = mid(ys, iP, TH)
p.node("s", "", 100, mid(ys, 0, EW), EW, EW, START)
p.text("Тревога error<br>или critical открыта", 60, mid(ys, 0, EW) + 40, 120, 30, EVLABEL)
p.node("t1", "Отправить trigger,<br>повторять с backoff,<br>circuit breaker", 180, q0, TW, TH + 6, TASK_HOT)
p.node("g1", "Принят<br>за 2 мин<br>от открытия?", 380, qa - 13, GW + 30, GW + 30, GATE)
p.node("e0", "", 412, qb + 14, EW, EW, END)
p.text("обычный БП-1", 380, qb + 54, 100, 20, EVLABEL)
p.node("t3", "Найти дежурных<br>в кеше расписаний", 530, qa + 4, TW, TH, TASK)
p.node("g2", "Кеш<br>актуален?", 730, qa + 1, GW, GW, GATE)
p.node("t4", "Взять резервные<br>контакты сервиса<br>(кеш пуст или<br>старше 24 ч)", 690, qb, TW, TH + 10, TASK)
p.node("t5", "Отправить по резервным<br>каналам (" + ("почта, webhook" if MVP else "SMS, звонок,<br>мессенджер") +
       "),<br>ссылка на контекст в Grafana", 870, qa - 2, TW + 30, TH + 12, TASK_HOT)
p.node("gE", "", 1110, qa + 1, GW, GW, EVGATE)
p.text("что раньше?", 1100, qa - 18, 90, 18, EVLABEL)
p.node("gEi", "", 1133, qa + 24, 24, 24, MSGEV.replace("#DAE8FC", "#FFFFFF").replace("#6C8EBF", "#D6B656"))
p.node("tm", "", 1127, qb + 19, EW, EW, TIMER)
p.text("10 мин" if MVP else "таймаут уровня", 1095, qb + 59, 100, 20, EVLABEL)
p.node("t6", "Эскалировать:<br>" + ("резервные контакты<br>сервиса" if MVP else "следующий уровень;<br>после последнего —<br>резервные контакты"),
       1210, qb, TW, TH + 10, TASK)
p.node("ev", "", 1440, qa + 18, EW, EW, MSGEV)
p.text("ack получен<br>(alerts.changed)", 1400, qa - 18, 120, 30, EVLABEL)
p.node("t10", "Остановить эскалацию,<br>записать в notify.log", 1530, qa + 4, TW + 10, TH, TASK)
p.node("e3", "", 1740, qa + 18, EW, EW, END)
ACK_LANE = 2 if MVP else iF
p.node("t7", "Передать ack<br>в Alert Engine<br>(alerts.commands)" if MVP else "Ack Handler: проверить<br>подпись, передать ack<br>(alerts.commands)", 1383, (mid(ys, 2, TH) if MVP else qb), TW, TH + 6, TASK)
p.node("d1", "Получить<br>сообщение", 885, qd, TW, TH, TASK)
p.node("d2", "Подтвердить:<br>" + ("ссылка, вход<br>через IdP" if MVP else "кнопка, ответ<br>на SMS, ссылка"), 1383, qd, TW, TH, TASK)
p.node("rv", "", 1838, mid(ys, 0, EW), EW, EW, MSGEV)
p.text("канал отвечает", 1800, mid(ys, 0, EW) + 40, 110, 20, EVLABEL)
p.node("t8", "Отправить trigger<br>с тем же dedup_key,<br>затем acknowledge,<br>если ack уже есть", 1940, q0 - 5, TW + 10, TH + 10, TASK_HOT)
p.node("p1", "Инцидент создан;<br>подтверждённые —<br>сразу в ack", 1945, qp, TW, TH, TASK)
p.node("e1", "", 2160, mid(ys, iP, EW), EW, EW, END)
p.node("t9", "Разослать «PagerDuty<br>снова работает»", 2190, qa + 4, TW, TH, TASK)
p.node("e2", "", 2390, qa + 18, EW, EW, END)
p.edge("s", "t1")
p.edge("t1", "g1", "pd.delivery", EDGE_ORTHO, points=[(430, q0 + 35)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("g1", "e0", "да", exit=(0.5, 1), entry=(0.5, 0))
p.edge("g1", "t3", "нет")
p.edge("t3", "g2")
p.edge("g2", "t5", "да", exit=(1, 0.5), entry=(0, 0.5))
p.edge("g2", "t4", "нет", exit=(0.5, 1), entry=(0.5, 0))
p.edge("t4", "t5", "", EDGE_ORTHO, points=[(924, qb + 37)], exit=(1, 0.5), entry=(0.3, 1))
p.edge("t5", "gE", exit=(1, 0.5), entry=(0, 0.5))
p.edge("gE", "tm", exit=(0.5, 1), entry=(0.5, 0))
p.edge("tm", "t6", exit=(1, 0.5), entry=(0, 0.5))
p.edge("t6", "t5", "повтор", EDGE_ORTHO, points=[(1285, qb + TH + 34), (1014, qb + TH + 34)], exit=(0.5, 1), entry=(0.8, 1))
p.edge("gE", "ev", exit=(1, 0.5), entry=(0, 0.5))
p.edge("ev", "t10")
p.edge("t10", "e3")
p.edge("t5", "d1", "", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.edge("d1", "d2")
p.edge("d2", "t7", "", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1))
p.edge("t7", "ev", "", EDGE_DASH, exit=(0.5, 0), entry=(0.5, 1))
p.edge("t1", "rv", "повторы с backoff продолжаются", EDGE_ORTHO, points=[(255, 88), (1856, 88)], exit=(0.5, 0), entry=(0.5, 0))
p.edge("rv", "t8")
p.edge("t8", "p1", "Events API v2", EDGE_DASH, exit=(0.5, 1), entry=(0.5, 0))
p.edge("p1", "e1")
p.edge("t8", "t9", "pd.delivery", EDGE_ORTHO, points=[(2265, q0 + 32)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("t9", "e2")
p.text("Резервный канал не заменяет PagerDuty, а закрывает время его недоступности. Все отправки пишутся в журнал (notify.log), "
       "Watchdog раз в сутки отправляет тестовое сообщение по каждому резервному каналу, чтобы канал не оказался сломанным в момент аварии.",
       40, ys[-1][0] + ys[-1][1] + 20, 2000, 40)
p.legend(labels={TASK_HOT: "Отправка в PagerDuty или резервные каналы"}, extra=[(EVGATE, "Шлюз по событию: что наступит раньше"), (MSGEV, "Промежуточное событие: сообщение")], edges=BP_EDGES)

# ---------------------------------------------------------------------
# Дорожная карта
# ---------------------------------------------------------------------
p = Page("Дорожная карта", "road"); pages.append(p)
if MVP:
    MW = 205
    p.text("<b>Дорожная карта MVP: пять этапов и пилот за 6 месяцев</b>", 40, 20, 1400, 30, TITLE)
    STG = [("1. Каркас", 1, 1, "Репозиторий, OpenBao, схема БД,<br>модель события, вход через OIDC", TASK),
           ("2. Сбор и шаблоны", 2, 3, "Конструктор коннекторов, парсеры, ack и inbox, шаблоны,<br>нагрузочный тест 10 000 событий в минуту", TASK),
           ("3. Карта CMDB и роли", 3, 4, "Обнаружение КЕ, плавающие облачные ID, ревью карты, РСМ;<br>ролевая модель и синхронизация с каталогом (AD)", TASK),
           ("4. Тревоги и дашборд инцидентов", 4, 5, "Схлопывание, окна обслуживания, RED и USE, влияние по РСМ;<br>дашборд инцидентов: фильтры, поиск, предустановки", TASK),
           ("5. PagerDuty, резервные каналы, контекст в Grafana", 5, 6,
            "Events API, webhooks, sync-back, кеш дежурств, почта и webhook, Watchdog;<br>Context Builder, связи контекста, дашборды инцидентов в Grafana", TASK_HOT),
           ("Пилот", 6, 6, "3 VM, 2–3 источника,<br>одна команда дежурных", L_CI)]
    NM = 6
else:
    MW = 120
    p.text("<b>Дорожная карта целевой архитектуры: от MVP к полному решению за 14 месяцев</b>", 40, 20, 1400, 30, TITLE)
    STG = [("MVP (отдельный набор документов)", 1, 6, "Этапы 1–5 и пилот: сбор, тревоги, PagerDuty и резервные каналы, дашборд инцидентов,<br>роли и синхронизация с каталогом, контекст в Grafana", L_MON),
           ("6. История", 7, 8, "ClickHouse, History Service,<br>отчёты MTTA и MTTR, поиск по истории", TASK),
           ("7. Полные резервные каналы", 7, 9, "SMS- и голосовой шлюз, мессенджеры с кнопкой,<br>escalation policy, запасная платформа", TASK_HOT),
           ("8. Корреляция", 9, 11, "Correlation Engine, инциденты и вероятная причина;<br>инциденты на дашборде, все КЕ в Grafana", TASK),
           ("9. ITSM", 10, 12, "ITSM Sync, обмен с корпоративной<br>CMDB, тикеты", TASK),
           ("10. Вторая площадка и 20 000/мин", 11, 14, "Тёплый резерв, RTO 15 мин, 5 app-узлов,<br>нагрузочный тест 40 000 событий в минуту", TASK)]
    NM = 14
X0 = 400
RH_ = 84
for m in range(1, NM + 1):
    p.node(f"m{m}", f"М{m}", X0 + (m - 1) * MW, 70, MW, 30, HDR2 + "align=center;")
for i, (n, a, b, d, st) in enumerate(STG):
    y = 110 + i * RH_
    p.node(f"n{i}", f"<b>{escape(n)}</b>", 40, y, 350, RH_ - 12, CELL2 + "fontSize=11;")
    p.node(f"b{i}", d, X0 + (a - 1) * MW + 4, y + 2, (b - a + 1) * MW - 8, RH_ - 16, st + "fontSize=10;")
p.text("Каждый этап заканчивается работающей системой, которую можно показать. Резервное оповещение входит уже в MVP (почта и webhook): "
       "без него недоступность PagerDuty оставляет дежурных без алертов. Трудоёмкость MVP — 28–34 человеко-месяца." if MVP else
       "Этапы 6–9 частично параллельны внутри одной команды (5,5 человека); вторая площадка нужна до перехода на нагрузку "
       "20 000 событий в минуту. Трудоёмкость месяцев 7–14 — 40–45 человеко-месяцев. Каждый этап выпускается отдельно и не ломает работающую систему.",
       40, 110 + len(STG) * RH_ + 6, X0 + NM * MW - 40, 40)
p.legend(auto=False, extra=[(TASK + "fontSize=10;", "Этап разработки"), (TASK_HOT + "fontSize=10;", "Этап, связанный с оповещением")] +
         ([(L_CI + "fontSize=10;", "Пилотная эксплуатация")] if MVP else [(L_MON + "fontSize=10;", "Этап MVP")]) +
         [(HDR2, "Месяц проекта")], edges=[])

# ---------------------------------------------------------------------
# Открытие инцидента в Grafana: последовательность
# ---------------------------------------------------------------------
p = Page("Открытие инцидента в Grafana", "gfs"); pages.append(p)
p.text("<b>Открытие инцидента в Grafana: клик на дашборде инцидентов → сгенерированный дашборд контекста</b>", 40, 20, 1600, 30, TITLE)
INC = "2051" if MVP else "731"
PART = [("duty", "Дежурный", LIFE_EXT), ("web", "Веб-интерфейс<br>(браузер)", LIFE), ("api", "Core API", LIFE),
        ("nats", "NATS JetStream", LIFE), ("cb", "Context Builder", LIFE), ("pg", "PostgreSQL", LIFE),
        ("gr", "Grafana", LIFE_EXT), ("st", "Хранилища метрик,<br>логов и трейсов", LIFE_EXT)]
STEPS = [
    ("m", "duty", "web", "клик по строке<br>инцидента #" + INC, EDGE),
    ("m", "web", "api", "GET /go/incidents/" + INC + "/<br>grafana (новая вкладка)", EDGE),
    ("s", "api", "сессия OIDC, право<br>dashboard.view, инцидент<br>в области пользователя"),
    ("m", "api", "nats", "request<br>context.request {id}", EDGE),
    ("m", "nats", "cb", "context.request<br>(queue group)", EDGE),
    ("m", "cb", "pg", "инцидент, события, КЕ,<br>правила связей контекста,<br>связи КЕ по карте CMDB", EDGE),
    ("m", "pg", "cb", "данные", EDGE_DASH),
    ("alt", "[в кеше, без изменений]", "nats", "gr"),
    ("s", "cb", "взять uid и URL из кеша"),
    ("else", "[иначе]"),
    ("s", "cb", "Rule Matcher → Relation<br>Resolver (глубина ≤ 3) →<br>Variable Binder → Time<br>Window → Dashboard Composer"),
    ("m", "cb", "gr", "POST /api/dashboards/db:<br>папка команды, uid umb-" + INC + ",<br>overwrite (идемпотентно)", EDGE),
    ("m", "gr", "cb", "200: url дашборда", EDGE_DASH),
    ("m", "cb", "gr", "POST /api/annotations:<br>события инцидента", EDGE),
    ("end",),
    ("m", "cb", "nats", "reply: URL c from/to<br>(начало − 10 мин …<br>сейчас + 10 мин)", EDGE_DASH),
    ("m", "nats", "api", "reply: URL", EDGE_DASH),
    ("m", "api", "web", "302 Found,<br>Location: URL Grafana", EDGE_DASH),
    ("m", "web", "gr", "GET /d/umb-" + INC + "<br>?from=…&amp;to=…", EDGE),
    ("s", "gr", "вход через IdP (OIDC, SSO),<br>права папки команды"),
    ("m", "gr", "st", "запросы панелей: метрики,<br>логи, трейсы", EDGE),
    ("m", "st", "gr", "ряды, логи, трейсы", EDGE_DASH),
    ("m", "gr", "duty", "графики, логи, трейсы<br>с аннотациями событий", EDGE_DASH),
    ("alt", "[Grafana недоступна]", "duty", "gr"),
    ("m", "cb", "nats", "reply: ошибка<br>grafana_unavailable", EDGE_DASH),
    ("m", "nats", "api", "reply: ошибка", EDGE_DASH),
    ("m", "api", "web", "страница ошибки<br>с данными инцидента", EDGE_DASH),
    ("s", "web", "события, хронология, статус<br>PagerDuty из Umbrella;<br>кнопка «Повторить»"),
    ("end",),
]
y = seq(p, PART, STEPS)
p.node("note", "<b>Правила</b><br><font style='font-size:11px'>"
       "• дашборд генерируется по запросу (цель ≤ 2 с) и кешируется (ветка «в кеше»: инцидент и правила не менялись); пересобирается при изменении инцидента или правил связей контекста<br>"
       "• окно по умолчанию: начало инцидента − 10 мин … закрытие (или сейчас) + 10 мин; правило может задать своё окно<br>"
       "• Variable Binder экранирует значения под язык запроса и берёт шаблоны только из белого списка: текст событий в запросы не попадает<br>"
       "• токен сервисного аккаунта Grafana берётся из OpenBao, права только на папки Umbrella; Cleanup Job удаляет дашборды через 30 дней после закрытия<br>"
       "• та же ссылка /go/incidents/{id}/grafana передаётся в событие PagerDuty (links) и в резервные сообщения"
       + ("" if MVP else "<br>• для коррелированного инцидента в дашборд попадают все КЕ инцидента, вероятная причина отмечена")
       + "</font>", 40, y + 16, len(PART) * 210 - 40, 120 if MVP else 136, L_TEAM + LEFT + "verticalAlign=top;spacingTop=6;")
p.legend(labels={L_TEAM: "Правила генерации контекста"},
         extra=[(SELF, "Внутренняя обработка участника"), (FRAME, "Фрагмент alt: альтернативные ветки")], skip=(TASK,),
         edges=SEQ_EDGES)

# ---------------------------------------------------------------------
# БП-6 Разбор инцидента
# ---------------------------------------------------------------------
p = Page("БП-6 Разбор инцидента", "bp6"); pages.append(p)
p.text("<b>БП-6 · Разбор инцидента: от оповещения через дашборд инцидентов к контексту в Grafana</b>", 40, 20, 1400, 30, TITLE)
ys = lanes(p, ["PagerDuty", "Дежурный", "Umbrella (дашборд<br>и Context Builder)", "Grafana"], 40, 70, 2520, [150, 160, 300, 150])
r0 = mid(ys, 0, TH)
r1 = mid(ys, 1, TH)
ua = ys[2][0] + 40
ub = ys[2][0] + 180
r3 = mid(ys, 3, TH)
p.node("s", "", 100, mid(ys, 0, EW), EW, EW, START)
p.text("Оповещение<br>об инциденте", 145, mid(ys, 0, EW) + 3, 110, 30, EVLABEL + "align=left;")
p.node("d1", "Открыть дашборд<br>инцидентов", 180, r1, TW, TH, TASK)
p.node("u1", "Применить предуста-<br>новку группы: фильтры,<br>колонки, виджеты", 380, ua, TW + 10, TH + 6, TASK)
p.node("d2", "Найти инцидент:<br>фильтры, поиск,<br>группировка", 600, r1, TW, TH, TASK)
p.node("d3", "Клик по<br>инциденту", 800, r1, TW, TH, TASK)
p.node("u2", "Проверить права<br>и область, запросить<br>контекст (context.request)", 970, ua + 4, TW + 20, TH + 6, TASK)
p.node("g", "Контекст<br>настроен?", 1210, ua - 1, GW + 10, GW + 10, GATE)
p.node("u3", "Собрать дашборд<br>по связям контекста,<br>аннотации событий", 1340, ua + 4, TW + 10, TH + 6, TASK)
p.node("u4", "Базовые панели КЕ<br>по типу КЕ", 1175, ub, TW, TH, TASK)
p.node("u5", "Задача инженеру<br>мониторинга: настроить<br>связи контекста", 1390, ub - 3, TW + 10, TH + 6, TASK_HOT)
p.node("e2", "", 1610, ub + 14, EW, EW, END)
p.node("gr1", "Показать дашборд<br>инцидента за окно<br>(вход OIDC)", 1700, r3, TW, TH + 6, TASK)
p.node("d4", "Разобраться по<br>графикам, логам,<br>трейсам", 1900, r1, TW, TH + 6, TASK)
p.node("d5", "ack / resolve<br>в PagerDuty<br>или в Umbrella", 2100, r1, TW, TH + 6, TASK)
p.node("u6", "Передать статус<br>в источники (sync-back)<br>и в PagerDuty", 2210, ua + 4, TW + 20, TH + 6, TASK_HOT)
p.node("e1", "", 2450, mid(ys, 0, EW), EW, EW, END)
p.text("статус<br>синхронизирован", 2330, mid(ys, 0, EW) + 3, 110, 30, EVLABEL + "align=right;")
p.edge("s", "d1", "ссылка<br>в оповещении", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", exit=(0.5, 1), entry=(0, 0.5),
       points=[(118, r1 + 32)])
p.edge("d1", "u1", "", EDGE_ORTHO, points=[(460, r1 + 32)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("u1", "d2", "", EDGE_ORTHO, points=[(675, ua + 35)], exit=(1, 0.5), entry=(0.5, 1))
p.edge("d2", "d3")
p.edge("d3", "u2", "GET /go/incidents/{id}/grafana", EDGE_ORTHO, points=[(1055, r1 + 32)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("u2", "g", exit=(1, 0.5), entry=(0, 0.5))
p.edge("g", "u3", "да", exit=(1, 0.5), entry=(0, 0.5))
p.edge("g", "u4", "нет", exit=(0.5, 1), entry=(0.5, 0))
p.edge("u4", "u5", exit=(1, 0.5), entry=(0, 0.5))
p.edge("u5", "e2")
p.edge("u3", "gr1", "POST /api/dashboards/db", EDGE_ORTHO, points=[(1775, ua + 39)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("u4", "gr1", "", EDGE_ORTHO, points=[(1250, r3 + 35)], exit=(0.5, 1), entry=(0, 0.5))
p.edge("gr1", "d4", "", EDGE_ORTHO, points=[(1975, r3 + 35)], exit=(1, 0.5), entry=(0.5, 1))
p.edge("d4", "d5")
p.edge("d5", "u6", "", EDGE_ORTHO, points=[(2295, r1 + 35)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("u6", "e1", "Events API v2", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(2468, ua + 39)], exit=(1, 0.5), entry=(0.5, 1))
p.text(("Инцидент на дашборде MVP — тревога, переданная в PagerDuty (одна на проблему). " if MVP else
        "Инцидент на дашборде — инцидент Correlation Engine (группа тревог); дашборд в Grafana включает все КЕ инцидента и отмечает вероятную причину. ")
       + "Дашборд и Grafana показывают только сервисы из области пользователя. Если правило связей контекста для типа КЕ не найдено, "
       "показываются базовые панели КЕ, а инженер мониторинга получает задачу настроить связи.",
       40, ys[-1][0] + ys[-1][1] + 20, 2000, 40)
p.legend(labels={TASK_HOT: "Действие с внешним эффектом: задача инженеру, sync-back"}, edges=BP_EDGES)

# ---------------------------------------------------------------------
# БП-7 Предоставление и отзыв доступа
# ---------------------------------------------------------------------
p = Page("БП-7 Предоставление и отзыв доступа", "bp7"); pages.append(p)
p.text("<b>БП-7 · Предоставление и отзыв доступа через группы корпоративного каталога</b>", 40, 20, 1400, 30, TITLE)
ys = lanes(p, ["Руководитель,<br>владелец группы", "Корпоративный<br>каталог (AD)", "Umbrella<br>(Directory Sync, RBAC)", "Пользователь"],
           40, 70, 2560, [150, 150, 300, 150])
k0 = mid(ys, 0, TH)
k1 = mid(ys, 1, TH)
ka = ys[2][0] + 36
kb = ys[2][0] + 186
k3 = mid(ys, 3, TH)
p.text("<b>Предоставление</b>", 100, ys[0][0] + 6, 200, 20, NOTE)
p.text("<b>Отзыв</b>", 1430, ys[0][0] + 6, 200, 20, NOTE)
p.text("<b>Изменение привязки группы к роли</b>", 100, kb - 30, 300, 20, NOTE)
p.node("s1", "", 100, mid(ys, 0, EW), EW, EW, START)
p.text("Заявка<br>на доступ", 70, mid(ys, 0, EW) + 40, 100, 30, EVLABEL)
p.node("r1", "Согласовать: добавить<br>в группу, например<br>UMB-oncall-payments", 180, k0, TW + 20, TH + 6, TASK)
p.node("a1", "Добавить<br>в группу", 400, k1, TW, TH, TASK)
p.node("u1", "Синхронизация:<br>LDAPS 636 каждые 15 мин,<br>SCIM 2.0 или при входе", 590, ka, TW + 20, TH + 6, TASK)
p.node("u2", "Применить привязку:<br>роль, область,<br>предустановка дашборда;<br>команда в Grafana", 810, ka - 4, TW + 20, TH + 14, TASK)
p.node("p1", "Вход через IdP<br>(OIDC)", 1040, k3, TW, TH, TASK)
p.node("p2", "Дашборд инцидентов<br>с предустановкой<br>группы", 1230, k3, TW, TH + 6, TASK)
p.node("e1", "", 1420, mid(ys, 3, EW), EW, EW, END)
p.node("s2", "", 1430, mid(ys, 0, EW) + 20, EW, EW, START)
p.text("Увольнение<br>или перевод", 1400, mid(ys, 0, EW) + 58, 100, 30, EVLABEL)
p.node("r2", "Заявка: удалить<br>из группы", 1520, k0 + 20, TW, TH, TASK)
p.node("a2", "Удалить<br>из группы", 1720, k1, TW, TH, TASK)
p.node("u3", "Синхронизация<br>(не позже 15 мин)", 1910, ka, TW, TH, TASK)
p.node("u4", "Отозвать сессии<br>и токены, убрать<br>из команды Grafana,<br>записать в аудит", 2110, ka - 4, TW + 10, TH + 14, TASK_HOT)
p.node("p3", "Доступ закрыт", 2330, k3, TW, TH, TASK)
p.node("e3", "", 2510, mid(ys, 3, EW), EW, EW, END)
p.node("s3", "", 100, kb + 14, EW, EW, START)
p.node("b1", "Администратор ролей<br>меняет привязку:<br>группа → роль, область", 180, kb, TW + 20, TH + 6, TASK)
p.node("b2", "Второй администратор<br>подтверждает<br>(второй ключ)", 400, kb, TW + 20, TH + 6, TASK_HOT)
p.node("b3", "Применить<br>и записать в аудит", 620, kb + 3, TW, TH, TASK)
p.node("e4", "", 820, kb + 14, EW, EW, END)
p.edge("s1", "r1")
p.edge("r1", "a1", "", EDGE_ORTHO, points=[(475, k0 + 35)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("a1", "u1", "LDAPS / SCIM", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(675, k1 + 32)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("u1", "u2")
p.edge("u2", "p1", "", EDGE_ORTHO, points=[(1115, ka + 35)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("p1", "p2")
p.edge("p2", "e1")
p.edge("s2", "r2")
p.edge("r2", "a2", "", EDGE_ORTHO, points=[(1795, k0 + 52)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("a2", "u3", "LDAPS / SCIM", EDGE_DASH + "edgeStyle=orthogonalEdgeStyle;", points=[(1985, k1 + 32)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("u3", "u4")
p.edge("u4", "p3", "", EDGE_ORTHO, points=[(2405, ka + 35)], exit=(1, 0.5), entry=(0.5, 0))
p.edge("p3", "e3")
p.edge("s3", "b1")
p.edge("b1", "b2")
p.edge("b2", "b3")
p.edge("b3", "e4")
p.text("Каталог — источник истины: права в Umbrella не выдаются вручную. Изменение привязки группы к роли требует двух ключей, "
       "себе права не выдаются; администратор ролей не редактирует коннекторы и правила. Аварийный локальный администратор "
       "(break-glass): учётные данные в OpenBao, MFA, каждый вход в аудит.", 40, ys[-1][0] + ys[-1][1] + 20, 2000, 40)
p.legend(labels={TASK_HOT: "Второй ключ или отзыв доступа"}, edges=BP_EDGES)

# ---------------------------------------------------------------------
# Ролевая модель
# ---------------------------------------------------------------------
MX_ON = BASE + "rounded=0;fillColor=#DAE8FC;strokeColor=#DDDDDD;fontSize=14;fontColor=#08427B;"
MX_OFF = BASE + "rounded=0;fillColor=#FFFFFF;strokeColor=#DDDDDD;fontSize=14;fontColor=#AAAAAA;"
MX_PERM = BASE + "rounded=0;fillColor=#FFFFFF;strokeColor=#DDDDDD;fontSize=10;align=left;spacingLeft=6;fontFamily=Courier New;"
p = Page("Ролевая модель", "rbac"); pages.append(p)
p.text("<b>Ролевая модель: группы каталога → привязки → роль, область, предустановки дашборда → пользователь</b>", 40, 20, 1700, 30, TITLE)
p.node("ad", "<b>Корпоративный каталог (AD)</b>", 40, 70, 290, 440, NODE_BOUND)
ADG = ["UMB-oncall-payments", "UMB-oncall-web", "UMB-mon-engineers", "UMB-svc-owners-payments", "UMB-auditors",
       "UMB-role-admins", "UMB-umbrella-admins"]
for i, g in enumerate(ADG):
    p.node(f"g{i}", g, 60, 110 + i * 54, 250, 40, L_MON + "fontFamily=Courier New;fontSize=11;")
p.node("idp", c4("Корпоративный IdP", "External System", "OIDC: claims групп при входе"), 40, 560, 290, 90, EXT)
p.node("ds", c4("Directory Sync", "Компонент Core API",
                 "LDAPS 636: опрос каждые 15 мин (OU, префикс UMB-*) или SCIM 2.0 push от IdP. "
                 "Удалён из группы → отзыв при следующей синхронизации (≤ 15 мин), сессии и токены отозваны"),
       450, 180, 240, 230, COMP)
p.node("gb", "<b>Привязки групп (GroupBinding)</b><br><br><font style='font-size:11px'>"
       "группа → роль + область + предустановки<br><br>"
       "пример: UMB-oncall-payments → Дежурный инженер; область: бизнес-услуга «Платежи», команда payments; "
       "предустановка «Дежурство payments»<br><br>"
       "изменение — в два ключа: администратор ролей и второй подтверждающий; запись в аудит</font>",
       800, 150, 280, 290, L_TEAM + LEFT + "verticalAlign=top;spacingTop=8;")
PERMS = ["dashboard.view", "incident.ack", "incident.silence", "connector.edit", "connector.publish", "template.edit",
         "rule.edit", "maintenance.create", "maintenance.approve", "cmdb.review", "routing.edit", "contacts.edit",
         "context.edit", "presets.edit", "roles.admin", "audit.read"]
if TGT:
    PERMS += ["reports.view", "itsm.admin", "correlation.edit"]
RLH = 170 if TGT else 150
p.node("role", "<b>Роль</b> — набор разрешений на функции<br><font style='font-size:10px;font-family:Courier New'>"
       + ", ".join(PERMS) + "</font>", 1170, 70, 380, RLH, L_SVC + LEFT + "verticalAlign=top;spacingTop=8;")
p.node("scope", "<b>Область</b> — на чём действуют разрешения:<br><font style='font-size:11px'>бизнес-услуги, ИТ-сервисы, "
       "команды, коннекторы и источники</font>", 1170, 265, 380, 90, L_BIZ + LEFT + "verticalAlign=top;spacingTop=8;")
p.node("preset", "<b>Предустановки дашборда</b><br><font style='font-size:11px'>фильтры (например, сервисы команды), колонки, "
       "группировка и сортировка, виджеты, автообновление, правила связей контекста, папка Grafana; версии</font>",
       1170, 380, 380, 110, L_CI + LEFT + "verticalAlign=top;spacingTop=8;")
p.node("user", c4("Пользователь", "Person",
                  "Права — объединение прав всех его групп, область — объединение областей. Видит все предустановки "
                  "своих групп, по умолчанию — группы с высшим приоритетом. Личные представления только внутри области"),
       1630, 190, 290, 200, PERSON)
p.edge("ad", "ds", "LDAPS 636 /<br>SCIM 2.0,<br>каждые 15 мин", exit=(1, 0.3545), entry=(0, 0.2))
p.edge("idp", "ds", "OIDC claims при входе", EDGE_ORTHO, points=[(570, 605)], exit=(1, 0.5), entry=(0.5, 1))
p.edge("ds", "gb", "группы<br>и участники", exit=(1, 0.5), entry=(0, 0.5))
p.edge("gb", "role", "", EDGE_ORTHO, points=[(1125, 190), (1125, 70 + RLH / 2)], exit=(1, 0.138), entry=(0, 0.5))
p.edge("gb", "scope", "", exit=(1, 0.5517), entry=(0, 0.5))
p.edge("gb", "preset", "", EDGE_ORTHO, points=[(1125, 400), (1125, 435)], exit=(1, 0.862), entry=(0, 0.5))
p.edge("role", "user", "", EDGE_ORTHO, points=[(1590, 70 + RLH / 2), (1590, 240)], exit=(1, 0.5), entry=(0, 0.25))
p.edge("scope", "user", "", exit=(1, 0.5), entry=(0, 0.6))
p.edge("preset", "user", "", EDGE_ORTHO, points=[(1590, 435), (1590, 340)], exit=(1, 0.5), entry=(0, 0.75))
# Grafana, break-glass, SoD
BY = 700
p.node("fts", c4("Folder & Team Sync", "Компонент Context Builder",
                 "команда Grafana на каждую группу каталога, права на папки команд"), 450, BY, 290, 120, COMP)
p.node("gr", c4("Grafana", "External System", "папки «Umbrella · ‹команда›», вход через тот же IdP"), 860, BY, 260, 120, EXT)
p.edge("gb", "fts", "привязки", EDGE_ORTHO, points=[(940, 655), (660, 655)], exit=(0.5, 1), entry=(0.7, 0))
p.edge("fts", "gr", "HTTPS 443:<br>teams, folder<br>permissions", exit=(1, 0.5), entry=(0, 0.5))
p.node("bg", "<b>Аварийный доступ (break-glass)</b><br><font style='font-size:11px'>локальный администратор на случай "
       "недоступности каталога или IdP: учётные данные только в OpenBao, MFA, каждый вход — в аудит и SIEM</font>",
       1170, BY, 380, 120, L_ALERT + LEFT + "verticalAlign=top;spacingTop=8;")
p.node("sod", "<b>Разделение обязанностей</b><br><font style='font-size:11px'>администратор ролей не редактирует коннекторы "
       "и правила; никто не выдаёт права себе; привязки групп меняются в два ключа</font>",
       1630, BY, 290, 120, L_TEAM + LEFT + "verticalAlign=top;spacingTop=8;")
# Матрица «Роль × функции»
ROLES = ["Наблюда-<br>тель", "Дежурный<br>инженер", "Инженер<br>монито-<br>ринга", "Владелец<br>сервиса", "Аудитор",
         "Админи-<br>стратор<br>ролей", "Админи-<br>стратор<br>Umbrella"]
GRANT = {  # Н Д И В А Р У
    "dashboard.view": "1111101", "incident.ack": "0100001", "incident.silence": "0110001", "connector.edit": "0010001",
    "connector.publish": "0010001", "template.edit": "0010001", "rule.edit": "0010001", "maintenance.create": "0111001",
    "maintenance.approve": "0001001", "cmdb.review": "0011001", "routing.edit": "0011001", "contacts.edit": "0001001",
    "context.edit": "0010001", "presets.edit": "0000011", "roles.admin": "0000010", "audit.read": "0000111",
    "reports.view": "1111101", "itsm.admin": "0000001", "correlation.edit": "0010001",
}
MY = BY + 190
p.text("<b>Встроенные роли × разрешения</b>", 40, MY - 34, 600, 24, NOTE + "fontSize=13;")
p.node("mh", "Разрешение", 40, MY, 220, 56, HDR2)
for j, r in enumerate(ROLES):
    p.node(f"mr{j}", r, 260 + j * 110, MY, 110, 56, HDR2 + "fontSize=10;")
for i, pm in enumerate(PERMS):
    y = MY + 56 + i * 26
    p.node(f"mp{i}", pm, 40, y, 220, 26, MX_PERM)
    for j in range(7):
        on = GRANT[pm][j] == "1"
        p.node(f"mc{i}_{j}", "●" if on else "○", 260 + j * 110, y, 110, 26, MX_ON if on else MX_OFF)
p.text("● — разрешение есть, действует только в пределах области роли; ○ — разрешения нет.<br><br>"
       "Роли и состав разрешений настраиваются администратором ролей; встроенные роли — стартовый набор.<br><br>"
       "Разрешения на изменение (connector.publish, maintenance.approve, привязки групп) требуют второго ключа: "
       "автор изменения не может сам его утвердить.<br><br>"
       "Пользователь в нескольких группах получает объединение разрешений; личные представления дашборда "
       "не расширяют область роли.", 1170, MY, 700, 200, NOTE)
p.legend(labels={L_MON: "Группа каталога", L_TEAM: "Привязка, правило", L_SVC: "Роль (разрешения)", L_BIZ: "Область",
                 L_CI: "Предустановка дашборда", L_ALERT: "Аварийный доступ", NODE_BOUND: "Корпоративный каталог",
                 COMP: "Компонент Umbrella", PERSON: "Пользователь"},
         extra=[(HDR2, "Заголовок матрицы"), (MX_ON, "● разрешение есть"), (MX_OFF, "○ разрешения нет")])

# ---------------------------------------------------------------------
# Макет: дашборд инцидентов
# ---------------------------------------------------------------------
WF = BASE + "rounded=0;"
WF_FRAME = WF + "fillColor=#FFFFFF;strokeColor=#555555;strokeWidth=2;"
WF_URL = WF + "fillColor=#EEEEEE;strokeColor=#555555;fontSize=10;fontColor=#555555;align=left;spacingLeft=12;fontFamily=Courier New;"
WF_APPBAR = WF + "fillColor=#08427B;strokeColor=#08427B;fontColor=#FFFFFF;fontSize=11;align=left;spacingLeft=12;"
WF_INPUT = BASE + "rounded=1;arcSize=10;fillColor=#FFFFFF;strokeColor=#999999;fontSize=10;fontColor=#444444;align=left;spacingLeft=8;"
WF_BTN = BASE + "rounded=1;arcSize=20;fillColor=#E8E8E8;strokeColor=#999999;fontSize=10;fontColor=#222222;"
WF_BTNP = BASE + "rounded=1;arcSize=20;fillColor=#1168BD;strokeColor=#0B4884;fontSize=10;fontColor=#FFFFFF;fontStyle=1;"
WF_HDR = WF + "fillColor=#E0E0E0;strokeColor=#BBBBBB;fontSize=10;fontStyle=1;align=left;spacingLeft=6;"
WF_CELL = WF + "fillColor=#FFFFFF;strokeColor=#DDDDDD;fontSize=10;align=left;spacingLeft=6;"
WF_HOVER = WF + "fillColor=#EAF2FB;strokeColor=#6C8EBF;fontSize=10;align=left;spacingLeft=6;"
WF_PANEL = WF + "fillColor=#FAFAFA;strokeColor=#AAAAAA;fontSize=10;align=left;verticalAlign=top;spacingLeft=8;spacingTop=4;"
WF_BAR = WF + "fillColor=#85BBF0;strokeColor=none;"
WF_TXT = "text;html=1;fillColor=none;strokeColor=none;align=left;verticalAlign=middle;fontSize=10;fontColor=#333333;whiteSpace=wrap;"
SEV = {"critical": BASE + "rounded=1;arcSize=30;fillColor=#F8CECC;strokeColor=#B85450;fontSize=10;fontStyle=1;",
       "error": BASE + "rounded=1;arcSize=30;fillColor=#FFE6CC;strokeColor=#D79B00;fontSize=10;fontStyle=1;",
       "warning": BASE + "rounded=1;arcSize=30;fillColor=#FFF2CC;strokeColor=#D6B656;fontSize=10;fontStyle=1;",
       "info": BASE + "rounded=1;arcSize=30;fillColor=#DAE8FC;strokeColor=#6C8EBF;fontSize=10;fontStyle=1;"}
CALL = ("ellipse;shape=ellipse;perimeter=ellipsePerimeter;html=1;fillColor=#C0392B;strokeColor=#FFFFFF;strokeWidth=2;"
        "fontColor=#FFFFFF;fontSize=11;fontStyle=1;")


def callout(p, n, x, y, sfx=""):
    p.node(f"call{n}{sfx}", str(n), x, y, 24, 24, CALL)


p = Page("Макет: дашборд инцидентов", "mdash"); pages.append(p)
p.text("<b>Макет страницы «Дашборд инцидентов» в веб-интерфейсе Umbrella</b>", 40, 20, 1500, 30, TITLE)
FX, FY, FWd = 40, 70, 1650
FH = 670
p.node("frame", "", FX, FY, FWd, FH, WF_FRAME)
p.node("url", "https://umbrella.corp.example/incidents?preset=oncall-payments&amp;status=open,ack&amp;severity=critical,error",
       FX, FY, FWd, 28, WF_URL)
p.node("appbar", "<b>Umbrella</b>", FX, FY + 28, FWd, 50, WF_APPBAR)
p.text("<u><b>Дашборд инцидентов</b></u> · Карта CMDB · Коннекторы · Связи контекста · Администрирование", FX + 110, FY + 38, 520, 30,
       WF_TXT + "fontColor=#FFFFFF;fontSize=11;")
p.node("search", "Поиск: заголовок, КЕ, сервис, текст событий", FX + 660, FY + 38, 520, 30, WF_INPUT)
callout(p, 1, FX + 1190, FY + 41)
p.text("Иванов И. · Дежурный инженер ▾", FX + 1400, FY + 38, 236, 30, WF_TXT + "fontColor=#FFFFFF;align=right;")
# предустановки и представления
L = FX + 46  # левый край содержимого (слева — поле выносок)
y = FY + 92
callout(p, 2, FX + 12, y + 3)
p.node("preset", "Предустановка группы: <b>Дежурство payments</b> ▾", L, y, 340, 30, WF_INPUT)
p.node("views", "Мои представления: Ночная смена ▾", L + 350, y, 260, 30, WF_INPUT)
p.node("bsave", "Сохранить представление", L + 620, y, 170, 30, WF_BTN)
p.node("blink", "Ссылка с фильтрами", L + 800, y, 140, 30, WF_BTN)
p.node("bcsv", "Экспорт CSV", L + 950, y, 110, 30, WF_BTN)
p.node("bgrp", "Группировка: по сервису ▾", L + 1070, y, 190, 30, WF_INPUT)
p.text("● обновление в реальном времени (WebSocket)", L + 1280, y, 300, 30, WF_TXT + "fontColor=#2E7D32;")
# фильтры
y = FY + 140
callout(p, 3, FX + 12, y + 20)
FLT = ["Статус: открыт, ack ▾", "Severity: critical, error ▾", "Бизнес-услуга: все ▾", "ИТ-сервис: все ▾", "КЕ: все ▾",
       "Тип КЕ: все ▾", "Команда: payments ▾", "Источник: все ▾", "Метод: RED, USE ▾", "PagerDuty: все ▾",
       "Резервное оповещение: все ▾", "Период: 24 ч ▾"]
for i, f in enumerate(FLT):
    p.node(f"f{i}", f, L + (i % 6) * 263, y + (i // 6) * 36, 253, 28, WF_INPUT + "fillColor=#F4F8FC;")
# счётчики и мини-хронология
y = FY + 222
callout(p, 4, FX + 12, y + 23)
for i, (sv, n) in enumerate([("critical", 3), ("error", 7), ("warning", 12), ("info", 4)]):
    p.node(f"cnt{i}", f"<font style='font-size:20px'><b>{n}</b></font><br>{sv}", L + i * 140, y, 130, 70, SEV[sv])
TLX = L + 580
p.node("tl", "Инциденты по часам, 24 ч", TLX, y, 1000, 70, WF_PANEL)
HB = [2, 1, 1, 0, 1, 2, 3, 2, 1, 1, 2, 4, 3, 2, 2, 1, 3, 5, 4, 2, 1, 2, 6, 3]
for i, hgt in enumerate(HB):
    bh = 6 * hgt + 2
    p.node(f"tb{i}", "", TLX + 14 + i * 40, y + 64 - bh, 28, bh, WF_BAR)
# массовые действия
y = FY + 310
callout(p, 5, FX + 12, y + 3)
p.text("☑ Выбрано: 2", L, y, 100, 30, WF_TXT)
p.node("back", "Подтвердить (ack)", L + 110, y, 150, 30, WF_BTN)
p.node("bsil", "Заглушить (silence)", L + 270, y, 150, 30, WF_BTN)
p.text("кнопки видны только при правах incident.ack и incident.silence в области пользователя", L + 440, y, 600, 30,
       WF_TXT + "fontColor=#777777;")
# таблица
TY = FY + 356
COLS_ = [("☐", 30), ("Severity", 80), ("Заголовок", 290), ("КЕ", 100), ("Сервис", 120), ("Начало", 60),
         ("Длительность", 92), ("PagerDuty", 92), ("Резервный канал", 116), ("Действия", 110)]
callout(p, 6, FX + 12, TY + 3)
x = L
for j, (n, w) in enumerate(COLS_):
    p.node(f"th{j}", n + (" ▾" if n == "Начало" else ""), x, TY, w, 30, WF_HDR)
    x += w
TW_ = x - L
if MVP:
    ROWS = [("critical", "Служба приложения не отвечает", "app-01", "Платежи", "09:12", "18 мин", "не принят", "почта, 09:14"),
            ("error", "Доля ошибок HTTP 5xx выше 5% (RED)", "api-gw-02", "API платежей", "09:20", "10 мин", "triggered", "—"),
            ("error", "Задержка репликации БД (USE)", "db-01", "Платежи", "08:55", "35 мин", "ack", "—"),
            ("warning", "Диск заполнен на 85% (USE)", "app-03", "Личный кабинет", "07:40", "1 ч 50 мин", "ack", "—"),
            ("info", "Окно обслуживания завершено", "lb-02", "Сайт", "06:00", "3 ч", "resolved", "—")]
else:
    ROWS = [("critical", "Служба не отвечает · 3 тревоги, причина db-01", "app-01, db-01", "Платежи", "09:12", "18 мин", "не принят", "SMS, 09:14"),
            ("error", "Доля ошибок HTTP 5xx выше 5% (RED) · 1 тревога", "api-gw-02", "API платежей", "09:20", "10 мин", "triggered", "—"),
            ("error", "Задержка репликации БД (USE) · 2 тревоги", "db-02", "Отчёты", "08:55", "35 мин", "ack", "—"),
            ("warning", "Диск заполнен на 85% (USE) · 1 тревога", "app-03", "Личный кабинет", "07:40", "1 ч 50 мин", "ack", "—"),
            ("info", "Окно обслуживания завершено · 1 тревога", "lb-02", "Сайт", "06:00", "3 ч", "resolved", "—")]
for i, (sv, ttl, ci, svc, st_, dur, pdst, fb) in enumerate(ROWS):
    yy = TY + 30 + i * 40
    cst = WF_HOVER if i == 0 else WF_CELL
    vals = ["☑" if i < 2 else "☐", "", ttl, ci, svc, st_, dur, pdst, fb, "ack · silence · <b>i</b>"]
    x = L
    for j, ((n, w), v) in enumerate(zip(COLS_, vals)):
        p.node(f"r{i}c{j}", v, x, yy, w, 40, cst)
        if j == 1:
            p.node(f"sv{i}", sv, x + 6, yy + 10, w - 12, 20, SEV[sv])
        x += w
callout(p, 7, FX + 12, TY + 38)
p.node("hint", "↗ откроется дашборд инцидента в Grafana (новая вкладка)", L + 120, TY + 30 + 5 * 40 + 10, 420, 26,
       WF_INPUT + "fillColor=#FFFDE7;strokeColor=#D6B656;")
callout(p, 8, L + 550, TY + 30 + 5 * 40 + 11)
# боковая панель
SPX = L + TW_ + 20
SPW = FX + FWd - 16 - SPX
p.node("sp", "", SPX, TY, SPW, 300, WF_PANEL)
p.node("sph", "Инцидент #" + ("2051" if MVP else "731") + " · critical", SPX, TY, SPW, 30, WF_HDR)
callout(p, 9, SPX + SPW - 30, TY + 3)
p.text("<b>События</b> · Хронология · PagerDuty · Резервное оповещение", SPX + 8, TY + 36, SPW - 16, 24, WF_TXT)
p.text("09:12 открыт: служба payment-svc не отвечает (app-01)<br>"
       "09:12 PagerDuty: trigger — таймаут, повторы<br>"
       "09:13 circuit breaker открыт, канал деградировал<br>"
       "09:14 резервное оповещение: " + ("почта дежурному L1" if MVP else "SMS дежурному L1") + "<br>"
       "09:16 событие: HTTP 5xx 12% (api-gw-02)<br>"
       + ("09:24 нет ack за 10 мин → резервные контакты" if MVP else "09:24 таймаут уровня 1 → уровень 2") + "<br><br>"
       "<b>PagerDuty:</b> не принят (канал деградировал)<br>"
       "<b>Резервный канал:</b> отправлено 2, ack нет<br>"
       + ("<b>КЕ:</b> app-01 → db-01 (по карте CMDB)" if MVP else "<b>КЕ:</b> app-01, db-01 (вероятная причина)"), SPX + 8, TY + 64, SPW - 16, 180, WF_TXT + "verticalAlign=top;")
p.node("spg", "↗ Открыть в Grafana", SPX + 10, TY + 256, 150, 30, WF_BTNP)
p.node("spa", "Подтвердить (ack)", SPX + 170, TY + 256, 140, 30, WF_BTN)
# пояснения к выноскам
KY = FY + FH + 20
p.text("<b>Пояснения к выноскам</b>", 40, KY, 600, 22, NOTE + "fontSize=13;")
KEYS = [
    "Полнотекстовый поиск по заголовку, КЕ, сервису и тексту событий: индексы PostgreSQL full-text и trigram"
    + ("." if MVP else "; история старше 7 дней — в ClickHouse через History Service."),
    "Предустановка группы из ролевой модели (фильтры, колонки, группировка, виджеты) и личные представления поверх неё; "
    "ссылка с фильтрами, экспорт CSV, группировка по сервису, команде или КЕ.",
    "Фильтры: статус, severity, бизнес-услуга, ИТ-сервис, КЕ, тип КЕ, команда, источник, метод RED/USE, "
    "состояние в PagerDuty, резервное оповещение, период. Значения ограничены областью роли.",
    "Счётчики по severity и мини-хронология инцидентов по часам; обновляются по WebSocket.",
    "Массовые действия ack и silence — только с правами incident.ack и incident.silence.",
    "Сортировка по любой колонке (стрелка в заголовке), группировка строк по выбранному признаку.",
    "Клик по строке открывает дашборд инцидента в Grafana в новой вкладке: /go/incidents/{id}/grafana.",
    "При наведении на строку — подсказка: клик откроет дашборд инцидента в Grafana.",
    "Иконка «i» открывает боковую панель Umbrella: события, хронология, статус PagerDuty, журнал резервного оповещения.",
]
for i, k in enumerate(KEYS):
    cx = 40 + (i % 3) * 560
    cy = KY + 30 + (i // 3) * 52
    callout(p, i + 1, cx, cy + 2, "k")
    p.text(escape(k), cx + 32, cy, 510, 46, NOTE + "fontSize=11;")
p.text("Инцидент на дашборде: " + ("тревога, переданная в PagerDuty (одна на проблему)." if MVP else
       "инцидент Correlation Engine — группа тревог с вероятной причиной."), 40, KY + 30 + 3 * 52 + 4, 1600, 22, NOTE + "fontSize=11;")
p.legend(auto=False, extra=[
    (WF_FRAME, "Окно браузера"), (WF_APPBAR, "Шапка приложения"), (WF_INPUT, "Поле, список, фильтр"),
    (WF_BTN, "Кнопка"), (WF_BTNP, "Основная кнопка"), (WF_HDR, "Заголовок таблицы или панели"),
    (WF_CELL, "Ячейка таблицы"), (WF_HOVER, "Строка под курсором"), (WF_PANEL, "Панель, виджет"),
    (WF_BAR, "Столбец мини-хронологии"), (SEV["critical"], "Severity critical"), (SEV["error"], "Severity error"),
    (SEV["warning"], "Severity warning"), (SEV["info"], "Severity info"), (CALL, "Выноска: пояснение под макетом"),
], edges=[])

# ---------------------------------------------------------------------
# Макет: дашборд инцидента в Grafana
# ---------------------------------------------------------------------
WF_ROWH = WF + "fillColor=#F0F0F0;strokeColor=#BBBBBB;fontSize=11;fontStyle=1;align=left;spacingLeft=8;"
WF_CARD = BASE + "rounded=1;arcSize=6;fillColor=#FFF2CC;strokeColor=#D6B656;fontSize=10;align=left;verticalAlign=top;spacingLeft=8;spacingTop=4;"
SPARK = "endArrow=none;html=1;rounded=0;strokeColor=#1168BD;strokeWidth=2;"
ANN = "endArrow=none;html=1;dashed=1;dashPattern=4 3;strokeColor=#C0392B;strokeWidth=1.5;"
MONO10 = WF_TXT + "verticalAlign=top;fontFamily=Courier New;fontSize=10;"
p = Page("Макет: дашборд инцидента в Grafana", "mgrf"); pages.append(p)
p.text("<b>Макет дашборда инцидента в Grafana: пример «служба приложения упала»</b>", 40, 20, 1500, 30, TITLE)
INC = "2051" if MVP else "731"
FX, FY, FWd, FH = 40, 70, 1200, 1040
p.node("frame", "", FX, FY, FWd, FH, WF_FRAME)
p.node("bar", "<b>Grafana</b> › Дашборды › Umbrella · payments › Инцидент #" + INC + " (uid umb-" + INC + ")", FX, FY, FWd, 44, WF_APPBAR)
p.node("time", "09:02 … сейчас + 10 мин ▾", FX + 960, FY + 7, 225, 30, WF_INPUT)
p.text("<b>Инцидент #" + INC + " · critical · Служба приложения не отвечает</b><br>"
       "КЕ: служба payment-svc на хосте app-01" + ("" if MVP else ", БД db-01") +
       " · окно: начало 09:12 − 10 мин = 09:02 … сейчас (или закрытие) + 10 мин", FX + 16, FY + 52, 1160, 40, WF_TXT + "fontSize=11;")
p.text("Переменные:", FX + 16, FY + 100, 80, 26, WF_TXT)
for i, v in enumerate(["ci.host = app-01", "service = payment-svc", "ci.name = db-01", "incident = " + INC]):
    p.node(f"var{i}", v, FX + 100 + i * 180, FY + 100, 170, 26, WF_INPUT + "fontFamily=Courier New;")
RX0 = FX + 40
RW = FWd - 56


def rowh(n, y, title):
    callout(p, n, FX + 8, y + 1)
    p.node(f"row{n}", "▾ " + title, RX0, y, RW, 26, WF_ROWH)


def chart(key, x, y, w, h, title, vals):
    p.node(key, "<b>" + title + "</b>", x, y, w, h, WF_PANEL)
    px0, px1, py0, py1 = x + 14, x + w - 14, y + 30, y + h - 24
    for fr in (10 / 38, 12 / 38, 19 / 38):
        ax = px0 + (px1 - px0) * fr
        p.line(ax, py0 - 4, ax, py1, "", ANN)
    n = len(vals)
    pts = [(px0 + (px1 - px0) * i / (n - 1), py1 - (py1 - py0) * v) for i, v in enumerate(vals)]
    for (ax, ay), (bx, by) in zip(pts, pts[1:]):
        p.line(round(ax, 1), round(ay, 1), round(bx, 1), round(by, 1), "", SPARK)
    p.text("09:02", px0 - 4, py1 + 2, 50, 18, WF_TXT + "fontSize=10;fontColor=#777777;")
    p.text("09:40", px1 - 40, py1 + 2, 44, 18, WF_TXT + "fontSize=10;fontColor=#777777;align=right;")


# R1 сводка
y = FY + 140
rowh(1, y, "Сводка инцидента")
p.node("sum", "<b>Сводка инцидента</b><br>Статус: открыт · critical · PagerDuty: не принят<br>"
       "Резервное оповещение: " + ("почта" if MVP else "SMS") + " дежурному L1, 09:14<br>"
       "Бизнес-услуга: Платежи · ИТ-сервис: payment-svc<br>Ссылка: карточка инцидента в Umbrella",
       RX0, y + 32, 560, 108, WF_PANEL)
p.node("anl", "<b>Аннотации: события Umbrella</b><br>09:12 тревога открыта<br>"
       + ("" if MVP else "09:13 в инцидент добавлена тревога db-01<br>") + "09:14 резервное оповещение<br>09:21 ack (Иванов И.)",
       RX0 + 570, y + 32, RW - 570, 108, WF_PANEL)
# R2 хост
y = FY + 296
rowh(2, y, "Хост app-01: CPU, RAM, диск")
CW_ = (RW - 20) / 3
chart("cpu", RX0, y + 32, CW_, 170, "CPU, %", [.35, .38, .36, .55, .9, .95, .1, .05, .06, .05, .05, .06])
chart("ram", RX0 + CW_ + 10, y + 32, CW_, 170, "RAM, %", [.5, .55, .6, .7, .82, .93, .3, .28, .28, .29, .3, .3])
chart("disk", RX0 + 2 * (CW_ + 10), y + 32, CW_, 170, "Диск: заполнение и IOPS", [.6, .6, .61, .62, .62, .63, .63, .63, .64, .64, .64, .65])
# R3 БД
y = FY + 516
rowh(3, y, "Связанная БД db-01 (по карте CMDB: payment-svc → использует → db-01)" if MVP else
     "★ Вероятная причина: БД db-01 (КЕ инцидента #731; связь payment-svc → использует → db-01)")
HW_ = (RW - 10) / 2
chart("dbc", RX0, y + 32, HW_, 170, "Активные соединения", [.4, .42, .45, .7, .97, .98, .2, .15, .15, .16, .15, .15])
chart("dbl", RX0 + HW_ + 10, y + 32, HW_, 170, "Время ответа запросов, мс", [.1, .12, .15, .5, .85, .9, .4, .2, .15, .12, .12, .1])
# R4 логи
y = FY + 736
rowh(4, y, "Логи службы payment-svc (уровень ≥ error)")
p.node("logs", "", RX0, y + 32, RW, 110, WF_PANEL)
p.text("09:11:41 ERROR pool exhausted: 200/200 connections to db-01:5432<br>"
       "09:11:58 ERROR request timeout after 5000 ms: POST /api/payments<br>"
       "09:12:03 FATAL out of memory, process exited (code 137)<br>"
       "09:12:20 ERROR health check failed: connection refused :8443", RX0 + 10, y + 40, RW - 20, 96, MONO10)
# R5 трейсы
y = FY + 892
rowh(5, y, "Трейсы службы payment-svc (ошибки)")
p.node("tr", "", RX0, y + 32, RW, 110, WF_PANEL)
TRC = [("<b>trace id</b>", "<b>операция</b>", "<b>длительность</b>", "<b>статус</b>"),
       ("4f2a…91c", "POST /api/payments", "5 004 мс", "error: timeout db-01"),
       ("9b7e…02d", "GET /api/payments/{id}", "4 870 мс", "error: timeout db-01"),
       ("c11d…7aa", "POST /api/refunds", "5 001 мс", "error: timeout db-01")]
for i, row in enumerate(TRC):
    for j, (v, cx) in enumerate(zip(row, (0, 160, 420, 560))):
        p.text(v, RX0 + 10 + cx, y + 38 + i * 18, 240, 18, MONO10 + "verticalAlign=middle;")
# справа: какие правила дали строки
SX = FX + FWd + 40
p.text("<b>Связи контекста → строки дашборда</b>", SX, FY, 470, 26, NOTE + "fontSize=13;")
CARDS = [
    (1, "<b>Сводка инцидента</b> — блок «Сводка инцидента» есть в каждом дашборде; аннотации — события инцидента из Umbrella", 50),
    (2, "<b>Правило «Служба приложения: недоступность»</b><br>Условие: тип КЕ = служба приложения, сигнал = доступность<br>"
        "Обход: служба → «размещена на» → хост, глубина 1<br>Панели метрик: CPU, RAM, диск; шаблон запроса с ${ci.host}", 72),
    (3, "<b>То же правило, второй обход</b><br>Обход: служба → «использует» → БД, глубина 1<br>"
        "Панели метрик: соединения, время ответа; шаблон с ${ci.name}", 60),
    (4, "<b>Панель логов</b><br>Источник: хранилище логов; фильтр service = ${service}, уровень ≥ error", 46),
    (5, "<b>Панель трейсов</b><br>Источник: хранилище трейсов; service = ${service}, status = error", 46),
]
cy = FY + 36
for n, txt, h in CARDS:
    callout(p, n, SX, cy + 4, "s")
    p.node(f"card{n}", txt, SX + 32, cy, 440, h, WF_CARD)
    cy += h + 14
p.node("cwin", "<b>Окно времени</b>: начало инцидента − 10 мин … закрытие (или сейчас) + 10 мин; правило может задать своё. "
       "Значения переменных экранируются, шаблоны запросов только из белого списка.", SX + 32, cy, 440, 60, WF_CARD)
cy += 74
p.node("ctgt", ("<b>Порядок применения</b>: правила выбираются по условию, обход по карте CMDB — не глубже 3 уровней; "
                "если правило для типа КЕ не найдено, показываются базовые панели КЕ.") if MVP else
       ("<b>Коррелированный инцидент</b>: строки строятся для всех КЕ инцидента (здесь app-01 и db-01); КЕ вероятной "
        "причины отмечена ★ в заголовке строки. Обход по карте CMDB — не глубже 3 уровней."),
       SX + 32, cy, 440, 62, WF_CARD)
p.legend(auto=False, extra=[
    (WF_FRAME, "Окно браузера"), (WF_APPBAR, "Шапка Grafana"), (WF_INPUT, "Выбор времени, переменная"),
    (WF_ROWH, "Строка дашборда (row)"), (WF_PANEL, "Панель Grafana"), (CALL, "Номер строки и правила"),
    (WF_CARD, "Правило связей контекста"),
], edges=[(SPARK, "Ряд метрики"), (ANN, "Аннотация: событие инцидента из Umbrella")])


# ---------------------------------------------------------------------
# Порядок страниц и отметка набора в заголовке
# ---------------------------------------------------------------------
ORDER = ["ctx", "flc", "fnf", "cnt", "cweb", "mdash", "capi", "rbac", "cing", "crt", "cnrm", "crul", "cale", "cdb", "cpdg", "cfbn",
         "cwdg", "cctx", "mgrf", "chst", "ccor", "citm", "bus", "ifc", "seq", "fbs", "gfs", "erd", "lay", "dep", "nod", "inf", "ha",
         "int", "thr", "bld", "cld", "rsm", "sig", "dup", "bp1", "bp2", "bp3", "bp4", "bp5", "bp6", "bp7", "road"]
pages.sort(key=lambda pg: ORDER.index(pg.pid) if pg.pid in ORDER else len(ORDER))
TAG = "MVP" if MVP else "Целевая архитектура"
for pg in pages:
    for i, c in enumerate(pg.cells):
        if c[0] == "v" and c[3] == TITLE:
            pg.cells[i] = (c[0], c[1], c[2].replace("</b>", f" · {TAG}</b>", 1), c[3], c[4], c[5])
            break
