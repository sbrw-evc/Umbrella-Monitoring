"""Генератор umbrella-architecture.drawio (многостраничный draw.io)."""
import xml.etree.ElementTree as ET
from html import escape
import os
MODE = os.environ.get("MODE", "mvp")
MVP = MODE == "mvp"
TGT = not MVP
FB_CHANNELS = ("Почта (SMTP) и webhook в чат-системы; включаются, если PagerDuty не принял алерт" if MVP else
               "SMS- и голосовой шлюз, мессенджеры, почта, webhook, запасная платформа оповещения")

# ---------- стили ----------
BASE = "whiteSpace=wrap;html=1;fontFamily=Helvetica;"
PERSON = BASE + "rounded=1;arcSize=30;fillColor=#08427B;strokeColor=#073B6F;fontColor=#FFFFFF;fontSize=12;"
SYSTEM = BASE + "rounded=1;arcSize=6;fillColor=#1168BD;strokeColor=#0B4884;fontColor=#FFFFFF;fontSize=12;"
EXT = BASE + "rounded=1;arcSize=6;fillColor=#8C8C8C;strokeColor=#6B6B6B;fontColor=#FFFFFF;fontSize=12;"
EXT_DASHED = EXT + "dashed=1;"
CONT = BASE + "rounded=1;arcSize=6;fillColor=#438DD5;strokeColor=#3C7FC0;fontColor=#FFFFFF;fontSize=12;"
DB = BASE + "shape=cylinder3;boundedLbl=1;backgroundOutline=1;size=12;fillColor=#438DD5;strokeColor=#3C7FC0;fontColor=#FFFFFF;fontSize=12;"
BUS = BASE + "rounded=1;arcSize=20;fillColor=#438DD5;strokeColor=#3C7FC0;fontColor=#FFFFFF;fontSize=12;"
COMP = BASE + "rounded=1;arcSize=6;fillColor=#85BBF0;strokeColor=#78A8D8;fontColor=#000000;fontSize=12;"
BOUND = ("rounded=1;arcSize=1;dashed=1;dashPattern=8 4;fillColor=none;strokeColor=#666666;"
         "fontColor=#333333;verticalAlign=bottom;align=left;spacingLeft=10;spacingBottom=6;html=1;fontSize=12;")
NODE_BOUND = ("rounded=1;arcSize=2;fillColor=none;strokeColor=#888888;fontColor=#333333;"
              "verticalAlign=top;align=left;spacingLeft=10;spacingTop=6;html=1;fontSize=12;")
TITLE = "text;html=1;fillColor=none;strokeColor=none;align=left;verticalAlign=top;fontSize=18;fontStyle=1;fontColor=#222222;"
NOTE = "text;html=1;fillColor=none;strokeColor=none;align=left;verticalAlign=top;fontSize=12;fontColor=#444444;whiteSpace=wrap;"
EDGE = ("endArrow=block;endFill=1;endSize=6;html=1;rounded=0;strokeColor=#707070;fontColor=#404040;"
        "fontSize=10;labelBackgroundColor=#FFFFFF;")
EDGE_ORTHO = EDGE + "edgeStyle=orthogonalEdgeStyle;"
EDGE_DASH = EDGE + "dashed=1;"

# BPMN
LANE = ("shape=swimlane;swimlane;horizontal=0;startSize=34;html=1;fillColor=#F5F5F5;strokeColor=#9E9E9E;"
        "fontStyle=1;fontSize=12;swimlaneFillColor=#FFFFFF;")
START = "ellipse;shape=ellipse;perimeter=ellipsePerimeter;html=1;fillColor=#D5E8D4;strokeColor=#82B366;strokeWidth=1.5;"
END = "ellipse;shape=ellipse;perimeter=ellipsePerimeter;html=1;fillColor=#F8CECC;strokeColor=#B85450;strokeWidth=3;"
TIMER = "ellipse;shape=ellipse;perimeter=ellipsePerimeter;html=1;fillColor=#FFFFFF;strokeColor=#666666;strokeWidth=1.5;double=1;"
TASK = BASE + "rounded=1;arcSize=12;fillColor=#DAE8FC;strokeColor=#6C8EBF;fontSize=11;"
TASK_HOT = BASE + "rounded=1;arcSize=12;fillColor=#FFE6CC;strokeColor=#D79B00;fontSize=11;"
GATE = BASE + "rhombus;shape=rhombus;perimeter=rhombusPerimeter;fillColor=#FFF2CC;strokeColor=#D6B656;fontSize=10;"
EVLABEL = "text;html=1;fillColor=none;strokeColor=none;align=center;verticalAlign=top;fontSize=10;fontColor=#333333;whiteSpace=wrap;"

