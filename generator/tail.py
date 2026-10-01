# ---------- запись ----------
mxfile = ET.Element("mxfile", host="app.diagrams.net", agent="Umbrella generator", version="24.0.0")
for pg in pages:
    d = ET.SubElement(mxfile, "diagram", id=pg.pid, name=pg.name)
    pg.xml(d)
ET.indent(mxfile)
import sys
out = sys.argv[1]
ET.ElementTree(mxfile).write(out, encoding="utf-8", xml_declaration=True)
print("pages:", [pg.name for pg in pages])
