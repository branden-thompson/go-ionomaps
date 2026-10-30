# Evidence behind watchpost D-38 to D-40

The scripts and request log behind watchpost 0.19.0's wave 2
(`watchpost/06_docs/02_features/propagation-overlays/02-analysis/wave2-findings.md`), kept so the path
ruling (D-40) can be re-run. **No GIRO or NOAA data is committed** (GIRO: CC BY-NC-SA 4.0; watchpost D-41,
this library's R-4.3). Re-running needs the data re-fetched under the throttle (watchpost D-39).

| File | What |
|---|---|
| `burst.py` | the one-sitting fetch (D-38): every station in GIRO's form list for one day, then 24 GloTEC grids; stops on the first denial and probes recovery. Needs the form page saved as `../scaled.html` |
| `burst_rest.py` | the two passes run by hand after `burst.py` stopped, which went past D-38 (the agent's error E-1) |
| `pairs.py` | ionosonde vs GloTEC-derived vs PyIRI climatology, at grid times; F10.7 = 100 by default |
| `loo.py` | path B (stations on climatology), leave-one-station-out |
| `loo_b_on_d.py` | path B on D (stations on GloTEC), leave-one-station-out, one day (DISCOVER); not PLAN's hybrid, which `week.py` scores |
| `subsets.py` | the figures `pairs.py` and `loo*.py` do not print: biases and quality subsets, B on D's MUF(3000) (3.97 and 3.22 MHz), and B on D by station |
| `dry_fetch.py` | PLAN's dry-run fetch (watchpost FR-10.6): a week of GIRO readings for the live stations in one burst, then 3-hourly GloTEC grids, one every 10 minutes (D-39); GIRO stops for the run once the full back-off is refused |
| `week.py` | PLAN's dry run, scored: climatologies (raw and refit, two F10.7 rules), GloTEC, B on D and B on C held out, by region, distance and station, with signed bias; kernels tuned on the first three days, scored on the rest; the forecast at +3 h and +12 h. Runs under PyIRI 0.1.7 (Python 3.12, watchpost D-86) |
| `plan-dry-run/` | PLAN's spikes, the G1 harness, the frame benchmark, the request logs and the WSPR query (watchpost D-124); its own README |
| `../../../../../tools/gm3` | the G-M3 instrument (G10.4, A-25): `go run ./tools/gm3 -inputs DIR -solar DSI [-from DAY] [-pairing shipped\|week]`, the library's own recipe (`internal/hybrid`) held out station by station over days read through `ionomapstest`; with `-pairing week` it agrees with `week.py` within 0.01 MHz on PLAN's test days |
| `offset.py` | the live offset (watchpost D-105): stations minus GloTEC foF2 at each grid time, and how often it leaves a 2-SD band; `offset.py DIR DSI DISTURBED`, running `week.py` first |
| `requests-2026-10-05.jsonl` | every request of the one-sitting run: time, URL, status, bytes, seconds, response headers, with the CDN's location headers (`X-Amz-Cf-Pop`, `X-Amz-Cf-Id`, `Via`) stripped (D-83); no reply bodies (GIRO's replies carry the requester's IP and are not kept) |

**Environment.** DISCOVER's scripts ran under Python 3.9 with PyIRI 0.0.4 (MIT), CCIR coefficients; PLAN's `week.py` and `offset.py` run under Homebrew Python 3.12 with PyIRI 0.1.7 (watchpost D-86); `week.py DIR DSI DISTURBED` (the storm days are required). The
coefficients scored here are PyIRI's raw CCIR tables, not the NRL refits go-ionomaps will ship (watchpost
D-43); the dry run scores those.

**Reproducing the published numbers.** `pairs.py burst-2026-10-05 100` gives 566 pairs from 28 stations;
then `loo.py 2026-10-05`, `loo_b_on_d.py 2026-10-05` and `subsets.py 2026-10-05` give every figure wave 2 and
the red team quote. Round 1's three reproducing reviewers and round 2's Docs reviewer matched them.

**Their limits, beside every quote of them:**
- one day;
- one F10.7;
- untuned kernels, chosen on the day they were scored;
- the under-500-km bin is seven European stations;
- by station, held out: the mainland-US stations 0.51-0.81 MHz (AL945, EG931, IF843, MHJ45), the Pacific stations 1.2-1.8 (EA653, LL721, WA619, GU513);
- MUF(3000) for B on D is a first score only (`subsets.py`: 3.97 MHz against GloTEC's 4.02); the dry run scores it properly.
