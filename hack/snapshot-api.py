#!/usr/bin/env python3
"""Snapshot every GET route of the UI backend, to compare before/after a refactor.

    snapshot-api.py snap  BASE_URL OUTDIR   # token in $MIGRATION_TOKEN
    snapshot-api.py diff  DIR_A DIR_B

Read-only. Placeholders in route templates are filled from what the cluster lists;
routes that would reach out to a vCenter or Forklift inventory are skipped. The
recorded status code is part of the snapshot, so a 404 for a missing object is a
valid baseline. Output may contain real object names: keep it out of git.
"""
import gzip, io, json, os, re, sys, tarfile, urllib.error, urllib.request

ROUTES = os.path.join(os.path.dirname(__file__), "..", "ui-backend", "internal", "api", "testdata", "routes.golden")
# Live log streams grow between runs: only status and content type are compared.
VOLATILE = ("/logs",)
SKIP = ("/vcenter/", "/forklift/inventory/", "/download")  # outbound calls / large payloads
DROP_KEYS = {"resourceVersion", "managedFields", "generatedAt", "timestamp", "time", "salt"}


def get(base, path, token):
    req = urllib.request.Request(base + path, headers={"X-Migration-Token": token})
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, r.headers.get("Content-Type", ""), r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.headers.get("Content-Type", ""), e.read()


def norm(v):
    if isinstance(v, dict):
        return {k: norm(x) for k, x in sorted(v.items()) if k not in DROP_KEYS}
    if isinstance(v, list):
        return [norm(x) for x in v]
    return v


def summarize(status, ctype, body):
    out = {"status": status, "content_type": ctype.split(";")[0]}
    if "gzip" in ctype or "tar" in ctype:
        with tarfile.open(fileobj=io.BytesIO(body), mode="r:*") as t:
            # The top directory carries the generation time (vm-import-support-<ts>).
            out["tar_members"] = sorted(re.sub(r"\d{8}T\d{6}Z", "<ts>", m.name) for m in t.getmembers())
    elif "json" in ctype:
        try:
            out["body"] = norm(json.loads(body))
        except ValueError:
            out["body_text"] = body.decode("utf-8", "replace")[:2000]
    else:
        out["body_text"] = body.decode("utf-8", "replace")[:500] if len(body) < 2000 else f"<{len(body)} bytes>"
    return out


def snap(base, outdir):
    token = os.environ["MIGRATION_TOKEN"]
    os.makedirs(outdir, exist_ok=True)
    routes = [l.split(" ", 1)[1].strip() for l in open(ROUTES) if l.startswith("GET ")]

    def first(path):
        st, _, b = get(base, path, token)
        try:
            items = json.loads(b) if st == 200 else []
        except ValueError:
            items = []
        items = items if isinstance(items, list) else []
        return (items[0].get("metadata", {}) if items else {})

    src = first("/api/v1/harvester/vmwaresources")
    plan = first("/api/v1/plans")
    vals = {"namespace": src.get("namespace", "default"), "name": src.get("name", "x"), "id": "x", "resource": "vms"}
    pvals = {"namespace": plan.get("namespace", vals["namespace"]), "name": plan.get("name", vals["name"])}
    done = 0
    for tmpl in routes:
        if tmpl == "/" or any(s in tmpl for s in SKIP):
            continue
        v = dict(vals, **pvals) if tmpl.startswith("/api/v1/plans/") else vals
        path = re.sub(r"\{(\w+)\}", lambda m: v[m.group(1)], tmpl)
        s = summarize(*get(base, path, token))
        if any(x in tmpl for x in VOLATILE):
            s.pop("body", None); s.pop("body_text", None)
        fn = re.sub(r"[^A-Za-z0-9]+", "_", tmpl).strip("_") or "root"
        json.dump({"template": tmpl, "path": path, **s}, open(os.path.join(outdir, fn + ".json"), "w"), indent=1, sort_keys=True)
        done += 1
    print(f"snapshotted {done} routes into {outdir}")


def diff(a, b):
    fa, fb = set(os.listdir(a)), set(os.listdir(b))
    bad = 0
    for f in sorted(fa ^ fb):
        print(f"ONLY IN {'A' if f in fa else 'B'}: {f}"); bad += 1
    for f in sorted(fa & fb):
        x, y = json.load(open(os.path.join(a, f))), json.load(open(os.path.join(b, f)))
        if x != y:
            bad += 1
            print(f"DIFFERS: {f} (status {x['status']} -> {y['status']})")
    print("identical" if not bad else f"{bad} difference(s)")
    return 1 if bad else 0


if __name__ == "__main__":
    if len(sys.argv) == 4 and sys.argv[1] == "snap":
        snap(sys.argv[2].rstrip("/"), sys.argv[3])
    elif len(sys.argv) == 4 and sys.argv[1] == "diff":
        sys.exit(diff(sys.argv[2], sys.argv[3]))
    else:
        sys.exit(__doc__)
