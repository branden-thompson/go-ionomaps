---
title: "go-ionomaps v0.1.0 — IMPLEMENTATION PLAN"
date: 2026-10-06
phase: PLAN
sev: SEV-0
authority: HUM LEAD
status: "APPROVED at the PLAN gate (watchpost D-139). No code: signatures, shapes, test descriptions, file paths and order only (watchpost 0.18.0 D-13)."
---

# Implementation plan

**Goal.** Build `requirements.md` (approved at watchpost D-84), test-first, so that watchpost 0.19.0 can pin
a release candidate in its BUILD and ship on v0.1.0 (watchpost D-3).

**Tech stack.**
- Go 1.26.9 (watchpost D-152; 1.25.13 until GO-2026-6617), no cgo, the standard library only, unless a dependency is ruled.
- PyIRI (MIT) is ported, not imported.
- A pinned `govulncheck` from the first commit (NFR-1).

**Every task follows the same order** (watchpost's):
1. Write the named test and watch it fail for the named reason.
2. Make the change.
3. Run the gate.
4. Mutation check: disable what the test protects, watch it fail, restore by copy.

## Where code lives

| Path | Holds |
|---|---|
| `ionomaps.go`, `snapshot.go`, `answers.go` | the public surface (watchpost architecture, "go-ionomaps' shape") |
| `internal/throttle/` | D-39's budget, back-off and early answers (R-5.3, R-5.4) |
| `internal/stations/` | the seed list and rotation (R-5.5); `stations.tsv`, codes and positions only |
| `internal/giro/`, `internal/glotec/`, `internal/drap/`, `internal/scales/` | requests and parsing for each source (R-3.3, R-3.4, R-8.2, R-2.10) |
| `internal/climatology/` | the PyIRI port over `Tables` (R-9.1, R-4.4) |
| `internal/climatology/tables/` | NRL's refits, converted from the plain-text export (D-114; R-4.3 lists every third-party dataset allowed) |
| `internal/magcoords/` | our own quasi-dipole coordinates and MLT, generated from IGRF-14 (D-107) |
| `internal/assimilate/` | the Gaussian process (R-9.2) |
| `internal/limits/` | P.533 path MUF, daytime absorption, D-RAP (R-2.4 to R-2.7) |
| `internal/terms/` | sources, terms and citations (R-4.1) |
| `tools/tables/` | the build-time converter: reads the committed plain-text export of the refits and the Apex sample (D-114), and IGRF-14's coefficients |
| `tools/export/` | the one-time export script (Python, run in the pinned PyIRI environment, its `pip freeze` and the source checksums recorded beside it; never run by the build) (D-114) |
| `testdata/` | synthetic inputs only; **no GIRO data** (R-4.3) |
| `scripts/gate`, `.github/workflows/gate.yml` | the gate (NFR-1), modelled on go-tuiMaps' |

## Work packages

| WP | What | Requirements | Needs |
|---|---|---|---|
| G0 | Foundations: the gate, CI, the third-party-data scan, the NOTICE test, the citation check | NFR-1, R-4.2, R-4.3, R-7.1 | — |
| G1 | Public types, terms, the fetcher contract | R-1.2, R-4.1, R-5.1 | G0 |
| G2 | Parsers: GIRO, GloTEC, D-RAP, the scales, with fuzzers and physical ranges | R-3.1 to R-3.4, R-8.2, R-2.10 | G1 |
| G3 | The climatology: the PyIRI port, the tables seam, the refits, our own magnetic coordinates (D-107) | R-9.1, R-9.7, R-4.4, R-7.3 | G1 |
| G4 | The background: GloTEC's foF2, the climatology's M(3000)F2 (D-101), the fallback named | R-9.3, R-3.2 | G2, G3 |
| G5 | Assimilation of foF2 and M(3000)F2 | R-9.2, R-9.4 | G4 |
| G6 | Limits and statuses: path MUF, absorption, disturbed | R-2.2, R-2.4 to R-2.7 | G5 |
| G7 | Answers: bands, path, reach, many points | R-2.1, R-2.3, R-2.8, R-2.9 | G6 |
| G8 | Hours ahead: the climatology's cached hours, no package of their own (Code N10a, A-30) | R-1.3 | G3 |
| G9 | Throttle, stations, update, concurrency | R-5.2 to R-5.7 | G2 |
| G10 | Reproducibility, fault injection, benchmarks, the G-M3 instrument | R-6.1, G-M2, G-M3, G-M4, R-8.1 | G7, G8, G9 |

Size (estimates, L-F14): G0 1, G1 1, G2 2, G3 3 (with IGRF and the export), G4 1, G5 1, G6 2, G7 2, G8 1, G9 2, G10 2: about 18 batches. The agent's estimate, not a measurement.

## G0 — Foundations

| # | Task | Test first |
|---|---|---|
| G0.1 | The gate: tests with `-race`, `go vet`, `gofmt -l`, fuzz smoke, P10, pinned `govulncheck`; a docs lane for Markdown-only changes | the gate fails on a planted vet error, an unformatted file and an empty change (a gate that cannot fail is refused, watchpost REFLECT L7) |
| G0.2 | No third-party data in the tree: a content scan (a FastChar header, URSI codes beside confidence columns) and an allowed list | `TestNoThirdPartyDataIsCommitted` fails on a planted FastChar reply |
| G0.3 | The NOTICE names every source in `internal/terms` and every third-party dataset committed (R-4.3: the refits, IGRF-14, the Apex sample) | `TestTheNoticeNamesEverySource` fails when a source or a committed dataset lacks its NOTICE entry |
| G0.4 | Every exported function cites its source | a citation check fails on an exported function with no "Source:" line |

## G1 — Types, terms, the fetcher

| # | Task | Shape | Test first |
|---|---|---|---|
| G1.1 | `Snapshot`, `Hour`, `Field`, `Source`, `Background` | as watchpost's architecture shows | `TestAFieldSaysWhenAndFromWhat`, `TestAGlobalFieldIsAValidTuimapsGrid`, `TestTheGridIsTwoDegrees` |
| G1.2 | `Fetcher`, `Response`, `Validators` | `Fetch(ctx, url, Validators) (Response, error)` | `TestTheLibraryOpensNoConnectionOfItsOwn` (a fetcher that counts; outside tests the module imports none of `net`, `net/http`, `crypto/tls`, `os/exec`; `net/url` alone is allowed; `Response` carries its own small header type, rate headers and validators only, I-1), `TestEverySourceCarriesItsTerms`, `TestEveryRequestGoesToAnExportedHost` |

## G2 — Parsers

| # | Task | Test first |
|---|---|---|
| G2.1 | GIRO FastChar text: header and comment lines dropped first; rows parsed; ranges checked | `TestReplyHeadersNeverLeaveTheParser`, `TestOutOfRangeReadingsAreRejectedAndCounted`, `FuzzGIROParser`, `TestErrorsNeverQuoteInput`, `TestLowConfidenceReadingsAreDropped`, `FuzzEveryInputParser`, `TestAStaleInputIsSaidStale` |
| G2.2 | GloTEC GeoJSON, typed decode | `BenchmarkGloTECDecode`, `FuzzGloTECDecode`, `TestFoF2FromNmF2` |
| G2.3 | D-RAP table | `FuzzDRAPParser`, `TestDRAPGridIsTwoByFour` (from the measured file's layout) |
| G2.4 | Station codes `^[A-Z0-9]{5}$`, through `url.Values` | `TestStationCodesAreChecked` |
| G2.5 | NOAA's scales feed: levels 0 to 5 checked, outlook days and probabilities, unset when missing or left null (D-88, A-16; built) | `FuzzEveryInputParser` (the scales parser joined it), `TestTheLevelsAndOutlookAreRead`, `TestABadFileIsRefused`, `TestTheScalesAreCarriedInTheSnapshot`, `TestMissingScalesAreUnsetNotZero` |

## G3 — The climatology

| # | Task | Test first |
|---|---|---|
| G3.1 | `Tables` and the port of PyIRI's refit evaluation (spherical harmonics in quasi-dipole latitude and magnetic local time, Fourier in time) | `TestTheFallbackMatchesPyIRI` against values produced by PyIRI 0.1.7 (MIT) at fixed inputs, committed as a small golden |
| G3.2 | The one-time export (D-114) and the refits converter (`tools/tables`, standard library only, reading the plain-text export) | `TestTheConvertedTablesRoundTrip`, `TestTheExportMatchesItsRecordedChecksums` |
| G3.3 | The F10.7 rule, the 30-day mean of NOAA's daily values, and its parser (D-104, R-9.5); the climatology takes F10.7 (R-7.3) | `TestTheClimatologyTakesF107`, `TestHighFluxFollowsPyIRI`, `TestTheF107RuleIsTheThirtyDayMean`, `TestAMissingSolarFileUsesTheLastMeanWithItsAge`, `FuzzSolarIndicesParser` |
| G3.4 | Any tables through the seam | `TestTheClimatologyRunsOnAnySuppliedTables` |
| G3.5 | Magnetic coordinates of our own (D-107): IGRF-14 evaluation, field-line tracing to the apex, quasi-dipole latitude and MLT, generated by `tools/tables`; the Apex.nc sample as oracle, its measured agreement reported in MHz of MUF(3000) beside the tolerance chosen (D-123); the end-date test | `TestIGRFMatchesItsPublishedValues` (IAGA's own check values), `TestOurCoordinatesAgreeWithApex`, `TestTheMagneticModelIsNotNearItsEnd`, `TestExtrapolatedCoordinatesAreSaid`; the table's size (binary and RSS once touched) and its lookup cost recorded against G-G1 (P-8) |

## G4 to G8 — The science

| # | Task | Test first |
|---|---|---|
| G4.1 | foF2 over GloTEC, M(3000)F2 over the climatology (D-101); climatology for foF2 when GloTEC is missing, named | `TestTheFallbackIsUsedAndNamed`, `TestM3000BackgroundIsTheClimatology` |
| G4.2 | The NOAA-only update for UAT-1 (watchpost D-151, A-4): GloTEC through its index, the solar file once a day, the background, the snapshot, D-39 for NOAA | `TestAnUpdateMakesTheFieldsFromNOAA`, `TestAnUpdateTooSoonAnswersFromTheLast`, `TestARefusalBacksOff`, `TestGridsAreAskedAtMostSixAnHour`, `TestTheNewestGridIsReadFromTheIndex` |
| G5.1 | The GP on the sphere for foF2 and M(3000)F2 residuals; kernel valid on the sphere | `TestTheKernelIsValidOnTheSphere` (positive-definite on random station sets), `TestAResidualAtAStationIsRecovered`, `TestAFieldSaysWhereItIsMeasured` |
| G5.2 | The live and typical offsets, and the no-readings correction, asked per update (D-105, R-9.6, A-30) | `TestTheLiveOffsetIsCarried`, `TestTheTypicalOffsetLearnsFromUpdates`, `TestTheNoReadingsCorrectionIsNamed`, `TestTogglingTheCorrectionMakesNoRequest` |
| G6.1 | P.533 path MUF | `TestPathMUFFollowsP533` against the recommendation's worked values, `TestEachBandsStatusForAPath`, `TestShortPathsUseFoF2` |
| G6.2 | Daytime absorption for the reference circuit | `TestDaytimeAbsorptionClosesTheLowBands` (80 m absorbed at local noon, open at night, mid-latitude) |
| G6.3 | D-RAP disturbed | `TestADisturbanceMarksTheBandsItCovers` |
| G6.4 | Statuses naming their limit | `TestEveryStatusNamesItsLimit` |
| G7.1 | `Bands`, `Path`, `Reach`, many points in one call | `TestBestBandsForAnArea`, `TestTheDaysOpenHours`, `TestAFrequencysReachIsAField`, `TestTheSkipZoneIsReturned`, `TestReadingsForManyPointsInOneCall`, `TestAReadingAtAPointComesFromTheField`, `TestAnswersCarryTheNearestStationDistance`, `FuzzAnswersNeverReturnNaN` |
| G7.2 | The host's inputs bounded (R-3.5): the answers refuse a non-finite or out-of-range origin, a radius that is not positive or above 2000 km, a frequency outside 1.8 to 30 MHz; an hour outside the snapshot returns no data; at most 64 places in one call; the answers make no fetch | `TestTheAnswersRefuseBadInput`, `FuzzAnswersNeverReturnNaN`, `TestTheAnswersFetchNothing` (a counting fetcher) |
| G8.1 | Hours ahead: the climatology's hours from the day's cache, marked typical, with the typical error and its basis (D-109) | `TestHoursAheadAreTheClimatology`, `TestAnHourAheadIsMarkedTypical`, `TestTheTypicalErrorCarriesItsBasis` |

## G9 — Throttle, stations, update

| # | Task | Test first |
|---|---|---|
| G9.1 | The burst cap, the hourly limit, 2 a minute, the back-off, `Retry-After` honoured | `TestAnUpdateNeverExceedsItsBurst`, `TestA429BacksOff`, `TestRetryAfterIsHonouredWhenSent` (a fake clock and a counting fetcher), `TestA429IsSeenByTheLibraryNotRetried`, `TestACancelledBurstSpendsTheHoursBudget`, `TestRetryAfterIsBoundedAt15Minutes`, `FuzzRateHeaders` |
| G9.2 | Early answers from the last good snapshot | `TestAnUpdateTooSoonAnswersFromTheLast` |
| G9.3 | The seed, the dark-station drop, one probe inside the 40 (D-87), rotation at 40 or more live | `TestAColdStartUsesTheSeedWithoutABurst`, `TestADarkStationIsDroppedAfterThreeDays`, `TestOneProbePerUpdate`, `TestTheProbeCountsInsideTheBurst` (40 live: 39 polled, 1 probe), `TestAboveFortyStationsRotate` |
| G9.4 | Readings only since the last held, only the characteristics used; the last reading per station held, no more (R-5.2, P-11) | `TestAnUpdateAsksOnlySinceTheLastReading`, `TestOnlyTheLastReadingPerStationIsHeld` |
| G9.5 | Concurrent callers merged; no goroutines of its own | `TestTheLibraryStartsNoGoroutines`, the race detector, `TestAHeldSnapshotIsUnchangedByTheNextUpdate` |
| G9.6 | Updates between GIRO's asks: NOAA only, the last readings re-assimilated with their age (D-94) | `TestAnUpdateBetweenGIROAsksReusesTheLastReadings`, `TestGIROIsNeverAskedMoreThanHourly` |

## G10 — Instruments

| # | Task | Test first |
|---|---|---|
| G10.1 | Reproducibility on synthetic inputs (G-M2, D-97: bit-identical on one architecture, at most 0.001 MHz across amd64 and arm64, both CI runners) | `TestAFieldIsReproducedFromItsInputs`, a cross-runner comparison of the same inputs' field |
| G10.2 | Fault injection: every input stopped, stale, truncated, reformatted (G-M4) | the fault table, each case reported, never a silent field |
| G10.3 | Cost per update (G-G1, D-98: cold open ≤ 0.5 s, refresh ≤ 50 ms, live heap ≤ 15 MB at 2°, D-112; RSS recorded); the day's quasi-dipole coordinates and climatology hours cached, not a Legendre cache (61 MB in the dry run's spike) | the gate asserts allocations and work per update (D-119); the 2° timing a release step on the reference machine, its record holding the machine model, the Go version, the commit and the figures, and the release check failing when it is missing or names another commit (Code N14); a linux/amd64 run recorded |
| G10.4 | The G-M3 instrument (target D-108): leave-one-station-out over **at least 3 days fetched in BUILD after PLAN** (D-132; `dry_fetch.py` under D-39), kept outside the tree with the PLAN evidence; failing when they are missing or a target or floor is missed; the Pacific and each distance band reported (D-116, D-129) | the Go replica agrees with `02-analysis/evidence/week.py` on PLAN's week (the replica check only), its agreement reported in MHz beside the bound chosen (as D-123 asks of the coordinates); then the fresh days are scored |
| G10.5 | The constants' table (R-9.8): each with its basis and date, carried in the snapshot (the hours ahead's typical error by distance band, D-134: under 500 km 0.77 \| 2.91, 500-1000 0.91 \| 3.24, 1000-2000 1.20 \| 4.49, over 2000 1.36 \| 4.58 MHz); the expiry test | `TestEveryConstantCarriesItsBasis`, the release step's expiry check (R-9.8) |

## Release

- v0.1.0 release candidates are tagged as G10 lands.
- watchpost pins each candidate in its BUILD.
- v0.1.0 is tagged when watchpost's REVIEW is satisfied, as go-tuiMaps v0.2.0 was.
