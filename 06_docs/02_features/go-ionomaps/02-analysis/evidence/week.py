"""PLAN's dry run, scored (watchpost FR-10.6): a week of GIRO soundings against GloTEC, the climatologies,
B on D and the hybrid (watchpost D-101), for foF2 and MUF(3000), held out; the oracle for go-ionomaps G10.4.

Sections: 1 the climatologies; 2-3 held out, test days and whole week, by region (B on D, the hybrid,
GloTEC, B on C, an effective-index proxy, the climatology as shipped); 4 by distance; 5 by station; 6 the
forecast candidates; 7 by day; 8 quiet against the storm; 9 skill by lead; 10 band answers of the blends
(watchpost D-130); 11 the hours ahead's typical error by distance band (watchpost D-134).

Usage: week.py DIR DSI DISTURBED
  DIR  holds dry_fetch.py's output: fc_*.txt (GIRO) and glotec_*.geojson (NOAA, 3-hourly)
  DSI  NOAA SWPC's text/daily-solar-indices.txt (F10.7 by day)
  DISTURBED  the storm days, comma-separated YYYY-MM-DD, from NOAA's daily geomagnetic indices (required:
             the quiet/storm splits are never printed without it); for the PLAN week: 2026-10-04,2026-10-05

Method:
- A sounding is paired with a grid when within 10 minutes of the grid's time and its confidence score is
  at least 70 (or 999, manual). With WEEK_TAKE_UNKNOWN=1 in the environment, a score of -1 (GIRO's
  "unknown") is taken as well (go-ionomaps G2.1's confidence question).
- GloTEC: foF2 = 8.98e-6 sqrt(NmF2); M(3000)F2 = 1490 / (hmF2 + 176) (Shimazaki 1955, first order).
  MUF(3000) = foF2 x M(3000)F2 for every method.
- Climatology: PyIRI 0.1.7 (MIT). main_library on the raw CCIR or URSI tables, and sh_library on the
  spherical-harmonic refits (watchpost D-43). Two F10.7 rules: the day's observed value, and the mean of the
  30 days before it.
- B on D: the station residuals of foF2 and of M(3000)F2 (sounding minus GloTEC), each predicted at the
  held-out station by a Gaussian process on the other stations at the same grid time. Kernel: exponential
  in great-circle distance (positive definite on the sphere, Gneiting 2013). B on C is the same over the
  climatology.
- Kernel length and noise are tuned on the first three days and scored on the rest (and reported on all).
- Forecast at +3 h and +12 h, issued at each grid time from B on D held out:
    clim(t+h) + (BD(t) - clim(t)) exp(-h/tau)
  tau tuned on the first three days; scored against climatology (watchpost D-75's floor). Also "yesterday":
  B on D at t+h-24 h, held out.

Exits 1 when it finds no grid, no station or no pair, so an empty run never reads as a result.
"""
import bisect, glob, json, math, os, re, sys, warnings
from collections import defaultdict
from datetime import datetime, timedelta

import numpy as np
import PyIRI
import PyIRI.main_library as ml
import PyIRI.sh_library as sh

warnings.filterwarnings("ignore")
if len(sys.argv) != 4:
    sys.exit("usage: week.py DIR DSI DISTURBED (see the docstring)")
D, DSI = sys.argv[1], sys.argv[2]
bad = {datetime.strptime(d, "%Y-%m-%d").date() for d in sys.argv[3].split(",")}  # checked against the week below
MIN_F107_DAYS = 20   # the 30-day mean is refused below this many observed days
PACIFIC = {"EA653", "LL721", "WA619", "GU513"}
CS_MIN = 70
TAKE_UNKNOWN = os.environ.get("WEEK_TAKE_UNKNOWN") == "1"  # GIRO's -1, "unknown", taken too


# ---------- inputs ----------
def f107_by_day(path):
    out = {}
    for line in open(path):
        p = line.split()
        if len(p) > 3 and p[0].isdigit() and len(p[0]) == 4:
            out[datetime(int(p[0]), int(p[1]), int(p[2])).date()] = float(p[3])
    return out


