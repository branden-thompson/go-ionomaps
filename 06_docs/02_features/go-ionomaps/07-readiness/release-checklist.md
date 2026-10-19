# Release checklist — what is done before v0.1.0 is tagged

Up: [plan](../04-development/implementation-plan.md) · [API review](../03-architecture-design/api-review.md) · Carries: watchpost D-1, D-3, D-206

| Field | Value |
|---|---|
| Status | **Open.** v0.1.0 is paired with watchpost 0.19.0 and go-tuiMaps v0.3.0 (watchpost D-1, D-3); it ships when watchpost 0.19.0 does. `v0.1.0-rc.1` was tagged at BUILD's exit for watchpost to pin and gate on (watchpost D-206), `v0.1.0-rc.2` for the day's climatology hours (watchpost D-222), `v0.1.0-rc.3` for the BUILD red team's second round (watchpost D-225 to D-229), and `v0.1.0-rc.4` for the instruments' seams, `internal/hybrid` and `ionomapstest` (watchpost D-237) |
| Who runs it | The coordinator prepares each row and shows its evidence; the HUM LEAD approves the tag |
| The machine check | None in v0.1.0's gate: each row is ticked by hand with its evidence. A release candidate, `v0.1.0-rc.N`, is not held to the rows |
| How a row is ticked | Its last cell becomes `[x]`, in the commit that holds its evidence, with the evidence named in the row |

## 0 · BUILD close-out

| # | Check | Evidence | Done |
|---|---|---|---|
| 0.1 | P10 clean: `a2dh validate` 100 %, every ledger row ratified by the HUM LEAD (watchpost D-215) | | [ ] |
| 0.2 | Every plan task built or ruled out: G0 to G9 (G10, the instruments, by watchpost D-208) | | [ ] |
| 0.3 | Every A-row put for veto has been read by the HUM LEAD | | [ ] |
| 0.4 | Hosted CI green on the release commit, both architectures, the **full** gate dispatched as well as the quick (watchpost H-10) | | [ ] |

## 1 · The phases

| # | Check | Evidence | Done |
|---|---|---|---|
| 1.1 | BUILD exit: the blind red team, its findings remediated, the report approved (watchpost D-203) | | [ ] |
| 1.2 | REVIEW: the release reviewed whole, the report approved | | [ ] |
| 1.3 | VALIDATE: every tier run, no skip path; the full suite with coverage | | [ ] |
| 1.4 | The README read for content, against the code as it is | | [ ] |
| 1.5 | The release notes say what the library does, its API as reviewed (D-216), and what was measured | | [ ] |

## 2 · What is in the tree

| # | Check | Evidence | Done |
|---|---|---|---|
| 2.1 | Every gate is green on a fresh clone, run outside the working tree | | [ ] |
| 2.2 | No GIRO reading, no NOAA grid and no ITU-R extract is committed (`TestNoThirdPartyDataIsCommitted`) | | [ ] |
| 2.3 | `NOTICE` names every source and dataset (`TestTheNoticeNamesEverySource`) and carries PyIRI's licence in full | | [ ] |
| 2.4 | No workspace file is tracked; no address or name that should not leave the tree | | [ ] |
| 2.5 | `govulncheck` v1.8.0, the pinned scanner, finds nothing reachable at the floor toolchain | | [ ] |

## 3 · The tags, in order (after the checks)

| # | Check | Evidence | Done |
|---|---|---|---|
| 3.1 | `release/v0.1.0` squash-merged to `main` from `feature/discover` | | [ ] |
| 3.2 | `v0.1.0` at the root | | [ ] |
| 3.3 | watchpost 0.19.0 pins `v0.1.0` and its gate is green on it | | [ ] |

## Ruled exceptions to the vulnerability scan

None. A row here names a GO- id in its first cell and the ruling that accepts it.