# РСМ
L_BIZ = BASE + "rounded=1;arcSize=8;fillColor=#E1D5E7;strokeColor=#9673A6;fontSize=12;"
L_SVC = BASE + "rounded=1;arcSize=8;fillColor=#DAE8FC;strokeColor=#6C8EBF;fontSize=12;"
L_CI = BASE + "rounded=1;arcSize=8;fillColor=#D5E8D4;strokeColor=#82B366;fontSize=12;"
L_MON = BASE + "rounded=1;arcSize=8;fillColor=#F5F5F5;strokeColor=#666666;fontSize=12;"
L_TEAM = BASE + "rounded=1;arcSize=8;fillColor=#FFF2CC;strokeColor=#D6B656;fontSize=12;"
L_ALERT = BASE + "rounded=1;arcSize=8;fillColor=#F8CECC;strokeColor=#B85450;fontSize=12;"
ST_OK = BASE + "rounded=1;arcSize=8;fillColor=#D5E8D4;strokeColor=#82B366;fontSize=11;"
ST_WARN = BASE + "rounded=1;arcSize=8;fillColor=#FFE6CC;strokeColor=#D79B00;fontSize=11;strokeWidth=2;"
ST_CRIT = BASE + "rounded=1;arcSize=8;fillColor=#F8CECC;strokeColor=#B85450;fontSize=11;strokeWidth=2;"

# ИБ / STRIDE
TB = ("rounded=0;dashed=1;dashPattern=6 3;fillColor=none;strokeColor=#C00000;strokeWidth=2;"
      "fontColor=#C00000;verticalAlign=top;align=left;spacingLeft=8;spacingTop=4;html=1;fontSize=11;fontStyle=1;")
THREAT = BASE + "shape=ellipse;perimeter=ellipsePerimeter;fillColor=#C00000;strokeColor=#800000;fontColor=#FFFFFF;fontSize=10;fontStyle=1;"
PROC = BASE + "shape=ellipse;perimeter=ellipsePerimeter;fillColor=#DAE8FC;strokeColor=#6C8EBF;fontSize=11;"
STORE = BASE + "shape=partialRectangle;right=0;left=0;fillColor=#FFF2CC;strokeColor=#D6B656;fontSize=11;"
ENTITY = BASE + "rounded=0;fillColor=#F5F5F5;strokeColor=#666666;fontSize=11;"
# модель данных
ENT = (BASE + "rounded=0;fillColor=#DAE8FC;strokeColor=#6C8EBF;fontSize=10;align=left;verticalAlign=top;"
       "spacingLeft=6;spacingTop=2;")
ENT_CFG = ENT.replace("#DAE8FC", "#FFF2CC").replace("#6C8EBF", "#D6B656")
ENT_CMDB = ENT.replace("#DAE8FC", "#D5E8D4").replace("#6C8EBF", "#82B366")
ENT_EXT = ENT.replace("#DAE8FC", "#F5F5F5").replace("#6C8EBF", "#666666")
# последовательность
LIFE = BASE + "rounded=0;fillColor=#438DD5;strokeColor=#3C7FC0;fontColor=#FFFFFF;fontSize=11;"
LIFE_EXT = BASE + "rounded=0;fillColor=#8C8C8C;strokeColor=#6B6B6B;fontColor=#FFFFFF;fontSize=11;"
ACT = "rounded=0;fillColor=#FFFFFF;strokeColor=#3C7FC0;html=1;"
LLINE = "endArrow=none;dashed=1;html=1;strokeColor=#999999;"
IFACE = BASE + "shape=ellipse;perimeter=ellipsePerimeter;fillColor=#FFFFFF;strokeColor=#3C7FC0;strokeWidth=2;fontSize=10;"
REPL = BASE + "rounded=1;arcSize=8;fillColor=#E1D5E7;strokeColor=#9673A6;fontSize=11;"