def station(path):
    text = open(path).read()
    m = re.search(r"GEO \(\s*([\d.]+)\s*([NS])\s+([\d.]+)\s*E", text)
    if not m:
        return None, []
    lat = float(m.group(1)) * (1 if m.group(2) == "N" else -1)
    lon = float(m.group(3)); lon = lon - 360 if lon > 180 else lon
    rows = []
    for line in text.splitlines():
        if not line[:4].isdigit():
            continue
        p = line.split()
        try:
            t = datetime.strptime(p[0][:19], "%Y-%m-%dT%H:%M:%S")
            cs, fo, muf, m3 = int(p[1]), float(p[2]), float(p[4]), float(p[6])
        except (ValueError, IndexError):
            continue
        if (cs >= CS_MIN or cs == 999 or (TAKE_UNKNOWN and cs == -1)) and 0.5 < fo < 20 and 1.5 < m3 < 5:
            rows.append((t, fo, muf, m3))
    return (lat, lon), rows


def grid(path):
    d = json.load(open(path))
    pts = {(f["geometry"]["coordinates"][0], f["geometry"]["coordinates"][1]): f["properties"] for f in d["features"]}
    lons = sorted({k[0] for k in pts}); lats = sorted({k[1] for k in pts})
    return datetime.strptime(d["time_tag"][:19], "%Y-%m-%dT%H:%M:%S"), pts, lons, lats


def bil(g, lat, lon, key):
    _, pts, lons, lats = g
    lon = ((lon + 180) % 360) - 180
    i = max(0, min(len(lons) - 2, bisect.bisect(lons, lon) - 1))
    j = max(0, min(len(lats) - 2, bisect.bisect(lats, lat) - 1))
    x0, x1, y0, y1 = lons[i], lons[i + 1], lats[j], lats[j + 1]
    tx = min(1, max(0, (lon - x0) / (x1 - x0))); ty = min(1, max(0, (lat - y0) / (y1 - y0)))
    v = lambda a, b: pts[(a, b)][key]
    return v(x0, y0) * (1 - tx) * (1 - ty) + v(x1, y0) * tx * (1 - ty) + v(x0, y1) * (1 - tx) * ty + v(x1, y1) * tx * ty


F107 = f107_by_day(DSI)
coords, sound = {}, {}
for p in sorted(glob.glob(f"{D}/fc_*.txt")):
    st = p.split("fc_")[1][:5]
    c, rows = station(p)
    if c and rows:
        coords[st], sound[st] = c, rows
grids = [grid(p) for p in sorted(glob.glob(f"{D}/glotec_*.geojson"))]
if not grids or not coords:
    sys.exit(f"week.py: no grids or no stations under {D}")
times = [g[0] for g in grids]
if not bad <= {t.date() for t in times}:
    sys.exit(f"week.py: storm days {sorted(bad - {t.date() for t in times})} are not in the week's grids")
STS = sorted(coords)


def mainland_us(st):
    la, lo = coords[st]
    return 24 <= la <= 50 and -125 <= lo <= -66


def gcd(a, b):
    la1, lo1, la2, lo2 = map(math.radians, (a[0], a[1], b[0], b[1]))
    c = math.sin(la1) * math.sin(la2) + math.cos(la1) * math.cos(la2) * math.cos(lo1 - lo2)
    return 6371.0 * math.acos(max(-1.0, min(1.0, c)))


DIST = {(a, b): gcd(coords[a], coords[b]) for a in STS for b in STS}

# ---------- observations and GloTEC at each grid time ----------
obs = {}   # (t, st) -> (fo, muf, m3)
glo = {}   # (t, st) -> (fo, m3)
for g in grids:
    t0 = g[0]
    for st in STS:
        rows = sound[st]
        near = [r for r in rows if abs(r[0] - t0) <= timedelta(minutes=10)]
        if near:
            r = min(near, key=lambda r: abs(r[0] - t0))
            obs[(t0, st)] = r[1:]
        N = bil(g, *coords[st], "NmF2"); hm = bil(g, *coords[st], "hmF2")
        glo[(t0, st)] = (8.98e-6 * math.sqrt(max(N, 0.0)), 1490.0 / (hm + 176.0))
if not obs:
    sys.exit("week.py: no pairs")

# ---------- climatologies ----------
COEFF_ML = PyIRI.coeff_dir if hasattr(PyIRI, "coeff_dir") else None


F107_DAYS = {}


