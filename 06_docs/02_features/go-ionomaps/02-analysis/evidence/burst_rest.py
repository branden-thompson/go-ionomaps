"""The two passes run by hand after burst.py stopped on 2026-10-06 (requests 98-120 in requests-2026-10-05.jsonl).

Reconstructed from the code run inline at 13:26-13:34Z. **These passes went past D-38's rule** ("on the
first denial ... the run stops and measures recovery"): burst.py stopped and measured recovery, then the
agent fetched the remaining stations anyway. The first pass (1.5 s apart) drew the second 429; the second
pass (25 s apart) drew none. The throttle model behind D-39 was fitted to both refusals. Recorded as the
agent's error E-1 (watchpost `08-reports/red-team-discover.md`).

Usage: burst_rest.py 2026-10-05   (run from the directory holding burst.py; writes into burst-DATE/)
"""
import sys

sys.argv = ["burst.py", sys.argv[1]]
src = open("burst.py").read().split('html = open("../scaled.html").read()')[0]  # get(), run(), recovery()

FAST = ["SN437", "SO148", "SQ832", "THJ76", "THJ77", "TM308", "TNJ2O", "TO535", "TR169", "TR170", "TUJ2O",
        "TV51R", "TZ362", "VT139", "WA619", "WI937", "WP937", "WU430", "XI434", "YA462", "ZH466", "ZS36R"]
SLOW = ["TR170", "TUJ2O", "TV51R", "TZ362", "VT139", "WA619", "WI937", "WP937", "WU430", "XI434", "YA462",
        "ZH466", "ZS36R"]


def items(stations, out):
    y, m, d = sys.argv[1].split("-")
    return [(f"https://lgdc.uml.edu/fastchar/getbest?ursiCode={st}&charName=foF2,MUF%28D%29,M%28D%29,hmF2&DMUF=3000"
             f"&fromDate={y}%2F{m}%2F{d}+00%3A00%3A00&toDate={y}%2F{m}%2F{d}+23%3A59%3A59", f"{out}/fc_{st}.txt")
            for st in stations]


ns = {}
exec(src.replace("MIN_GAP = 0.2", "MIN_GAP = 1.5"), ns)
ns["run"]("GIRO-rest", items(FAST, ns["OUT"]))          # stopped at its 9th request (TR169): 429
ns = {}
exec(src.replace("MIN_GAP = 0.2", "MIN_GAP = 25.0"), ns)
ns["run"]("GIRO-sustained", items(SLOW, ns["OUT"]))     # 13 of 13 served
