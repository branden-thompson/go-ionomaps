---
title: "go-ionomaps — PROJECT BRIEF"
date: 2026-10-05
phase: DISCOVER (intake)
report_template: project-brief v1.1.0
level: LEVEL-1
sev: SEV-0
authority: HUM LEAD
directives: FULL GIT; FULL DOCS; FULL REPORTS; FULL DIAGRAMS; FULL RCC; FULL PLAN; FULL TDD; FULL INST
branch: feature/discover
paired_with: "watchpost 0.19.0 (its host); go-tuiMaps v0.3.0 (draws the fields)"
status: "APPROVED by the HUM LEAD 2026-10-05 (watchpost D-16) as the intake record. PS-G LOCKED (watchpost D-11), amended (D-19). Where DISCOVER's rulings changed it, requirements.md is current and wins."
---

# New Library | `go-ionomaps` (named `go-giro-data` until watchpost D-56)

> **This is the intake brief, approved at watchpost D-16.** DISCOVER changed its scope and answered its
> questions; **`requirements.md` is the current record and wins on any conflict.**

**LEVEL-1; SEV-0; FULL GIT; FULL DOCS; FULL REPORTS; FULL DIAGRAMS; FULL RCC; FULL PLAN; FULL TDD; FULL INST**

## Summary & Intent

A Go library that gives a program the ionosphere's state as fields a terminal map can draw: the
maximum usable frequency over a 3000 km path, MUF(3000), and the F2 layer's critical frequency (foF2),
over the globe, by UTC hour, with their age and their sources' terms.

**Why it exists.** The global MUF maps an HF operator reads today come from prop.kc2g.com, whose code
has no licence and whose output has no stated reuse terms. GIRO's global maps (IRTAM) and its data are
non-commercial, or paid with no right to share them. The open sources found are regional (INGV, CC BY 4.0)
or publish no MUF (NOAA GloTEC, public domain). A program that wants to show global MUF has nothing it is
free to build on and able to check (PS-G, amended at watchpost D-19).

**Its first host** is watchpost 0.19.0, whose operators' problems are PS-1 and PS-2. Its fields are drawn
by go-tuiMaps v0.3.0.

**Public, MIT** for the code (watchpost D-2). No third-party data is committed until its terms are
ruled.

**What happens if it is not built.** watchpost can show propagation only by depending on KC2G's service
without permission, or not at all.

## Problem Statement — LOCKED (watchpost D-11), amended (watchpost D-19)

> **PS-G.** "A developer who wants to show HF operators current MUF and foF2 over the whole globe
> has no source they are free to build on and able to check: the openly licensed maps published
> today are regional, or are total-electron-content products that do not publish MUF; the global MUF
> and foF2 maps are for non-commercial use or behind a paid licence; and none can be re-run end to
> end, so a program that shows them depends on someone else's service without permission, and goes
> blank or wrong without warning when that service changes."

The scorecard is in `problem-statement.md`. The wave-1 source survey found an openly licensed regional map
(INGV, CC BY 4.0) and public-domain global NmF2/hmF2 (NOAA GloTEC), and PS-G was amended to what
survives (watchpost D-19).

## Requirements

These are intake requirements. DISCOVER refines them, and watchpost's host requirements (GR-1 to GR-6,
in its brief) are folded in.

- **G-R1 — Fields.** MUF(3000) and foF2 over the globe for a UTC hour, each with its valid time and age (GR-1).
- **G-R2 — Paths.** The answer for a path between two points at an hour and band, or what a host needs to compute it (GR-2).
- **G-R3 — Fails out loud.** An input that stops, goes stale or changes format yields an error or an age, never a silently old or empty field (G-M4, GR-3).
- **G-R4 — Terms travel with the data.** Each source's name and terms are readable by the host (G-M1, GR-4). The repository's NOTICE and README state them.
- **G-R5 — The host owns the network.** No HTTP client of its own. The host supplies the fetcher (User-Agent, timeout; "allowed hosts" struck at watchpost A-12) (GR-5).
- **G-R6 — Reproducible.** The recorded inputs for an hour regenerate the same field elsewhere (G-M2).
- **G-R7 — Bounded cost.** CPU and memory per update are stated, measured and bounded (G-G1, GR-6).
- **G-R8 — A reimplementation, traceable to its sources.** Every algorithm cites the published work it implements (papers, PyIRI, ITU-R recommendations). Nothing is copied from `arodland/prop`, which has no licence.

## Metrics of Success — RULED (watchpost D-12)

Normative in `problem-statement.md`:
- G-M1 free to build on;
- G-M2 reproducible;
- G-M3 fidelity, against held-out ionosondes, with climatology the baseline (watchpost D-23; the KC2G reference removed, D-52);
- G-M4 fails out loud;
- G-G1 compute cost, the guardrail.

Targets are set from PLAN's dry run, each by its own ruling, against floors set before it (watchpost D-49, D-75).

## Technical Constraints

