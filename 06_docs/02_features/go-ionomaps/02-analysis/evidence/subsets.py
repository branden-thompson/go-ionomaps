"""The figures wave 2 and the round-2 red team quote that pairs.py and loo*.py do not print.

Usage: subsets.py YYYY-MM-DD   (reads pairs-YYYY-MM-DD.txt and burst-YYYY-MM-DD/; run beside loo.py)

Prints:
1. foF2 RMS and signed bias for GloTEC and climatology, all pairs and by GloTEC quality flag (0-2, 3-5),
   with the share of pairs where GloTEC is closer;
2. B on D's MUF(3000), held out: its foF2 times GloTEC's M(3000)F2, and times the measured M(3000)F2;
3. B on D's foF2 RMS by station, held out.
"""
import glob, math, re, sys
from collections import defaultdict

import numpy as np

DAY = sys.argv[1]

rows = []
for l in open(f"pairs-{DAY}.txt"):
    m = re.match(r"(\w+) (\d\d):\d\d \| ([\d.]+) ([\d.]+) ([\d.]+) \| ([\d.]+) ([\d.]+) ([\d.]+) \| (\d)", l)
    if m:
        g = m.groups()
        rows.append((g[0], int(g[1])) + tuple(float(x) for x in g[2:8]) + (int(g[8]),))
if not rows:
    sys.exit("subsets.py: no pairs")

rms = lambda v: math.sqrt(sum(x * x for x in v) / len(v))


def show(label, sel):
    if not sel:
        return
    g = [r[3] - r[2] for r in sel]; c = [r[4] - r[2] for r in sel]
    closer = sum(abs(r[3] - r[2]) < abs(r[4] - r[2]) for r in sel) / len(sel)
    print(f"{label:16s} n={len(sel):3d}  foF2 RMS GloTEC {rms(g):.2f} clim {rms(c):.2f} | "
          f"bias GloTEC {sum(g)/len(g):+.2f} clim {sum(c)/len(c):+.2f} | GloTEC closer {closer:.0%}")


print("1. GloTEC and climatology, foF2")
show("all", rows)
show("quality 0-2", [r for r in rows if r[8] <= 2])
show("quality 3-5", [r for r in rows if r[8] >= 3])

# 2-3. B on D, held out: the same GP as loo_hybrid.py (L = 1000 km, noise 0.2)
coords = {}
for f in glob.glob(f"burst-{DAY}/fc_*.txt"):
    m = re.search(r"GEO \(\s*([\d.]+)\s*([NS])\s+([\d.]+)\s*E", open(f).read())
    if m:
        lat = float(m.group(1)) * (1 if m.group(2) == "N" else -1)
        lon = float(m.group(3)); lon = lon - 360 if lon > 180 else lon
        coords[f.split("fc_")[1][:5]] = (lat, lon)


def gcd(a, b):
    la1, lo1, la2, lo2 = map(math.radians, (a[0], a[1], b[0], b[1]))
    c = math.sin(la1) * math.sin(la2) + math.cos(la1) * math.cos(la2) * math.cos(lo1 - lo2)
    return 6371.0 * math.acos(max(-1.0, min(1.0, c)))


L, NOISE = 1000.0, 0.2
muf_bd_g, muf_bd_m, by_station = [], [], defaultdict(list)
for h in sorted({r[1] for r in rows}):
    hr = [r for r in rows if r[1] == h and r[0] in coords]
    for i, held in enumerate(hr):
        rest = [r for j, r in enumerate(hr) if j != i]
        if len(rest) < 3:
            continue
        res = np.array([r[2] - r[3] for r in rest]); mu = res.mean(); y = res - mu
        var = max(float(y.var()), 1e-3)
        P = [coords[r[0]] for r in rest]
        K = np.array([[var * math.exp(-gcd(a, b) / L) for b in P] for a in P]) + NOISE * np.eye(len(P))
        k = np.array([var * math.exp(-gcd(coords[held[0]], b) / L) for b in P])
        fo = held[3] + mu + k @ np.linalg.solve(K, y)        # B on D's foF2 at the held-out station
        m_glotec = held[6] / held[3]                         # GloTEC's M(3000)F2 = its MUF / its foF2
        m_meas = held[5] / held[2]                           # the ionosonde's measured M(3000)F2
        muf_bd_g.append(fo * m_glotec - held[5]); muf_bd_m.append(fo * m_meas - held[5])
        by_station[held[0]].append(fo - held[2])

print("\n2. B on D, MUF(3000), held out")
print(f"   with GloTEC's M(3000)F2: RMS {rms(muf_bd_g):.2f} MHz   (GloTEC alone {rms([r[6]-r[5] for r in rows]):.2f})")
print(f"   with the measured M(3000)F2: RMS {rms(muf_bd_m):.2f} MHz")
print("\n3. B on D, foF2 RMS by station, held out")
for st in sorted(by_station):
    print(f"   {st}  n={len(by_station[st]):2d}  {rms(by_station[st]):.2f}")
