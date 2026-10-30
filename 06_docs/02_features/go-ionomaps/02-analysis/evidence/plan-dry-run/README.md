# PLAN dry run: the instruments (watchpost 0.19.0, D-124)

The scripts, spikes and logs behind watchpost's `07-readiness/dry-run.md` and the rulings set from it
(D-91, D-95 to D-98, D-111's starting figures, A-27). **No data is committed:** GIRO's readings
(CC BY-NC-SA; replies carry the requester's IP), NOAA's grids and the ITU-R/VOACAP extracts stay outside the
tree; the re-fetch is `../dry_fetch.py` under the throttle (D-39).

| Path | What | Behind |
|---|---|---|
| `spike/` | Throwaway Go (its own module, standard library, `CGO_ENABLED=0`): the answers' cost per keypress, the update's assimilation, GloTEC's typed decode, the climatology's evaluation. **Placeholder physics of equivalent cost**: it measures cost, not the method. `RESULT.md` and `bench*.txt` hold the runs. The decode test reads one grid from `GLOTEC_FILE`, and skips without it | FR-1.17, A-27, D-98, R-8.2 |
| `tmbench/` | go-tuiMaps v0.2.0's whole-globe frame benchmark (`bench.txt`, `base.txt`). It needs go-tuiMaps v0.2.0, from the module proxy or a local `go.work` | D-96 |
| `g1/` | The G1 harness: `run.exp` (expect, under `sandbox-exec` with the network denied, ending by signal) and `sample.sh` (`ps` every 30 s), with the 10-minute dry run's samples | G1, D-95 |
| `logs/requests.jsonl` | Every request of the week's fetch: time, URL, status, bytes, seconds, rate headers only | NFR-3, D-39 |
| `logs/requests.log` | The other PLAN requests (D-RAP's page, the scales, the solar and geomagnetic indices, wspr.live) | NFR-3 |
| `logs/w00-requests.log` | BUILD's W0.0: three gzip-asked requests (the GloTEC index, a grid, D-RAP) and their size, encoding and validator headers | watchpost D-111, D-141 |
| `logs/wspr_nvis13.sql` | The near-vertical density query at +13 dB, ground wave removed | D-91 |

Every Go file is headed as a placeholder; nothing here is imported by the library.
