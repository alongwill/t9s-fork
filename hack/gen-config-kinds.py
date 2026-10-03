#!/usr/bin/env python3
"""Extract the config-document catalogue for t9s from the Talos config explorer data.

Source: agent-skills/talos/tutorials/config-explorer-v2/data.js (window.TALOS_DOCS),
which generate-data.py builds from every registry.Register(...) in the Talos source.

Usage: hack/gen-config-kinds.py <path/to/data.js> > internal/catalog/config-kinds.json
"""
import json, re, sys

src = open(sys.argv[1]).read()
m = re.search(r"window\.TALOS_DOCS\s*=\s*(\[.*?\]);\s*\n\s*window\.", src, re.S)
docs = json.loads(m.group(1))
meta = json.loads(re.search(r"window\.TALOS_META\s*=\s*(\{.*?\});", src, re.S).group(1))
out = {
    "generatedFrom": meta.get("talosDescribe", ""),
    "kinds": [{k: d.get(k, "") for k in ("kind", "group", "since", "desc")} for d in docs],
}
json.dump(out, sys.stdout, indent=1)
sys.stdout.write("\n")