def f107(day, rule):
    if rule == "day":
        return F107[day]
    prev = [F107[day - timedelta(days=k)] for k in range(1, 31) if day - timedelta(days=k) in F107]
    if len(prev) < MIN_F107_DAYS:
        sys.exit(f"week.py: only {len(prev)} days of F10.7 before {day} (need {MIN_F107_DAYS})")
    F107_DAYS[day] = len(prev)
    return sum(prev) / len(prev)


CLIMS = [("CCIR raw", "ml", 0), ("URSI raw", "ml", 1), ("CCIR refit", "sh", "CCIR"), ("URSI refit", "sh", "URSI")]
clim = {}  # (name, rule) -> {(t, st): (fo, m3)}
lats = np.array([coords[s][0] for s in STS]); lons = np.array([coords[s][1] for s in STS])
by_day = defaultdict(list)
for t in times:
    by_day[t.date()].append(t)
for name, lib, which in CLIMS:
    for rule in ("day", "30d"):
        out = {}
        for day, ts in sorted(by_day.items()):
            ut = np.array([t.hour + t.minute / 60 for t in ts])
            if lib == "ml":
                F2 = ml.IRI_density_1day(day.year, day.month, day.day, ut, lons, lats, np.array([300.0]),
                                         f107(day, rule), COEFF_ML, which)[0]
            else:
                F2 = sh.IRI_density_1day(day.year, day.month, day.day, ut, lons, lats, np.array([300.0]),
                                         f107(day, rule), foF2_coeff=which, old_output=True)[0]
            fo = np.array(F2["fo"]).reshape(len(ts), len(STS)); m3 = np.array(F2["M3000"]).reshape(len(ts), len(STS))
            for i, t in enumerate(ts):
                for j, st in enumerate(STS):
                    out[(t, st)] = (float(fo[i, j]), float(m3[i, j]))
        clim[(name, rule)] = out


# ---------- B on a background, held out ----------
def gp_loo(back, L, noise):
    """{(t, st): (fo, m3)} predicted at each held-out station from the others' residuals at time t."""
    pred = {}
    for t in times:
        here = [s for s in STS if (t, s) in obs]
        for held in here:
            rest = [s for s in here if s != held]
            if len(rest) < 3:
                continue
            out = []
            for k in (0, 2):            # foF2, then M(3000)F2
                bk = 0 if k == 0 else 1
                res = np.array([obs[(t, s)][k] - back[(t, s)][bk] for s in rest])
                mu = res.mean(); y = res - mu; var = max(float(y.var()), 1e-4)
                K = np.array([[var * math.exp(-DIST[(a, b)] / L) for b in rest] for a in rest])
                K += noise * var * np.eye(len(rest))
                kv = np.array([var * math.exp(-DIST[(held, b)] / L) for b in rest])
                out.append(back[(t, held)][bk] + mu + kv @ np.linalg.solve(K, y))
            pred[(t, held)] = (out[0], out[1])
    return pred


def errors(pred, keys):
    """Errors at exactly the keys given: a key the method lacks is an error, never silently dropped."""
    missing = [k for k in keys if k not in pred]
    if missing:
        raise KeyError(f"{len(missing)} of {len(keys)} pairs missing from a method, e.g. {missing[0]}")
    fo = [pred[k][0] - obs[k][0] for k in keys]
    mf = [pred[k][0] * pred[k][1] - obs[k][1] for k in keys]
    return np.array(fo), np.array(mf)


rms = lambda e: float(np.sqrt(np.mean(np.square(e)))) if len(e) else float("nan")
bias = lambda e: float(np.mean(e)) if len(e) else float("nan")
split = times[0] + timedelta(days=3)
ALL = sorted(obs)
TUNE = [k for k in ALL if k[0] < split]
TEST = [k for k in ALL if k[0] >= split]


def tune(back):
    best = None
    for L in (500, 1000, 2000, 4000, 8000):
        for noise in (0.05, 0.2, 0.5, 1.0, 2.0, 4.0):
            p = gp_loo(back, L, noise)
            fo, mf = errors(p, [k for k in TUNE if k in p])
            if not len(fo):
                sys.exit("week.py: nothing to tune on")
            score = rms(fo) / 1.0 + rms(mf) / 3.0
            if best is None or score < best[0]:
                best = (score, L, noise, p)
    return best[1], best[2], best[3]


# the climatology is the one that ships: NRL's CCIR refit (watchpost D-43) with the 30-day mean F10.7 (D-104).
# A-28 chose CCIR over URSI on this same week's figures (section 1), so the choice is not independent of the
# data scored here; fresh days in BUILD (watchpost D-132) are the independent check.
def clim_rms(c):
    fo, mf = errors(c, ALL)
    return rms(fo) + rms(mf) / 3.0


