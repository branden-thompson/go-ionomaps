"""Wave 2 in one sitting (D-38): fetch a day's baseline data as watchpost would.

Sequential, at most 5 requests a second (watchpost's httpx default), every response's status,
headers, bytes and timing logged. On the first denial (429/403/503) the source stops, and recovery
is probed once at +60 s, +300 s and +900 s. Usage: burst.py YYYY-MM-DD
"""
import json, re, sys, time, urllib.error, urllib.request
from datetime import datetime, timezone

D = sys.argv[1]
Y, M, DY = D.split("-")
UA = "watchpost-discover (+https://github.com/branden-thompson/watchpost)"
MIN_GAP = 0.2
OUT = f"burst-{D}"
import os; os.makedirs(OUT, exist_ok=True)
log = open(f"{OUT}/requests.jsonl", "a")
last = [0.0]
DENY = {429, 403, 503}


def get(url, path):
    wait = MIN_GAP - (time.monotonic() - last[0])
    if wait > 0:
        time.sleep(wait)
    t0 = time.monotonic(); last[0] = t0
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            body = r.read(); status = r.status; headers = dict(r.headers)
    except urllib.error.HTTPError as e:
        body = e.read() or b""; status = e.code; headers = dict(e.headers or {})
    except Exception as e:  # network failure: logged, treated as a denial for safety
        body = b""; status = -1; headers = {"error": repr(e)}
    dt = time.monotonic() - t0
    if status == 200 and path:
        open(path, "wb").write(body)
    rec = {"utc": datetime.now(timezone.utc).isoformat(timespec="seconds"), "url": url, "status": status,
           "bytes": len(body), "secs": round(dt, 3), "headers": headers}
    log.write(json.dumps(rec) + "\n"); log.flush()
    print(f"{rec['utc']} {status} {len(body):>9} {dt:6.2f}s {url[:110]}", flush=True)
    return status


def recovery(probe_url, label):
    for after in (60, 300, 900):
        print(f"-- {label}: denied; probing again in {after}s", flush=True)
        time.sleep(after)
        st = get(probe_url, None)
        if st == 200:
            print(f"-- {label}: recovered after {after}s", flush=True)
            return after
    print(f"-- {label}: not recovered after 15 min; stopping", flush=True)
    return None


def run(label, items):
    n = 0; started = time.monotonic()
    for url, path in items:
        st = get(url, path); n += 1
        if st in DENY or st == -1:
            print(f"== {label}: DENIED ({st}) on request {n} after {time.monotonic()-started:.0f}s", flush=True)
            recovery(url, label)
            return n, st
    print(f"== {label}: {n} requests, no denial, {time.monotonic()-started:.0f}s", flush=True)
    return n, 200


html = open("../scaled.html").read()
stations = sorted(set(re.findall(r'<option value="([A-Z0-9]{5})">', html)))
giro = [(f"https://lgdc.uml.edu/fastchar/getbest?ursiCode={st}&charName=foF2,MUF%28D%29,M%28D%29,hmF2&DMUF=3000"
         f"&fromDate={Y}%2F{M}%2F{DY}+00%3A00%3A00&toDate={Y}%2F{M}%2F{DY}+23%3A59%3A59", f"{OUT}/fc_{st}.txt")
        for st in stations]
noaa = [(f"https://services.swpc.noaa.gov/products/glotec/geojson_2d_urt/glotec_icao_{Y}{M}{DY}T{h:02d}0500Z.geojson",
         f"{OUT}/glotec_{h:02d}05.geojson") for h in range(24)]

print(f"GIRO stations: {len(stations)}; NOAA grids: {len(noaa)}", flush=True)
run("GIRO", giro)
run("NOAA", noaa)
