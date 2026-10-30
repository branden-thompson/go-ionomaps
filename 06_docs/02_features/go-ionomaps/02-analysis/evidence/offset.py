"""The live offset (watchpost D-105): at each grid time, the mean over reporting stations of sounding foF2 minus
GloTEC foF2, which is what the assimilation's mean term removes; and how often it leaves a 2-SD band.

Usage: offset.py DIR DSI DISTURBED   (the same arguments as week.py, which it runs first)
"""
import contextlib, io, os, runpy, statistics as stt, sys

with contextlib.redirect_stdout(io.StringIO()):
    g = runpy.run_path(os.path.join(os.path.dirname(os.path.abspath(__file__)), "week.py"), run_name="__main__")
obs, glo, times, STS = g["obs"], g["glo"], g["times"], g["STS"]
by_day = {}
for t in times:
    r = [obs[(t, s)][0] - glo[(t, s)][0] for s in STS if (t, s) in obs]
    if len(r) >= 5:
        by_day.setdefault(t.date(), []).append((t, sum(r) / len(r), len(r)))
allv = [m for d in by_day.values() for _, m, _ in d]
if not allv:
    sys.exit("offset.py: no grid time with five or more reporting stations")
mu, sd = stt.mean(allv), stt.pstdev(allv)
print(f"live offset over the week: mean {mu:+.2f} MHz, sd {sd:.2f}, min {min(allv):+.2f}, max {max(allv):+.2f}, n={len(allv)}")
for d, rows in sorted(by_day.items()):
    v = [m for _, m, _ in rows]
    print(f"  {d}  mean {stt.mean(v):+.2f}  range {min(v):+.2f} to {max(v):+.2f}  stations/time {min(n for *_, n in rows)}-{max(n for *_, n in rows)}")
flags = [(t, m) for d in by_day.values() for t, m, _ in d if abs(m - mu) > 2 * sd]
print(f"flagged at |live - typical| > 2 sd ({2 * sd:.2f} MHz): {len(flags)} of {len(allv)}")
for t, m in flags:
    print(f"  {t:%m-%d %H:%M}Z  live {m:+.2f}")