ranked = sorted(clim, key=lambda k: clim_rms(clim[k]))
CBEST = ("CCIR refit", "30d")
Lg, Ng, BD = tune(glo)
Lc, Nc, BC = tune(clim[CBEST])

print(f"week: {times[0]:%Y-%m-%d %H:%M} to {times[-1]:%Y-%m-%d %H:%M}, {len(times)} grids, {len(STS)} stations, "
      f"{len(ALL)} pairs (tune: first 3 days {len(TUNE)}, test: rest {len(TEST)})")
print(f"F10.7 (day): {', '.join(f'{d:%m-%d} {F107[d]:.0f}' for d in sorted(by_day))}")
fetched = glob.glob(f"{D}/fc_*.txt")
no_rows = sum(1 for p in fetched if not station(p)[1])
print(f"stations with usable soundings: {len(STS)} of {len(fetched)} fetched ({no_rows} had no sounding at CS >= {CS_MIN})")
print(f"kernel tuned: B on D L={Lg} km noise={Ng}; B on C L={Lc} km noise={Nc}; the climatology: {CBEST[0]} / F10.7 {CBEST[1]} (as shipped)")

print("\n1. Climatologies (all pairs): foF2 RMS / bias | MUF(3000) RMS / bias")
for k in ranked:
    fo, mf = errors(clim[k], ALL)
    print(f"   {k[0]:11s} F10.7 {k[1]:3s}  {rms(fo):.2f} / {bias(fo):+.2f} | {rms(mf):.2f} / {bias(mf):+.2f}")

# hybrid: foF2 from B on D, M(3000)F2 from B on C (GloTEC's hmF2-derived M(3000)F2 is the weak half)
HY = {k: (BD[k][0], BC[k][1]) for k in BD if k in BC}


# climatology plus the other stations' mean residual at that time, held out: an effective-index proxy (CQ-N1)
def clim_plus_mean(c):
    out = {}
    for t in times:
        here = [s for s in STS if (t, s) in obs]
        for held in here:
            rest = [s for s in here if s != held]
            if len(rest) < 3:
                continue
            dfo = sum(obs[(t, s)][0] - c[(t, s)][0] for s in rest) / len(rest)
            dm3 = sum(obs[(t, s)][2] - c[(t, s)][1] for s in rest) / len(rest)
            out[(t, held)] = (c[(t, held)][0] + dfo, c[(t, held)][1] + dm3)
    return out


CM = clim_plus_mean(clim[CBEST])
methods = [("B on D", BD), ("hybrid", HY), ("GloTEC", glo), ("B on C", BC), ("clim+mean", CM), ("clim", clim[CBEST])]
COMMON = [k for k in ALL if all(k in m for _, m in methods)]   # every method scored on the same pairs
COMMON_TEST = [k for k in COMMON if k in set(TEST)]
print(f"common pairs: {len(COMMON)} of {len(ALL)} (test days {len(COMMON_TEST)}); F10.7 30-day mean over "
      f"{min(F107_DAYS.values())} to {max(F107_DAYS.values())} observed days")
groups = [("all", lambda k: True), ("mainland US", lambda k: mainland_us(k[1])),
          ("Pacific", lambda k: k[1] in PACIFIC), ("elsewhere", lambda k: not mainland_us(k[1]) and k[1] not in PACIFIC)]


def table(title, keys):
    print(f"\n{title}: foF2 RMS / bias | MUF(3000) RMS / bias   (n)")
    for gname, sel in groups:
        ks = [k for k in keys if sel(k)]
        if not ks:
            continue
        cells = []
        for mname, m in methods:
            fo, mf = errors(m, ks)
            cells.append(f"{mname} {rms(fo):.2f}/{bias(fo):+.2f} | {rms(mf):.2f}/{bias(mf):+.2f}")
        print(f"   {gname:11s} (n={len(ks):4d})  " + "   ".join(cells))


table("2. Held out, test days (after tuning)", COMMON_TEST)
table("3. Held out, whole week", COMMON)

print("\n4. By distance to the nearest other reporting station, whole week (foF2 RMS | MUF RMS)")
nearest = {}
for k in ALL:
    others = [s for s in STS if s != k[1] and (k[0], s) in obs]
    nearest[k] = min(DIST[(k[1], s)] for s in others) if others else 1e9