LEGEND_DEFAULT = [
    (PERSON, "Пользователь (Person)"),
    (SYSTEM, "Система Umbrella"),
    (CONT, "Контейнер или сервис Umbrella"),
    (COMP, "Компонент внутри контейнера"),
    (BUS, "Шина событий (NATS JetStream)"),
    (DB, "Хранилище данных"),
    (EXT_DASHED, "Внешняя система (опционально)"),
    (EXT, "Внешняя система или инфраструктура"),
    (BOUND, "Граница системы или контейнера"),
    (NODE_BOUND, "Узел, сетевая зона или группа"),
    (LANE, "Дорожка: участник процесса"),
    (START, "Начальное событие"),
    (END, "Конечное событие"),
    (TIMER, "Событие-таймер"),
    (TASK, "Задача"),
    (TASK_HOT, "Задача с внешним эффектом (PagerDuty, откат, ревью)"),
    (GATE, "Развилка (шлюз «или»)"),
    (L_BIZ, "Бизнес-услуга"),
    (L_SVC, "ИТ-сервис"),
    (L_CI, "Конфигурационная единица (КЕ)"),
    (L_MON, "Объект или данные мониторинга"),
    (L_TEAM, "Настройка, правило, команда"),
    (L_ALERT, "Тревога или результат"),
    (ST_OK, "Состояние: норма"),
    (ST_WARN, "Состояние: деградация"),
    (ST_CRIT, "Состояние: авария"),
    (TB, "Граница доверия"),
    (THREAT, "Угроза У-NN из перечня"),
    (PROC, "Процесс (DFD)"),
    (STORE, "Хранилище (DFD)"),
    (ENTITY, "Внешняя сущность (DFD)"),
    (ENT_CFG, "Сущность конфигурации"),
    (ENT_CMDB, "Сущность CMDB и РСМ"),
    (ENT_EXT, "Внешний объект (ссылка)"),
    (ENT, "Операционная сущность"),
    (LIFE_EXT, "Внешний участник"),
    (LIFE, "Участник Umbrella"),
    (ACT, "Активность участника"),
    (IFACE, "Интерфейс (предоставляемый)"),
    (REPL, "Резервная площадка (тёплый резерв)"),
]


def c4(name, kind, desc):
    out = f"<b>{escape(name)}</b>"
    if kind:
        out += f"<br><font style='font-size:10px'>[{escape(kind)}]</font>"
    if desc:
        out += f"<br><br><font style='font-size:11px'>{escape(desc)}</font>"
    return out


