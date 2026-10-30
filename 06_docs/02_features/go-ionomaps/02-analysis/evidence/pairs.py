"""Three-way pairs for one wave-2 day: ionosonde vs GloTEC-derived vs PyIRI climatology.

Usage: pairs.py DIR [F107]
DIR holds the day's glotec_*.geojson and fc_*.txt (burst.py writes burst-YYYY-MM-DD/).
F107 defaults to 100: the F10.7 the published figures use (NOAA's 2026-10-05 daily flux was 97-103).
Exits 1 when it finds no grid, no station file or no pair, so an empty run never reads as a result.
foF2 = 8.98e-6 sqrt(NmF2); M(3000)F2 ~ 1490/(hmF2+176) (Shimazaki 1955, first order).
An ionosonde sounding is paired when within 10 minutes of a grid time and its CS >= 70.
"""
import bisect, glob, json, math, os, re, sys
from datetime import datetime, timedelta

import numpy as np
import PyIRI
import PyIRI.main_library as ml

D = sys.argv[1]
F107 = float(sys.argv[2]) if len(sys.argv) > 2 else 100.0
COEFF = os.path.join(os.path.dirname(PyIRI.__file__), "coefficients")


def grid(path):
    d = json.load(open(path))
    pts = {(f["geometry"]["coordinates"][0], f["geometry"]["coordinates"][1]): f["properties"] for f in d["features"]}
    lons = sorted({k[0] for k in pts}); lats = sorted({k[1] for k in pts})
    return d["time_tag"], pts, lons, lats


def bil(g, lon, lat, key):
    _, pts, lons, lats = g
    lon = ((lon + 180) % 360) - 180
    i = max(0, min(len(lons) - 2, bisect.bisect(lons, lon) - 1))
    j = max(0, min(len(lats) - 2, bisect.bisect(lats, lat) - 1))
    x0, x1, y0, y1 = lons[i], lons[i + 1], lats[j], lats[j + 1]
    tx = min(1, max(0, (lon - x0) / (x1 - x0))); ty = min(1, max(0, (lat - y0) / (y1 - y0)))
    v = lambda a, b: pts[(a, b)][key]
    q = min(pts[(a, b)]["quality_flag"] for a in (x0, x1) for b in (y0, y1))
    return v(x0, y0) * (1 - tx) * (1 - ty) + v(x1, y0) * tx * (1 - ty) + v(x0, y1) * (1 - tx) * ty + v(x1, y1) * tx * ty, q


def soundings(path):
    head = open(path).read()
    m = re.search(r"GEO \(\s*([\d.]+)\s*([NS])\s+([\d.]+)\s*E", head)
    if not m:
        return None, []
    lat = float(m.group(1)) * (1 if m.group(2) == "N" else -1)
    lon = float(m.group(3)); lon = lon - 360 if lon > 180 else lon
    rows = []
    for line in head.splitlines():
        if not line[:4].isdigit():
            continue
        p = line.split()
        try:
            t = datetime.strptime(p[0][:19], "%Y-%m-%dT%H:%M:%S")
            cs, fo, muf = int(p[1]), float(p[2]), float(p[4])
        except (ValueError, IndexError):
            continue
        rows.append((t, cs, fo, muf))
    return (lon, lat), rows


grids = [grid(p) for p in sorted(glob.glob(f"{D}/glotec_*.geojson"))]
if not grids or not glob.glob(f"{D}/fc_*.txt"):
    sys.exit(f"pairs.py: no grids or no station files under {D}")
out = []
for path in sorted(glob.glob(f"{D}/fc_*.txt")):
    st = path.split("fc_")[1][:-4]
    where, rows = soundings(path)
    if not where or not rows:
        print(f"{st}: no data"); continue
    for g in grids:
        t0 = datetime.strptime(g[0][:19], "%Y-%m-%dT%H:%M:%S")
        near = [r for r in rows if abs(r[0] - t0) <= timedelta(minutes=10) and r[1] >= 70]
        if not near:
            continue
        r = min(near, key=lambda r: abs(r[0] - t0))
        N, q = bil(g, where[0], where[1], "NmF2"); hm, _ = bil(g, where[0], where[1], "hmF2")
        gfo = 8.98e-6 * math.sqrt(N); gmuf = gfo * 1490.0 / (hm + 176.0)
        ut = t0.hour + t0.minute / 60
        F2 = ml.IRI_density_1day(t0.year, t0.month, t0.day, np.array([ut]), np.array([where[0]]), np.array([where[1]]), np.array([300.0]), F107, COEFF, 0)[0]
        cfo = float(np.array(F2["fo"]).reshape(-1)[0]); cmuf = cfo * float(np.array(F2["M3000"]).reshape(-1)[0])
        out.append((st, g[0][11:16], r[2], gfo, cfo, r[3], gmuf, cmuf, q))

print("station UT  | foF2 iono glotec clim | MUF iono glotec clim | q")
for o in out:
    print(f"{o[0]} {o[1]} | {o[2]:.2f} {o[3]:.2f} {o[4]:.2f} | {o[5]:.2f} {o[6]:.2f} {o[7]:.2f} | {o[8]}")
if not out:
    sys.exit("pairs.py: no pairs found")
if out:
    a = np.array([o[2:8] for o in out])
    rms = lambda x, y: float(np.sqrt(np.mean((x - y) ** 2)))
    print(f"n={len(out)}  foF2 RMS: glotec {rms(a[:,1],a[:,0]):.2f}  clim {rms(a[:,2],a[:,0]):.2f}  |  MUF RMS: glotec {rms(a[:,4],a[:,3]):.2f}  clim {rms(a[:,5],a[:,3]):.2f}")
    print(f"     |glotec-clim| foF2 mean {float(np.mean(abs(a[:,1]-a[:,2]))):.2f}")
