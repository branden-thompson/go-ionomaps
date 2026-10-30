---
title: "go-ionomaps — problem statement"
date: 2026-10-05
phase: DISCOVER (intake)
sev: SEV-0
authority: HUM LEAD
status: "LOCKED (watchpost D-11), amended (watchpost D-19); metrics ruled (watchpost D-12), targets set from PLAN's dry run (watchpost D-49)"
---

# Problem statement

go-ionomaps (named go-giro-data until watchpost D-56) has a statement of its own (watchpost D-10), beside the host requirements watchpost's
DISCOVER writes for it. Its host's statements are PS-1 and PS-2, in
`watchpost/06_docs/02_features/propagation-overlays/01-objectives/problem-statement.md`.

## PS-G (amended, watchpost D-19)

> A developer who wants to show HF operators current MUF and foF2 over the whole globe has no source
> they are free to build on and able to check: the openly licensed maps published today are
> regional, or are total-electron-content products that do not publish MUF; the global MUF and foF2
> maps are for non-commercial use or behind a paid licence; and none can be re-run end to end, so a
> program that shows them depends on someone else's service without permission, and goes blank or
> wrong without warning when that service changes.

| Criterion | |
|---|---|
| Bad outcome | depends on a service without permission; blank or wrong without warning |
| Affected humans | the developer; the HF operators who read the result, second |
| Tech agnostic | no technology named |
| Non-prescriptive | permission from KC2G, or a licensed source found in DISCOVER, answers it as well as our own computation |
| Verifiable | yes; the wave-1 source survey (watchpost `propagation-overlays/02-analysis/wave1-findings.md`) found INGV's Europe map CC BY 4.0 and NOAA GloTEC's global NmF2/hmF2 public domain, and the statement was amended to what survives (watchpost D-19) |

## Metrics (watchpost D-12)

Targets are set from PLAN's dry run, each by its own ruling (watchpost D-49).

| # | Name · symbol | Type | Definition | Measured in |
|---|---|---|---|---|
| G-M1 | Free to build on · F | compliance | every input the library reads, and every output it publishes, has recorded terms that permit the use; a test holds the list of sources and their terms. **Scored per layer (watchpost D-69):** the code (MIT) and the computed field (a derivative product, watchpost D-41) are free to build on; the live GIRO input is non-commercial, so a commercial user arranges their own access or runs on the GloTEC and climatology background alone | sources without permitting terms for the stated use (target 0) |
| G-M2 | Reproducible · P | reproducibility | from the recorded inputs for one hour, another machine regenerates the same field | largest difference, MHz |
| G-M3 | Fidelity · Φ | accuracy | foF2 and MUF(3000) against ionosonde readings held out of the fit (leave-one-out), with the IRI climatology as the baseline to beat (watchpost D-23); the KC2G reference was removed (watchpost D-52) | RMS error, MHz |
| G-M4 | Fails out loud · W | honesty | when an input stops, goes stale or changes format, the library returns an error or an age, never a silently old or empty field | share of injected faults reported (target 100%) |
| G-G1 | Compute cost · K | guardrail (watchpost issue #25) | CPU seconds and peak memory for one map update on a reference machine | CPU s; MB |

Anti-solution check:
- G-M1: KC2G's maps used with written permission pass, so the metric does not force a reimplementation.
- G-M2: PS-G's "able to check". A service nobody else can rerun fails.
- G-M3: a climatology that never changes fails. Held-out readings keep the fit from grading itself.
- G-M4: PS-G's "blank or wrong without warning".

## Targets (watchpost D-49)

Each ruled from PLAN's dry run, against watchpost D-75's floors, and listed here as it is ruled.

| Metric | Target | Ruling |
|---|---|---|
| G-M1 | every source's terms recorded and tested (pass or fail) | watchpost D-12, D-69 |
| G-M2 | bit-identical on the same OS and architecture; at most 0.001 MHz apart across amd64 and arm64 | watchpost D-97 |
| G-M4 | 100% of injected faults reported | watchpost D-12 |
| G-G1 | a cold open of the hour now at 2° ≤ 0.5 s CPU; each hour ahead, built when asked, ≤ 50 ms; a refresh ≤ 100 ms; live heap with 24 hours ahead held ≤ 15 MB (Apple M5 Pro) | watchpost D-98, D-112, D-347 |
| G-M3 | held-out RMS ≥ 15% below the climatology's in the mainland US and ≥ 10% below overall, on foF2 and MUF(3000); within 500 km of a reporting station foF2 ≤ 0.5 MHz, MUF(3000) ≤ 1.6 MHz; on ≥ 3 days fetched in BUILD after PLAN, G10.4 | watchpost D-108, D-132 |
| G-M3 (reported, not gated) | the Pacific stations and each distance band (under 500, 500-1000, 1000-2000, over 2000 km) reported separately | watchpost D-116 |