class Page:
    def __init__(self, name, pid):
        self.name, self.pid = name, pid
        self.cells = []
        self.geo = {}
        self.n = 0
        self.leg = None

    def _id(self, prefix):
        self.n += 1
        return f"{self.pid}-{prefix}{self.n}"

    def node(self, key, label, x, y, w, h, style, parent="1"):
        cid = f"{self.pid}-{key}"
        self.geo[key] = (x, y, w, h)
        if not hasattr(self, "sty"):
            self.sty = {}
        self.sty[key] = style
        self.cells.append(("v", cid, label, style, (x, y, w, h), parent))
        return key

    def text(self, label, x, y, w, h, style=NOTE):
        return self.node(self._id("note")[len(self.pid) + 1:], label, x, y, w, h, style)

    def edge(self, src, dst, label="", style=EDGE, points=None, exit=None, entry=None, lpos=None, loff=None):
        """lpos: положение подписи вдоль линии (-1 … 1, 0 — середина); loff: сдвиг подписи (dx, dy)."""
        st = style
        if not exit and not entry and not points and src in self.geo and dst in self.geo:
            # почти горизонтальная или почти вертикальная прямая связь выравнивается, чтобы не было косой линии в 1–4 px
            ax, ay, aw, ah = self.geo[src]
            bx, by, bw, bh = self.geo[dst]
            dyc = (by + bh / 2) - (ay + ah / 2)
            dxc = (bx + bw / 2) - (ax + aw / 2)
            if abs(dyc) <= 4 and (bx >= ax + aw or ax >= bx + bw):
                y = by + bh / 2 if ah <= bh else ay + ah / 2
                right = bx >= ax + aw
                exit = (1 if right else 0, round((y - ay) / ah, 4))
                entry = (0 if right else 1, round((y - by) / bh, 4))
            elif abs(dxc) <= 4 and (by >= ay + ah or ay >= by + bh):
                x = bx + bw / 2 if aw <= bw else ax + aw / 2
                down = by >= ay + ah
                exit = (round((x - ax) / aw, 4), 1 if down else 0)
                entry = (round((x - bx) / bw, 4), 0 if down else 1)
        def flat(key, pt):
            # точка у середины стороны эллипса или ромба: без проекции на периметр, иначе линия получается косой
            stl = getattr(self, "sty", {}).get(key, "")
            if not pt or key not in self.geo or not ("ellipse" in stl or "rhombus" in stl):
                return False
            _, _, w, h = self.geo[key]
            return (pt[0] in (0, 1) and abs(pt[1] - 0.5) * h <= 6) or (pt[1] in (0, 1) and abs(pt[0] - 0.5) * w <= 6)
        if exit:
            st += f"exitX={exit[0]};exitY={exit[1]};exitDx=0;exitDy=0;" + ("exitPerimeter=0;" if flat(src, exit) else "")
        if entry:
            st += f"entryX={entry[0]};entryY={entry[1]};entryDx=0;entryDy=0;" + ("entryPerimeter=0;" if flat(dst, entry) else "")
        eid = self._id("edge")
        self.cells.append(("e", eid, label, st, (f"{self.pid}-{src}", f"{self.pid}-{dst}", points or [], lpos, loff), "1"))

    def vert(self, src, dst, label="", style=EDGE, down=True):
        """Вертикальная связь с общей шиной/широким блоком: точка входа по центру источника."""
        sx, sy, sw, sh = self.geo[src]
        dx, dy, dw, dh = self.geo[dst]
        cx = sx + sw / 2
        fx = round((cx - dx) / dw, 4)
        if down:
            self.edge(src, dst, label, style, exit=(0.5, 1), entry=(fx, 0))
        else:
            self.edge(src, dst, label, style, exit=(0.5, 0), entry=(fx, 1))

    def line(self, x1, y1, x2, y2, label="", style=EDGE):
        """Свободная стрелка по координатам (для легенды и схем последовательности)."""
        eid = self._id("edge")
        self.cells.append(("l", eid, label, style, (x1, y1, x2, y2), "1"))

    def legend(self, labels=None, extra=None, skip=(), edges=None, cols=None, auto=True):
        """Легенда под диаграммой. Типы узлов определяются по стилям на странице;
        labels переопределяют подписи, extra добавляет пункты [(style, label)], edges — список (style, label)."""
        self.leg = (labels or {}, extra or [], skip, edges, cols, auto)

    def _build_legend(self):
        labels, extra, skip, edges, cols, auto = self.leg
        used, has_dash, has_solid, first = [], False, False, {}
        known = sorted(LEGEND_DEFAULT, key=lambda t: -len(t[0]))
        for kind, cid, label, style, g, parent in self.cells:
            if kind == "v" and auto:
                for st, _ in known:
                    if style.startswith(st):
                        if st not in used and st not in skip:
                            used.append(st)
                            first[st] = style
                        break
            elif kind == "e":
                if "dashed=1" in style:
                    has_dash = True
                else:
                    has_solid = True
        order = [st for st, _ in LEGEND_DEFAULT]
        used.sort(key=order.index)
        items = [("v", st, labels.get(st, dict(LEGEND_DEFAULT)[st])) for st in used]
        items += [("v", st, lab) for st, lab in extra]
        if edges is None:
            edges = []
            if has_solid:
                edges.append((EDGE, "Связь, вызов или поток данных (направление по стрелке)"))
            if has_dash:
                edges.append((EDGE_DASH, "Обратный, асинхронный или вспомогательный поток"))
        items += [("e", st, lab) for st, lab in edges]
        vs = [g for k, _, _, _, g, par in self.cells if k == "v" and par == "1"]
        ls = [g for k, _, _, _, g, _ in self.cells if k == "l"]
        xs = [g[0] for g in vs]
        rs = [g[0] + g[2] for g in vs]
        bs = [g[1] + g[3] for g in vs] + [max(g[1], g[3]) for g in ls]
        x0, right, y0 = min(xs), max(rs), max(bs) + 30
        width = max(right - x0, 900)
        CWL = 300
        ncol = cols or max(1, int((width - 20) // CWL))
        rows = (len(items) + ncol - 1) // ncol
        RH = 34
        self.node(f"lgbox", "<b>Легенда</b>", x0, y0, width, 40 + rows * RH + 10,
                  "rounded=0;fillColor=#FAFAFA;strokeColor=#BBBBBB;html=1;verticalAlign=top;align=left;"
                  "spacingLeft=10;spacingTop=6;fontSize=12;fontColor=#333333;")
        for i, (k, st, lab) in enumerate(items):
            cx = x0 + 14 + (i % ncol) * CWL
            cy = y0 + 38 + (i // ncol) * RH
            if k == "v":
                sw_st = st
                fs = first.get(st, "")
                if "dashed=1" in fs and "dashed=1" not in st:
                    sw_st = st + "dashed=1;" + ("dashPattern=4 4;" if "dashPattern=4 4" in fs else "")
                if "shape=cylinder3" in st:
                    self.node(f"lgs{i}", "", cx, cy - 4, 40, 30, sw_st)
                elif "ellipse" in st:
                    self.node(f"lgs{i}", "", cx + 8, cy, 24, 24, sw_st)
                elif "rhombus" in st:
                    self.node(f"lgs{i}", "", cx + 6, cy - 2, 28, 28, sw_st)
                else:
                    self.node(f"lgs{i}", "", cx, cy, 40, 24, sw_st)
            else:
                self.line(cx, cy + 12, cx + 40, cy + 12, "", st)
            self.node(f"lgt{i}", escape(lab), cx + 48, cy - 2, CWL - 58, 28,
                      "text;html=1;fillColor=none;strokeColor=none;align=left;verticalAlign=middle;"
                      "fontSize=11;fontColor=#333333;whiteSpace=wrap;")

    def xml(self, diagram):
        if self.leg is not None:
            self._build_legend()
            self.leg = None
        model = ET.SubElement(diagram, "mxGraphModel", dx="1400", dy="900", grid="1", gridSize="10",
                              guides="1", tooltips="1", connect="1", arrows="1", fold="1", page="0",
                              pageScale="1", math="0", shadow="0")
        root = ET.SubElement(model, "root")
        ET.SubElement(root, "mxCell", id="0")
        ET.SubElement(root, "mxCell", id="1", parent="0")
        for kind, cid, label, style, g, parent in self.cells:
            if kind == "v":
                c = ET.SubElement(root, "mxCell", id=cid, value=label, style=style, vertex="1", parent=parent)
                x, y, w, h = g
                ET.SubElement(c, "mxGeometry", x=str(x), y=str(y), width=str(w), height=str(h),
                              **{"as": "geometry"})
            elif kind == "l":
                x1, y1, x2, y2 = g
                c = ET.SubElement(root, "mxCell", id=cid, value=label, style=style, edge="1", parent=parent)
                geo = ET.SubElement(c, "mxGeometry", relative="1", **{"as": "geometry"})
                ET.SubElement(geo, "mxPoint", x=str(x1), y=str(y1), **{"as": "sourcePoint"})
                ET.SubElement(geo, "mxPoint", x=str(x2), y=str(y2), **{"as": "targetPoint"})
            else:
                s, t, pts = g[:3]
                lpos, loff = (g[3], g[4]) if len(g) > 3 else (None, None)
                c = ET.SubElement(root, "mxCell", id=cid, value=label, style=style, edge="1",
                                  parent=parent, source=s, target=t)
                ga = {"x": str(lpos)} if lpos is not None else {}
                geo = ET.SubElement(c, "mxGeometry", relative="1", **ga, **{"as": "geometry"})
                if loff:
                    ET.SubElement(geo, "mxPoint", x=str(loff[0]), y=str(loff[1]), **{"as": "offset"})
                if pts:
                    arr = ET.SubElement(geo, "Array", **{"as": "points"})
                    for px, py in pts:
                        ET.SubElement(arr, "mxPoint", x=str(px), y=str(py))


pages = []

