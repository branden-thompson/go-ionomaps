# Reference-chart answer cost spike (PLAN dry run, throwaway)

## Machine

- Apple M5 Pro, 18 cores, darwin/arm64
- go1.27.1, `CGO_ENABLED=0`, standard library only
- Single goroutine. The machine was shared during the runs: load average 6 to 7 for the component runs and up to 12 for the composite runs. The spread on the composites comes from that load.

## Results

Run with `go test -bench . -benchtime=2s -count=5`. Medians of 5 runs; spread = (max − min) / median. "Before" is the straightforward first draft; "after" is the one optimization pass. Raw output is in `bench1.txt` and `bench2.txt`, summarized by `summarize.sh`.

| Benchmark | Before median | spread | B/op | allocs | After median | spread | B/op | allocs |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Reach 2° (180×91) | 5.49 ms | 4% | 1.28 MB | 16 381 | **3.47 ms** | 7% | 23 | 0 |
| Reach 1° (360×181) | 21.2 ms | 1% | 5.10 MB | 65 161 | **13.4 ms** | 4% | 387 | 0 |
| BandsArea now (50 pts × 10 bands) | 226 µs | 2% | 2 KB | 7 | **9.5 µs** | 3% | 0 | 0 |
| BandsArea now + 24 h | 5.49 ms | 7% | 2 KB | 7 | **170 µs** | 2% | 0 | 0 |
| Path 7 targets × 10 bands × 24 h | 587 µs | 5% | 127 KB | 1 680 | **36 µs** | 11% | 0 | 0 |
| Centre (reading + Bands at a point, now + 24 h) | 120 µs | 1% | 16 B | 1 | **3.7 µs** | 8% | 0 | 0 |
| **Keypress** 2°: Reach + Centre | 5.67 ms | 17% | 1.28 MB | 16 382 | **3.77 ms** | 7% | 0 | 0 |
| Keypress 1° | 23.6 ms | 42% | 5.10 MB | 65 162 | **17.0 ms** | 34% | 0 | 0 |
| **Hour step** 2°: Reach + BandsArea now + Path now + Centre | 7.14 ms | 3% | 1.29 MB | 16 459 | **4.43 ms** | 19% | 0 | 0 |
| Hour step 1° | 31.7 ms | 13% | 5.11 MB | 65 239 | **18.7 ms** | 41% | 0 | 0 |
| Grid sin/cos precompute, 2° / 1° (once per grid) | — | | | | 1.1 / 2.0 µs | | 4.6 / 9.2 KB | 4 |

Reach is the only expensive answer: about 212 ns per cell after the pass (335 before), so its cost grows with cell count (×4 from 2° to 1°). Every other answer is in the microseconds. The composites are roughly Reach plus a little. They read higher than the component sums because the machine was busier during those runs; the minimums (hour step after the pass: 3.9 ms at 2°, 16.1 ms at 1°) match the sums.

## Method

- Synthetic smooth fields: foF2 2 to 14 MHz following the sub-solar point, M(3000)F2 2.5 to 3.8, 24 hourly snapshots built before timing.
- Reach, per cell: great-circle distance (acos of a dot product); control points by slerp (one midpoint for paths up to 4000 km, two points 2000 km from each end beyond that); bilinear interpolation of both fields at each control point; solar zenith at each control point; foE, basic MUF and absorption. Skip and maximum distance are the min and max over reached cells. There is **no early exit**: every cell pays the full cost, which is the worst case.
- Bands: 50 sample points (the centre plus rings of 8, 16 and 25), judged over 4 reference distances per band. Path: 7 continent targets. Centre: a point reading plus Bands at radius 0, now and all 24 hours.
- `phys.go` holds **placeholder formulas of equivalent cost**, not the published equations. They make the same kind of transcendental calls as a P.533-style computation (pow for foE; sin plus polynomials for the basic MUF; cos, sin, atan, asin and cos for elevation and sec i; acos, cos and pow for the zenith factor; pow for the frequency factor; exp for f/foE).
- The optimization pass, in `fast.go`:
  - per-row and per-column sin/cos tables;
  - cos(zenith) as a dot product with the sun vector;
  - zero allocations (fixed arrays and a reused output field);
  - band-independent work hoisted out of the band loop.
- `TestAgreement` checks that before and after give identical outputs at both resolutions.

## What these numbers do not show