for lo, hi in ((0, 500), (500, 1000), (1000, 2000), (2000, 20000)):
    ks = [k for k in COMMON if lo <= nearest[k] < hi]
    if ks:
        cells = []
        for mname, m in methods:
            fo, mf = errors(m, ks)
            cells.append(f"{mname} {rms(fo):.2f} | {rms(mf):.2f}")
        print(f"   {lo:5d}-{hi:5d} km (n={len(ks):4d})  " + "   ".join(cells))

print("\n5. By station, whole week, held out: n, B on D foF2 RMS/bias, MUF RMS/bias, clim foF2 RMS, region")
for st in STS:
    ks = [k for k in COMMON if k[1] == st]
    if not ks:
        continue
    fo, mf = errors(BD, ks); cf, cm = errors(clim[CBEST], ks)
    reg = "US" if mainland_us(st) else ("Pacific" if st in PACIFIC else "")
    print(f"   {st} ({coords[st][0]:6.1f},{coords[st][1]:7.1f}) n={len(ks):3d}  {rms(fo):.2f}/{bias(fo):+.2f}  "
          f"{rms(mf):.2f}/{bias(mf):+.2f}  clim {rms(cf):.2f}/{rms(cm):.2f}  {reg}")

# ---------- forecast ----------
C = clim[CBEST]


def forecast(h, tau, base=None):
    pred = {}
    for (t, st), (fo, m3) in (base or BD).items():
        tt = t + timedelta(hours=h)
        if (tt, st) not in C:
            continue
        w = 0.0 if tau == 0 else (1.0 if tau == math.inf else math.exp(-h / tau))
        pred[(tt, st)] = (C[(tt, st)][0] + (fo - C[(t, st)][0]) * w, C[(tt, st)][1] + (m3 - C[(t, st)][1]) * w)
    return pred


def blend(w, base=None):
    """w x (the field a day before the target time) + (1 - w) x climatology at the target time. It does not
    depend on the lead: the same prediction serves every hour ahead up to 24."""
    pred = {}
    for (t, st), v in (base or BD).items():
        tt = t + timedelta(hours=24)
        if (tt, st) in C:
            pred[(tt, st)] = (w * v[0] + (1 - w) * C[(tt, st)][0], w * v[1] + (1 - w) * C[(tt, st)][1])
    return pred


def yesterday():
    """B on D a day before the target time, as the prediction for it."""
    return {(t + timedelta(hours=24), st): v for (t, st), v in BD.items()}


bw = None
for w in (0.25, 0.5, 0.75, 1.0):
    p = blend(w)
    fo, mf = errors(p, [k for k in TUNE if k in p])
    s = rms(fo) + rms(mf) / 3.0
    if bw is None or s < bw[0]:
        bw = (s, w)
W_BLEND = bw[1]

print("\n6. Forecast, held out (foF2 RMS | MUF RMS); tau tuned on the first three days, scored on the rest")
for h in (3, 12):
    best = None
    for tau in (3, 6, 12, 24, 48, math.inf):
        p = forecast(h, tau)
        fo, mf = errors(p, [k for k in TUNE if k in p])
        s = rms(fo) + rms(mf) / 3.0
        if best is None or s < best[0]:
            best = (s, tau)
    tau = best[1]
    cands = ((f"decay tau={tau}", forecast(h, tau)), (f"hybrid decay", forecast(h, tau, HY)),
             ("persistence", forecast(h, math.inf)),
             ("yesterday", yesterday()), (f"blend w={W_BLEND}", blend(W_BLEND)),
             ("hybrid blend", blend(W_BLEND, HY)), ("climatology", forecast(h, 0)))
    common = [k for k in TEST if all(k in p for _, p in cands)]   # every method scored on the same pairs
    for label, p in cands:
        ks = common
        fo, mf = errors(p, ks)
        us = [k for k in ks if mainland_us(k[1])]
        fu, mu = errors(p, us)
        print(f"   +{h:2d} h {label:16s} n={len(ks):4d}  all {rms(fo):.2f} | {rms(mf):.2f}   mainland US (n={len(us)}) "
              f"{rms(fu):.2f} | {rms(mu):.2f}")

