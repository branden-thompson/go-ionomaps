"""Path B on D, prototyped offline (DISCOVER, one day): the GloTEC background plus a spatial GP of station
residuals (iono - GloTEC), scored leave-one-station-out. A copy of loo.py with the background swapped; see
loo.py for the method. Not PLAN's hybrid (watchpost D-101), which week.py scores.

Reads pairs-<day>.txt (station, hour, iono, glotec, clim) and the stations' coordinates from the fc files.
Kernel: exponential in great-circle distance (positive definite on the sphere, Gneiting 2013).
For every hour and every station with a pair, the station is held out, the residual (iono - GloTEC) is
predicted from the other stations that hour, and GloTEC + prediction is scored against the held-out reading.

Usage: loo_b_on_d.py YYYY-MM-DD   (reads pairs-YYYY-MM-DD.txt and burst-YYYY-MM-DD/fc_*.txt)
"""
import glob, math, re, sys
import numpy as np

DAY = sys.argv[1]
coords = {}
for f in glob.glob(f"burst-{DAY}/fc_*.txt"):
    m = re.search(r"GEO \(\s*([\d.]+)\s*([NS])\s+([\d.]+)\s*E", open(f).read())
    if m:
        lat = float(m.group(1)) * (1 if m.group(2) == "N" else -1)
        lon = float(m.group(3)); lon = lon - 360 if lon > 180 else lon
        coords[f.split("fc_")[1][:5]] = (lat, lon)

rows = []
for l in open(f"pairs-{DAY}.txt"):
    m = re.match(r"(\w+) (\d\d):\d\d \| ([\d.]+) ([\d.]+) ([\d.]+) \|", l)
    if m:
        rows.append((m.group(1), int(m.group(2)), float(m.group(3)), float(m.group(4)), float(m.group(5))))


def gcd(a, b):
    la1, lo1, la2, lo2 = map(math.radians, (a[0], a[1], b[0], b[1]))
    c = math.sin(la1) * math.sin(la2) + math.cos(la1) * math.cos(la2) * math.cos(lo1 - lo2)
    return 6371.0 * math.acos(max(-1.0, min(1.0, c)))


def run(L, noise):
    err_b, err_g, err_c, dist = [], [], [], []
    for h in sorted({r[1] for r in rows}):
        hr = [r for r in rows if r[1] == h and r[0] in coords]
        for i, held in enumerate(hr):
            rest = [r for j, r in enumerate(hr) if j != i]
            if len(rest) < 3:
                continue
            res = np.array([r[2] - r[3] for r in rest])
            mu = res.mean(); y = res - mu
            var = max(float(y.var()), 1e-3)
            P = [coords[r[0]] for r in rest]
            K = np.array([[var * math.exp(-gcd(a, b) / L) for b in P] for a in P]) + noise * np.eye(len(P))
            k = np.array([var * math.exp(-gcd(coords[held[0]], b) / L) for b in P])
            pred = mu + k @ np.linalg.solve(K, y)
            err_b.append(held[3] + pred - held[2]); err_g.append(held[3] - held[2]); err_c.append(held[4] - held[2])
            dist.append(min(gcd(coords[held[0]], b) for b in P))
    rms = lambda e: float(np.sqrt(np.mean(np.square(e))))
    return rms(err_b), rms(err_g), rms(err_c), len(err_b), np.array(dist), np.array(err_b), np.array(err_g), np.array(err_c)


for L in (500, 1000, 2000, 4000):
    for noise in (0.05, 0.2):
        b, g, c, n, *_ = run(L, noise)
        print(f"L={L:5d} km noise={noise:.2f}  n={n}  foF2 RMS  B-on-D {b:.2f}  GloTEC {g:.2f}  climatology {c:.2f}")

b, g, c, n, d, eb, eg, ec = run(1000, 0.2)
print("\nby distance to the nearest other station (L=1000, noise=0.2):")
for lo, hi in ((0, 500), (500, 1000), (1000, 2000), (2000, 20000)):
    sel = (d >= lo) & (d < hi)
    if sel.sum():
        r = lambda e: float(np.sqrt(np.mean(np.square(e[sel]))))
        print(f"  {lo:5d}-{hi:5d} km n={int(sel.sum()):3d}  B-on-D {r(eb):.2f}  GloTEC {r(eg):.2f}  clim {r(ec):.2f}")