- **Synthetic fields and placeholder formulas.** The real P.533 terms may cost more or less per path, for example with more control points or E-layer modes. The cost shape matches; the constants do not.
- **No GC pressure or contention from the rest of the app.** There is no TUI render, no fetchers, and no other goroutines. The "before" version's 1.3 to 5 MB per call would add GC work in a real app; the micro-benchmark hides that.
- **Single-threaded and untuned.** Possible headroom was not measured:
  - splitting Reach by rows across cores (the work is embarrassingly parallel);
  - tabulating the distance-only terms (hop geometry) in a 1-D table;
  - early exit when the MUF already fails;
  - caching Reach by (snapshot, origin, frequency).
- Cache effects: the 1° fields (2 × 0.5 MB) fit in cache on this machine; a larger grid may not.
- These are timings from one machine under background load (load average 6 to 12), not a controlled environment.

---

# Extension: update-time compute (G-G1) and GloTEC decode (R-8.2)

Same machine and settings as above, `-benchtime=2s -count=5`, medians. Load average was about 7 during the runs. Raw output is in `bench3.txt`; the code is in `gp.go`, `glotec.go` and `gp_test.go`.

## G-G1: Gaussian-process assimilation (once per data update)

- 39 stations: 14 in Europe, 10 in North America, 5 in Asia, and 10 sparse points in the south and the Pacific.
- Synthetic residuals for both foF2 and M(3000)F2.
- Kernel: var·exp(−d/L), with d the great-circle distance and L = 4000 km. The diagonal adds noise·var, with noise = 1.0.
- One 39×39 Cholesky solve per field. The residual is then predicted at every grid cell and added to the background field.

| Benchmark | Before | B/op | allocs | After | B/op | allocs |
|---|---:|---:|---:|---:|---:|---:|
| Assimilate 2° (16 380 cells × 39 stations × 2 fields) | 39.8 ms (5%) | 287 KB | 7 | **6.0 ms** (5%) | 657* | 0 |
| Assimilate 1° (65 160 cells) | 153.5 ms (4%) | 1.07 MB | 7 | **22.6 ms** (1%) | 10 KB* | 0 |

\* Amortized from the first iteration's output-field allocation; the steady state allocates nothing.

- **Before:** a fresh matrix and fresh output fields on every call. Haversine is computed from degrees for every (cell, station) pair, separately for each field.
- **After:**
  - station unit vectors computed once;
  - distance as acos of a dot product, with cell vectors built from the grid tables;
  - one exp per (cell, station) pair, shared by both fields (the variance is folded into the weights);
  - a reusable workspace and output.
- Before and after agree to within 1e-8. Cost is about 9 ns per (cell, station) pair after the pass, dominated by acos plus exp. The Cholesky solves are negligible at this size.
- Not measured:
  - splitting rows across cores;
  - a cutoff radius (exp(−d/L) is below 1% beyond about 18 000 km, so a cutoff saves little at this L);
  - acos/exp approximations.

## R-8.2: GloTEC decode (real file, 2 502 606 bytes, 5 184 point features)

The file is read once from local disk, outside the timed loop.

| Decode | Median | spread | Throughput | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|
| Typed `encoding/json` into structs (coordinates, NmF2, hmF2, quality_flag, time_tag) | **4.87 ms** | 18% | ~514 MB/s | 740 KB | 16 |
| Generic `map[string]any` | 10.7 ms | 30% | ~233 MB/s | 7.61 MB | 191 890 |
| Ratio, generic / typed | **2.2×** | | | 10.3× | ~12 000× |

The generic decode's real cost is the allocation count, which would land on the GC in a running app. That cost is not visible in this isolated benchmark.

---

# Extension: climatology evaluation (PyIRI-shaped spherical harmonics)

Same machine, single goroutine, `-benchtime=2s -count=5`, medians. Load average was 5 to 9 during the runs. Raw output is in `bench4.txt` (before), `bench5.txt` (after) and `bench6.txt` (25 hours); the code is in `clim.go` and `clim_test.go`.

## The model's shape

The shape was confirmed by reading PyIRI's MIT `sh_library.py` (`IRI_sh_params`, `Apex`, `real_SH_func`, `real_FS_func`). The coefficients are random; none of PyIRI's data is used.