# ---------- by day, and quiet against disturbed ----------
print("\n7. By day, held out (foF2 RMS | MUF RMS), all stations and mainland US")
for day in sorted(by_day):
    ks = [k for k in COMMON if k[0].date() == day]
    us = [k for k in ks if mainland_us(k[1])]
    cells = []
    for mname, m in methods:
        fo, mf = errors(m, ks); fu, mu = errors(m, us)
        cells.append(f"{mname} {rms(fo):.2f}|{rms(mf):.2f} (US {rms(fu):.2f}|{rms(mu):.2f})")
    print(f"   {day:%m-%d} n={len(ks):3d}  " + "  ".join(cells))

print(f"\n8. Forecast on the test days, quiet against disturbed ({', '.join(sorted(d.isoformat() for d in bad))})")
for h in (3, 12):
    cands = ((f"decay tau={'6' if h == 3 else '12'}", forecast(h, 6 if h == 3 else 12)),
             ("blend", blend(W_BLEND)), ("hybrid blend", blend(W_BLEND, HY)), ("climatology", forecast(h, 0)))
    common = [k for k in TEST if all(k in p for _, p in cands)]
    for label, p in cands:
        for name, sel in (("quiet", lambda k: k[0].date() not in bad), ("disturbed", lambda k: k[0].date() in bad)):
            ks = [k for k in common if sel(k)]; us = [k for k in ks if mainland_us(k[1])]
            fo, mf = errors(p, ks); fu, mu = errors(p, us)
            print(f"   +{h:2d} h {label:13s} {name:9s} n={len(ks):3d}  all {rms(fo):.2f} | {rms(mf):.2f}   US (n={len(us)}) {rms(fu):.2f} | {rms(mu):.2f}")

# ---------- skill by lead: does today's departure from climatology last? ----------
print("\n9. By lead, test days, decay from now (tau tuned per lead) against climatology; US foF2 | MUF, then all")
for h in (0, 3, 6, 9, 12, 24):
    if h == 0:
        cands = (("now (hybrid)", HY), ("climatology", C))
    else:
        best = None
        for tau in (3, 6, 12, 24, 48, math.inf):
            p = forecast(h, tau, HY)
            fo, mf = errors(p, [k for k in TUNE if k in p])
            s = rms(fo) + rms(mf) / 3.0
            if best is None or s < best[0]:
                best = (s, tau)
        cands = ((f"decay tau={best[1]}", forecast(h, best[1], HY)), ("climatology", forecast(h, 0)))
    common = [k for k in TEST if all(k in p for _, p in cands) and k[0].date() not in bad]
    for label, p in cands:
        us = [k for k in common if mainland_us(k[1])]
        fo, mf = errors(p, common); fu, mu = errors(p, us)
        print(f"   +{h:2d} h quiet {label:18s} US (n={len(us):3d}) {rms(fu):.2f} | {rms(mu):.2f}   all (n={len(common):3d}) {rms(fo):.2f} | {rms(mf):.2f}")

# ---------- what a host can run: no field from yesterday unless it fetched one ----------
# B on C yesterday needs only GIRO's readings for the past 24 h (one request per station covers the range)
# and the climatology; the hybrid's yesterday needs yesterday's GloTEC grids, which the host never holds.
BANDS = (3.5, 5.3, 7.0, 10.1, 14.0, 18.1, 21.0, 24.9, 28.0)
open_set = lambda muf: frozenset(f for f in BANDS if f < muf)
print("\n10. Forecast a host can run (test days, quiet | storm), and band answers (MUF(3000) above each band)")
for h in (3, 12):
    cands = (("blend, hybrid yesterday (needs past GloTEC)", blend(W_BLEND, HY)), ("blend, B on C yesterday", blend(W_BLEND, BC)),
             ("climatology", forecast(h, 0)))
    common = [k for k in TEST if all(k in p for _, p in cands)]
    for label, p in cands:
        for name, sel in (("quiet", lambda k: k[0].date() not in bad), ("storm", lambda k: k[0].date() in bad)):
            ks = [k for k in common if sel(k)]; us = [k for k in ks if mainland_us(k[1])]
            fo, mf = errors(p, ks); fu, mu = errors(p, us)
            print(f"   +{h:2d} h {label:44s} {name:5s} n={len(ks):3d} all {rms(fo):.2f} | {rms(mf):.2f}  US {rms(fu):.2f} | {rms(mu):.2f}")
    # band answers: where the forecast and climatology disagree on which bands sit below MUF(3000), who is right?
    for label, p in cands[:2]:
        c = cands[2][1]
        diff = [k for k in common if open_set(p[k][0] * p[k][1]) != open_set(c[k][0] * c[k][1])]
        obs_set = lambda k: open_set(obs[k][1])
        f_right = sum(open_set(p[k][0] * p[k][1]) == obs_set(k) for k in diff)
        c_right = sum(open_set(c[k][0] * c[k][1]) == obs_set(k) for k in diff)
        print(f"   +{h:2d} h {label:44s} differs from climatology's bands in {len(diff)} of {len(common)} "
              f"({100 * len(diff) / len(common):.0f}%); of those, forecast right {f_right}, climatology right {c_right}, neither {len(diff) - f_right - c_right}")

