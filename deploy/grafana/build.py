#!/usr/bin/env python3
"""Builds the file-provisioning JSON from the Grafana dashboard resources.

The *.yaml files next to this script are the source of truth: Grafana 12
resources (apiVersion dashboard.grafana.app/v1beta1, kind Dashboard) whose
spec is the classic dashboard JSON model. Grafana file provisioning reads
JSON only, so this writes spec to dashboards/<metadata.name>.json.

    python3 deploy/grafana/build.py           # regenerate the JSON
    python3 deploy/grafana/build.py --check   # fail when the JSON is stale

Needs PyYAML (Debian/Ubuntu: apt install python3-yaml; or pip install pyyaml).
"""
import json
import pathlib
import sys

HERE = pathlib.Path(__file__).resolve().parent
OUT = HERE / "dashboards"


def load_yaml(path):
    try:
        import yaml
    except ImportError:
        sys.exit("build.py: PyYAML is missing (apt install python3-yaml or pip install pyyaml)")
    with open(path, encoding="utf-8") as f:
        return yaml.safe_load(f)


def render(doc, path):
    if doc.get("apiVersion", "").split("/")[0] != "dashboard.grafana.app" or doc.get("kind") != "Dashboard":
        sys.exit(f"{path}: not a dashboard.grafana.app Dashboard resource")
    name = doc["metadata"]["name"]
    spec = doc["spec"]
    if spec.get("uid") != name:
        sys.exit(f"{path}: spec.uid {spec.get('uid')!r} must equal metadata.name {name!r}")
    return name, json.dumps(spec, ensure_ascii=False, indent=2) + "\n"


def main():
    check = "--check" in sys.argv[1:]
    stale = []
    for path in sorted(HERE.glob("*.yaml")):
        doc = load_yaml(path)
        if not isinstance(doc, dict) or doc.get("kind") != "Dashboard":
            continue
        name, text = render(doc, path)
        target = OUT / f"{name}.json"
        current = target.read_text(encoding="utf-8") if target.exists() else None
        if check:
            if current is None or json.loads(current) != json.loads(text):
                stale.append(target)
            continue
        if current != text:
            OUT.mkdir(exist_ok=True)
            target.write_text(text, encoding="utf-8")
            print(f"wrote {target.relative_to(HERE.parent.parent)}")
    if stale:
        for t in stale:
            print(f"stale: {t} (run python3 deploy/grafana/build.py)", file=sys.stderr)
        sys.exit(1)
    if check:
        print("dashboards are up to date")


if __name__ == "__main__":
    main()