- **Coefficients:** per solar level (2) and field (foF2, M(3000)F2), 11 real Fourier terms in UT × 900 real SH terms. The SH terms run to lmax 29 and are 4π-normalized, with the Condon-Shortley phase.
- **GEO to QD mapping:** a 441-term SH evaluation (lmax 20) of QDLat, QDLon_cos and QDLon_sin. MLT angle = QD longitude − the sub-solar point's QD longitude + 180°.
- **Per hour:**
  - the 11 Fourier terms are combined once into 900 coefficients;
  - each cell needs Legendre values to l = 29 plus cos/sin(m·MLT angle), then a 900-term sum;
  - both solar levels are evaluated and blended.

## Before and after

**Before** is PyIRI-style: per cell, a fresh basis vector (900, and 441 for the mapping), cos/sin computed separately for each m, and four 900-term dot products (2 levels × 2 fields).

**After** (single goroutine):
- the QD mapping is computed once per day; it does not depend on UT, only the sub-solar QD longitude does;
- cos/sin(m·φ) by angle addition, with one Sincos per cell;
- the solar-level blend folded into the coefficients (the model is linear in them), so each cell needs 2 sums instead of 4;
- a division-free Legendre recursion;
- zero allocations.

Three cache variants were measured:
- **qdonly:** QD coordinates only. Legendre values are recomputed each hour.
- **leg64:** Legendre values P_lm(cos QD colatitude) cached per cell as float64 (465 per cell).
- **leg32:** the same, as float32.

Every variant agrees with "before" to within 6e-14 (leg32: 2e-7).

| Benchmark | 2° (180×91) | B/op | allocs | 1° (360×181) | B/op | allocs |
|---|---:|---:|---:|---:|---:|---:|
| One hour, both fields, **before** | 134 ms (7%) | 302 MB | 65 529 | 549 ms (4%) | 1.20 GB | 260 651 |
| One hour, after, qdonly | 12.7 ms (1%) | 0 | 0 | 49.9 ms (3%) | 0 | 0 |
| One hour, after, **leg64** | **6.7 ms** (8%) | 0 | 0 | **27.7 ms** (11%) | 0 | 0 |
| One hour, after, leg32 | 8.7 ms (3%) | 0 | 0 | 35.3 ms (3%) | 0 | 0 |
| Day cache build, qdonly | 13.1 ms | 0.39 MB | 4 | 51.8 ms | 1.57 MB | 4 |
| Day cache build, leg64 | 19.8 ms | **61.3 MB** | 5 | 78.7 ms | **244 MB** | 5 |
| Day cache build, leg32 | 22.3 ms | **30.9 MB** | 5 | 88.4 ms | **123 MB** | 5 |
| **First open, 25 h, before** | 3.39 s (1%) | 7.56 GB | 1.6 M | 13.4 s (6%) | 30.1 GB | 6.5 M |
| First open, 25 h, after, qdonly | 334 ms (6%) | 6.9 MB | 54 | 1.29 s (1%) | 27.8 MB | 54 |
| First open, 25 h, after, **leg64** | **191 ms** (1%) | 67.9 MB | 55 | **737 ms** (6%) | 270 MB | 55 |
| First open, 25 h, after, leg32 | 247 ms (4%) | 37.4 MB | 55 | 953 ms (7%) | 149 MB | 55 |

The first-open B/op includes the 25 hours of output fields, which are kept: 2 fields × 25 hours × 8 bytes per cell, 6.6 MB at 2° and 26 MB at 1°.

- **Cache memory:** caching the Legendre values costs 465 × 8 bytes per cell, which is 61 MB at 2° and 244 MB at 1° (half that as float32).
- **What the cache buys:** it saves about 6 ms per hour at 2° (12.7 → 6.7 ms) and about 22 ms per hour at 1°.
- **Where the hour goes:** with the cache, an hour is dominated by the two 900-term sums per cell, at about 2 × 900 multiply-adds per cell.
- **A middle option, not measured:** a qdonly cache (0.4 MB at 2°) with every hour computed in the background costs about 2× the CPU of leg64 and almost no memory.

## What these numbers do not show

- Random coefficients: the cost is the same as with real coefficients, but the values are meaningless.
- PyIRI evaluates more parameters (hmF2, B0, B1, foEs). This spike evaluates only foF2 and M(3000)F2, so each extra parameter would add about one more 900-term sum per cell.
- No SIMD and no parallelism, by rule. Splitting rows across cores would divide the time almost linearly.
- No GC pressure from the rest of the app. In a running app, a 244 MB cache at 1° would also cost memory-bandwidth headroom.
