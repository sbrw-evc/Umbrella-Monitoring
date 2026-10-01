#!/bin/bash
# Собирает генератор, пишет draw.io и PNG для набора: ./build.sh [mvp|target|both]
set -e
G=$(cd "$(dirname "$0")" && pwd); ROOT=$(dirname "$G")
MODES=${1:-both}; [ "$MODES" = both ] && MODES="mvp target"
cat $G/head.py $G/body.py $G/body_new.py $G/b3_comp.py $G/b3_main.py $G/tail.py > $G/generate_drawio.py
for m in $MODES; do
  MODE=$m python3 $G/generate_drawio.py $ROOT/$m/diagrams/umbrella-$m.drawio > /dev/null
  T=$(mktemp -d); node $G/render.js $ROOT/$m/diagrams/umbrella-$m.drawio $T > /dev/null
  rm -f $ROOT/$m/diagrams/*.png
  python3 - "$ROOT" "$m" "$T" <<'PY'
import sys, xml.etree.ElementTree as ET, shutil
R, m, T = sys.argv[1:]
ids = [d.get("id") for d in ET.parse(f"{R}/{m}/diagrams/umbrella-{m}.drawio").getroot().iter("diagram")]
for i, pid in enumerate(ids, 1):
    shutil.move(f"{T}/page{i}.png", f"{R}/{m}/diagrams/{i:02d}-{pid}.png")
print(m, len(ids), "pages")
PY
done
