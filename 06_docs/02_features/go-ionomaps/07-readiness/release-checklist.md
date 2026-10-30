# Release checklist — what is done before v0.1.0 is tagged

Up: [plan](../04-development/implementation-plan.md) · [API review](../03-architecture-design/api-review.md) · Carries: watchpost D-1, D-3, D-206

| Field | Value |
|---|---|
| Status | **Ready for the tag.** Sections 0 to 2 ticked with their evidence (watchpost D-344); v0.1.0 is cut from `release/v0.1.0` on the HUM LEAD's approval. Candidates `v0.1.0-rc.1` to `v0.1.0-rc.17`, the last at `37b7d42`; watchpost 0.19.0 pins the final tag (row 3.3) |
| Who runs it | The coordinator prepares each row and shows its evidence; the HUM LEAD approves the tag |
| The machine check | `scripts/gate --release`: the full gate and the release steps (row 0.5). The other rows are ticked by hand with their evidence. A release candidate, `v0.1.0-rc.N`, is not held to the rows |
| How a row is ticked | Its last cell becomes `[x]`, in the commit that holds its evidence, with the evidence named in the row |

## 0 · BUILD close-out

| # | Check | Evidence | Done |
|---|---|---|---|
| 0.1 | P10 clean: `a2dh validate` 100 %, every ledger row ratified by the HUM LEAD (watchpost D-215) | `a2dh validate` 100 % (16 of 16) and `a2dh p10 check` 0 live findings at `271f770`; its 18 ledger exemptions each RATIFIED, mirrored in `06_docs/p10-ledger.md` | [x] |
| 0.2 | Every plan task built or ruled out: G0 to G10 (G10, the instruments, by watchpost D-208; G10.5 built in REVIEW, watchpost RD-3) | Every test the plan names exists (106, at `271f770`); G9.3 names the six-point model's tests (watchpost D-316 to D-319); G10.3's release step built at SHIP (watchpost D-345, D-347) | [x] |
| 0.3 | Every A-row put for veto has been read by the HUM LEAD | A-7 to A-17 under this log's D-75; under watchpost's: A-18 to A-22 (D-223), A-23 and A-24 (D-234), A-25 to A-27 (D-240), A-29 (D-267), A-37 (3) and A-39 (D-325), A-41 (D-342), and the sixteen never batched before - A-1 to A-6, A-28, A-30 to A-38, A-40 (D-346). All stand, but A-29 (13) reversed (D-281), A-30 (1) vetoed (D-288) and A-33 (1) withdrawn (D-298), each watchpost's | [x] |
| 0.4 | Hosted CI green on the release commit, both architectures, the **full** gate dispatched as well as the quick (watchpost H-10) | The hosted full gate green on linux/amd64 and linux/arm64, run 41114829482 at `271f770`; the quick gate on every push. Dispatched again on the release commit at SHIP | [x] |
| 0.5 | `scripts/gate --release` green on the release commit: no measured constant's data over a year old (R-9.8) | `scripts/gate --release` on the release commit of `release/v0.1.0`, which carries this tick: its line in that branch's `06_docs/gate-runs.md`; green before it on a fresh clone at `271f770` (row 2.1), R-9.8's and G-G1's legs passing | [x] |

## 1 · The phases

| # | Check | Evidence | Done |
|---|---|---|---|
| 1.1 | BUILD exit: the blind red team, its findings remediated, the report approved (watchpost D-203) | watchpost D-233: BUILD exited on `e889e72a`, the report approved (`08-reports/build-report.md`) | [x] |
| 1.2 | REVIEW: the release reviewed whole, the report approved | watchpost D-322: REVIEW exited on `v0.1.0-rc.16` after ten red-team rounds, the report approved (watchpost `08-reports/review-report.md`) | [x] |
| 1.3 | VALIDATE: every tier run, no skip path; the full suite with coverage | watchpost D-341: VALIDATE exited, the report signed off (watchpost `08-reports/validate-report.md`) - the full gate and the hosted full gate on both architectures, G-M3 scored at `v0.1.0-rc.17`; the suite with coverage at `271f770`: 87 to 100 % a package (the root 96.5 %), but `internal/repocheck` (0 %, the tests' own repository checks) and `internal/terms` (50 %, a table of sources) | [x] |
| 1.4 | The README read for content, against the code as it is | Read for content against the code at VALIDATE (`01f5dbb`); its gate paragraph brought to G10.3 (`271f770`) | [x] |
| 1.5 | The release notes say what the library does, its API as reviewed (D-216), and what was measured | `release-notes.md`: what it does, its API as reviewed (D-216), what was measured - G-M1 to G-M4 and G-G1's record - its known limits and terms | [x] |

## 2 · What is in the tree

| # | Check | Evidence | Done |
|---|---|---|---|
| 2.1 | Every gate is green on a fresh clone, run outside the working tree | `scripts/gate --release` green on a fresh clone of `feature/discover` at `271f770` from GitHub, outside the working tree, with the harness's two local files copied in so P10 runs (2026-10-30); a first clone without them failed `TestTheP10MirrorMatchesTheLedger` alone, as it is built to where the harness is but its ledger is not (watchpost RC-13) | [x] |
| 2.2 | No GIRO reading, no NOAA grid and no ITU-R extract is committed (`TestNoThirdPartyDataIsCommitted`) | `TestNoThirdPartyDataIsCommitted`, green in every gate | [x] |
| 2.3 | `NOTICE` names every source and dataset (`TestTheNoticeNamesEverySource`) and carries PyIRI's licence in full | `TestTheNoticeNamesEverySource`, green in every gate; `NOTICE` carries PyIRI's MIT licence in full | [x] |
| 2.4 | No workspace file is tracked; no address or name that should not leave the tree | No workspace file tracked; every tracked file at `271f770` (231) scanned for a home directory and an e-mail address: none, but the reserved `gate@example.invalid` | [x] |
| 2.5 | `govulncheck` v1.8.0, the pinned scanner, finds nothing reachable at the floor toolchain | `govulncheck` v1.8.0 at the floor toolchain, green in the release gates at `271f770` and on the hosted full run | [x] |

## 3 · The tags, in order (after the checks)

| # | Check | Evidence | Done |
|---|---|---|---|
| 3.1 | `release/v0.1.0` squash-merged to `main` from `feature/discover` | | [ ] |
| 3.2 | `v0.1.0` at the root | | [ ] |
| 3.3 | watchpost 0.19.0 pins `v0.1.0` and its gate is green on it | | [ ] |

## Ruled exceptions to the vulnerability scan

None. A row here names a GO- id in its first cell and the ruling that accepts it.
