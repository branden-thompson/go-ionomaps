"""PLAN's dry run, the fetch (watchpost FR-10.6): a week of GIRO readings and 3-hourly GloTEC grids.

Under the feature's throttle (watchpost D-39): GIRO's live stations in one burst of at most 40, one request
per station for the whole range; on a 429 it waits 60, 120, 240 and 480 s (15 minutes in all), retrying after each, then stops
that source for the run; NOAA grids at most 6 an hour (one every 10 minutes). Every response's status, size, time and
rate-related headers are logged; CDN location headers are never kept (D-83).

Usage: dry_fetch.py OUTDIR START END STATIONS_FILE   (dates YYYY-MM-DD, inclusive)
"""
import json, os, sys, time, urllib.error, urllib.request
from datetime import date, datetime, timedelta, timezone

OUT, START, END, STATIONS = sys.argv[1], date.fromisoformat(sys.argv[2]), date.fromisoformat(sys.argv[3]), sys.argv[4]
UA = "watchpost-discover (+https://github.com/branden-thompson/watchpost)"
RATE = ("retry-after", "x-ratelimit", "ratelimit")
os.makedirs(OUT, exist_ok=True)
log = open(f"{OUT}/requests.jsonl", "a")


def get(url, path):
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            body, status, hdr = r.read(), r.status, dict(r.headers)
    except urllib.error.HTTPError as e:
        body, status, hdr = e.read() or b"", e.code, dict(e.headers or {})
    except Exception as e:
        body, status, hdr = b"", -1, {"error": repr(e)}
    if status == 200 and path:
        open(path, "wb").write(body)
    rate = {k: v for k, v in hdr.items() if k.lower().startswith(RATE)}
    rec = {"utc": datetime.now(timezone.utc).isoformat(timespec="seconds"), "url": url, "status": status,
           "bytes": len(body), "secs": round(time.monotonic() - t0, 3), "rate_headers": rate}
    log.write(json.dumps(rec) + "\n"); log.flush()
    print(f"{rec['utc']} {status} {len(body):>9} {url[:100]}", flush=True)
    return status


def with_backoff(url, path):
    wait = 60
    while True:
        st = get(url, path)
        if st not in (429, 503, -1):
            return st
        if wait > 900:
            print("giving up on this source after the 15-minute back-off", flush=True)
            return st
        print(f"denied ({st}); backing off {wait}s", flush=True)
        time.sleep(wait); wait *= 2


stations = [l.strip() for l in open(STATIONS) if l.strip()]
assert len(stations) <= 40, "D-39: one burst is at most 40"
a, b = START.strftime("%Y%%2F%m%%2F%d"), END.strftime("%Y%%2F%m%%2F%d")
for st in stations:
    url = (f"https://lgdc.uml.edu/fastchar/getbest?ursiCode={st}&charName=foF2,MUF%28D%29,M%28D%29,hmF2&DMUF=3000"
           f"&fromDate={a}+00%3A00%3A00&toDate={b}+23%3A59%3A59")
    if with_backoff(url, f"{OUT}/fc_{st}.txt") in (429, 503, -1):
        print("GIRO refused after the full back-off: stopping GIRO for this run (D-39)", flush=True)
        break

d = START
while d <= END:
    for hh in range(0, 24, 3):
        name = f"glotec_icao_{d.strftime('%Y%m%d')}T{hh:02d}0500Z.geojson"
        path = f"{OUT}/{name}"
        if not os.path.exists(path):
            if with_backoff(f"https://services.swpc.noaa.gov/products/glotec/geojson_2d_urt/{name}", path) in (429, 503, -1):
                print("NOAA refused after the full back-off: stopping NOAA for this run", flush=True)
                d = END
                break
            time.sleep(600)  # D-39: NOAA grids at most 6 an hour
    d += timedelta(days=1)
print("done", flush=True)