### GC-1 — The reference has no licence
`arodland/prop` returns no licence from the GitHub API (watchpost research, line 17). The pipeline was
read file by file for understanding during 0.18.0's research. That makes this a reimplementation from
the published science, not a clean-room one in the strict sense, where those who read the code never
write the code. The protocol that keeps it traceable was ruled at watchpost D-53 (G-R8; requirements R-7).

### GC-2 — The data
- GIRO is CC BY-NC-SA 4.0 (cite Reinisch & Galkin 2011).
- Its public FastChar endpoint returned 429 on the first probe.
- KC2G reads it through a private FTP account.
- Live probes share the HUM LEAD's IP and are budgeted.
- The MIT code licence does not relicense data. GIRO offers access "only for educational and non-commercial research purposes", which binds every copy that fetches, not only whoever redistributes (watchpost D-69, IS-5).

### GC-3 — The background model
Checked in wave 1 (watchpost `wave1-findings.md`): IRI-2020's licence grants "use, copy, and modify" but no
distribution, so its Fortran is never ported; NASA's PyIRI is MIT and is the reference ported (watchpost D-40). Its
raw CCIR/URSI tables are not shipped; NRL's refits are, behind a swappable seam (watchpost D-43).

### GC-4 — Go, beside its hosts
watchpost and go-tuiMaps are `go 1.26.10` (watchpost D-244, after GO-2026-6809; D-152 before it), and so is this module (`go.mod`). Module path:
`github.com/branden-thompson/go-ionomaps` (renamed at watchpost D-56).

### GC-5 — Gates do not exist yet
A2DH's P10 check fails closed on an empty module (no packages). The gate script, CI and the docs lane
are built in PLAN and BUILD, as go-tuiMaps' were.

## Other Considerations

- **The path is B on D** (watchpost D-40): GIRO station residuals assimilated over a background derived from
  NOAA GloTEC, with a PyIRI-port climatology as the fallback. Path A (KC2G's grid) and A′ (GIRO's IRTAM
  coefficients) were dropped at watchpost D-20; path C, the full reimplementation, is absorbed into B on D,
  whose fallback is that reimplementation's background.
- **Writing to KC2G** is on the HUM LEAD's clock (watchpost brief, Other Considerations).
- **The name.** The library was `go-giro-data` until watchpost D-54 and D-56 renamed it `go-ionomaps` before v0.1.0: two of its three inputs are not GIRO, and it publishes derived maps, never GIRO's data.
- **Standing rules:**
  - rulings one at a time, minor items as A-n rows under watchpost D-13;
  - TDD;
  - no code in PLAN;
  - no AI attribution;
  - the record moves before the gate.

## Discovery Handoff Package

**Areas to investigate**
1. The source survey that tests PS-G.
2. Path A / B / C: cost, fidelity, the licence of each block.
3. GIRO access (FastChar, DIDBase, terms, rate).
4. IRI and PyIRI licences and coefficient terms.
5. The fidelity protocol: held-out stations (the KC2G comparison was removed, watchpost D-52).
6. The fetch seam and the API shape for watchpost and go-tuiMaps.

**Stakeholders.** The HUM LEAD; watchpost (host); go-tuiMaps (renderer); KC2G; GIRO / UMass Lowell;
NASA (PyIRI).

**Risk signals**
- **RK-G1:** no-licence reference (GC-1).
- **RK-G2:** NC/SA data terms and rate limits (GC-2).
- **RK-G3:** the background model's licence (GC-3): resolved; PyIRI is MIT, IRI is not ported, NRL's refits ship behind a seam.
- **RK-G4:** a fit that grades itself (G-M3).
- **RK-G5:** compute cost (G-G1).

**Open questions**
- **OQ-G1** Path A, B or C? **Answered:** B on D (watchpost D-40).
- **OQ-G2** May any GIRO-derived output be cached or committed, and under what notice? **Answered:** the computed field is a derivative product; raw readings are never committed or cached (watchpost D-41, R-4.3).
- **OQ-G3** Pull or push? **Answered:** pull, the throttle inside the library (watchpost D-42).
- **OQ-G4** Who keeps history? **Answered:** the host (watchpost D-31, D-62); the library keeps only the last good field.

## Completeness Check

```
PROJECT BRIEF — COMPLETENESS CHECK
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  [✓] Header              — new library; paired with watchpost 0.19.0, go-tuiMaps v0.3.0
  [✓] Directives          — inherited (watchpost D-1)
  [✓] Summary / Intent    — what, why it exists, first host, cost of not building
  [✓] Problem statement   — PS-G LOCKED (watchpost D-11)
  [✓] Requirements        — G-R1..G-R8 at intake
  [✓] Metrics of Success  — ruled (watchpost D-12); targets in DISCOVER
  [✓] Tech Constraints    — GC-1..GC-5
  [✓] Considerations      — RCC paths, KC2G, the name, standing rules
  [✓] Handoff package     — 6 areas, stakeholders, RK-G1..RK-G5, OQ-G1..OQ-G4

  [✓] Rulings             — D-0..D-3 here; watchpost D-1, D-2, D-5, D-10..D-13
  [✓] Approval            — APPROVED as presented (watchpost D-16).  INTAKE CLOSED.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```