# ---------- the hours ahead's typical error, by distance band (watchpost D-134) ----------
print("\n11. The hours ahead's typical error: the climatology held out on the test days, by distance to the nearest "
      "other reporting station (foF2 | MUF(3000) RMS, MHz)")
for lo, hi in ((0, 500), (500, 1000), (1000, 2000), (2000, 20000)):
    ks = [k for k in COMMON_TEST if lo <= nearest[k] < hi]
    if not ks:
        sys.exit(f"week.py: no test pairs in the {lo}-{hi} km band")
    fo, mf = errors(C, ks)
    print(f"   {lo:5d}-{hi:5d} km (n={len(ks):3d})  {rms(fo):.2f} | {rms(mf):.2f}")


# ---------- the readings' age (go-ionomaps G5: how old a held reading may be and still be assimilated) ----------
def lagged(age):
    """{(t, st): (fo, muf, m3)} from each station's sounding nearest t minus age minutes, within 10 minutes of it."""
    out = {}
    for t in times:
        want = t - timedelta(minutes=age)
        for st in STS:
            near = [r for r in sound[st] if abs(r[0] - want) <= timedelta(minutes=10)]
            if near:
                out[(t, st)] = min(near, key=lambda r: abs(r[0] - want))[1:]
    return out


def gp_aged(back, L, noise, train, k, bk):
    """One characteristic (k: 0 foF2, 2 M(3000)F2) at each held-out station at t, from the other stations' aged
    readings over the background at t: gp_loo's method, the inputs older."""
    pred = {}
    for t in times:
        for held in [s for s in STS if (t, s) in obs]:
            rest = [s for s in STS if s != held and (t, s) in train]
            if len(rest) < 3:
                continue
            res = np.array([train[(t, s)][k] - back[(t, s)][bk] for s in rest])
            mu = res.mean(); y = res - mu; var = max(float(y.var()), 1e-4)
            K = np.array([[var * math.exp(-DIST[(a, b)] / L) for b in rest] for a in rest])
            K += noise * var * np.eye(len(rest))
            kv = np.array([var * math.exp(-DIST[(held, b)] / L) for b in rest])
            pred[(t, held)] = back[(t, held)][bk] + mu + kv @ np.linalg.solve(K, y)
    return pred


print("\n12. The readings' age: the hybrid held out on the test days, the other stations' readings N minutes older than "
      "the grid (foF2 | MUF(3000) RMS, MHz), beside GloTEC alone and the climatology alone, on the same pairs")
AGES = (0, 15, 30, 45, 60, 75, 90, 120)
aged = {}
for age in AGES:
    tr = lagged(age)
    fo, m3 = gp_aged(glo, Lg, Ng, tr, 0, 0), gp_aged(C, Lc, Nc, tr, 2, 1)
    aged[age] = {k: (fo[k], m3[k]) for k in fo if k in m3}
SAME = [k for k in COMMON_TEST if all(k in aged[a] for a in AGES)]
if not SAME:
    sys.exit("week.py: no test pair is scored at every age")
gfo, gmf = errors(glo, SAME); cfo, cmf = errors(C, SAME)
print(f"   (n={len(SAME)} pairs)  GloTEC alone {rms(gfo):.2f} | {rms(gmf):.2f}   the climatology alone {rms(cfo):.2f} | {rms(cmf):.2f}")
for age in AGES:
    efo, emf = errors(aged[age], SAME)
    print(f"   {age:3d} min   hybrid {rms(efo):.2f} | {rms(emf):.2f}")
