---
title: "go-ionomaps - the update's timing and GIRO's burst: the settled model"
date: 2026-10-27
phase: REVIEW
authority: HUM LEAD (watchpost D-311)
status: "APPROVED by the HUM LEAD 2026-10-27 (watchpost D-312). The one statement of these rules: each red-team round's findings in this area are judged against it, and a change to it is presented whole, with every section it touches."
---

# The update's timing and GIRO's burst: the settled model

Up: [design](design.md) · [requirements, R-5.3](../01-objectives/requirements.md) · [rulings](../02-analysis/rulings.md)

## Why this page exists

Rounds 3 to 7 of watchpost 0.19.0's REVIEW red team ruled the update's timing and the burst's order one finding at a time (watchpost D-285 to D-310). Each fix was right for its finding, and each broke the premise of an earlier one:
- D-291's room rule cut healthy bursts short (D-295).
- D-307's tail collided with D-302's head (D-309).
- That collision manufactured D-286's runs (D-310).

The HUM LEAD's direction (watchpost D-311): pick one model, write it in the docs, and judge each later round against it, so no round reverses another in isolation. This page is that model. The rulings stand as the record of how it was reached; where this page and a ruling's wording differ, this page is the model and the ruling is history.

## The four principles, in order

Where two principles meet, the earlier one wins.

1. **Bounded.** An update never runs into its deadline: every ask starts only when the update has room for it and a margin after (watchpost D-291, D-295, D-298, D-301).
2. **Truthful about GIRO.** GIRO is called down only on evidence that GIRO itself stopped answering - never because of the host's deadline, the host's cancel, or a station already known to hang (D-247, D-286, D-296, D-305, D-310).
3. **Every answering station read.** A burst asks the stations that answer before the stations known not to, and no station is starved (D-287, D-303, D-307, D-309).
4. **Said as it is.** What the host is told - and watchpost says - names what happened: a refusal, GIRO unavailable, a station's own failure, a burst the deadline cut short. Nothing is said that did not happen (D-239, D-297, D-302).

## 1. The time budget

| Rule | Value | Ruling |
|---|---|---|
| The least deadline an update needs, which asks | `MinDeadline`, 32 s: two full GIRO asks and their margins | D-301 |
| The margin every ask leaves after it | 1 s (`deadline.Margin`) | D-301 |
| A NOAA ask | 20 s, all or none: none starts without 20 s and the margin left | D-285, D-298 |
| A GIRO ask, full | 15 s, while 16 s are left | D-275, D-295 |
| A GIRO ask, shortened | the time left less the margin, down to a 2 s floor; under it, the burst ends | D-295 |
| An ask that runs out | in real time, or by the host's client giving up first | D-285, R4L-3 |

- **No room for any ask:** nothing is asked, nothing is backed off, no GIRO hour is spent, and `Update`'s error wraps `ErrNoRoom`, naming the time it had (D-298).
- **A NOAA round with no room left** ends there, with nothing backed off. `Inputs.Unread` names the file whose ask found no room and any input the snapshot lacks; a young file held stands in silently (D-298, D-304).
- **The host's `Fetch` returns once its context ends.** Every bound above rests on it.

## 2. Evidence about GIRO

Each ask ends one way. What that end is evidence of:

| An ask's end | It is | It counts toward |
|---|---|---|
| An answer | GIRO answering | resets every run |
| A 429 | GIRO refusing | stops the burst; GIRO backed off at once (D-39) |
| A 401 or 403 | GIRO refusing this host | stops the burst; GIRO left alone a day (D-247) |
| A 5xx, 404, 410, a status not named, an error | the station's own failure | before any answer, the failures-in-a-row run; after one, nothing (D-271, D-272) |
| A full ask run out, the station not known to hang | the station's failure, and evidence against GIRO | the timeouts run (D-286, D-296) |
| A full ask run out, the station already known to hang | the station's own failure only | nothing; like any end but a counted timeout, it breaks the timeouts run (D-305, D-310) |
| A shortened ask run out | the update's end, not the station's | nothing (D-295) |
| The host's cancel | the host's | nothing (D-302) |

**Runs** are asks one after another with nothing between them. Any other end breaks a run (D-305).

**GIRO is called down** - said unavailable, backed off by D-39 - on any one of these:
- a 429;
- three failures in a row before any station has answered (D-271);
- three counted timeouts in a row, whatever answered before (D-286);
- two counted timeouts in a row as the update's deadline comes (D-296).

## 3. The order of a burst

A burst asks, in this order:

1. the probe a burst before it did not reach, which kept its turn (D-303);
2. the stations a burst before it did not reach (D-302), but not those known to hang;
3. the rotation, from its head (D-287);
4. the stations known to hang, in the order they ran out (D-307, D-309);
5. this hour's probe, one station not live, so a dark station can return (D-51, R-5.5).

A station is **known to hang** from the burst in which a full ask of it runs out, until an ask of it ends any other way: an answer, its own failure, or a refusal (D-307). It stays known to hang even when a deadline leaves it unreached (D-309).

**The rotation's head** moves past a run's last station only when that station's own ask failed (D-287). A station that ran out goes last whatever the head.

**A burst that stops** puts the stations it did not reach first next hour, the one a cancel interrupted among them (D-302), except those known to hang.

## 4. How a burst ends, and what it counts

| End | GIRO | The hour | Stations not reached (`NotReached`) | Next hour |
|---|---|---|---|---|
| Every station asked | as section 2 | spent | none | the rotation |
| The update's deadline | as section 2 | spent | the live stations it left, not the probe or those known to hang (D-302, D-303, D-309) | they go first |
| The host's cancel | untouched | spent once anything was asked | none (D-302) | they go first |
| A 429, 401 or 403 | refused, backed off | spent | none (D-302) | they go first |
| No room, or a cancel, before the first ask | untouched | not spent; the last burst's counts stand | as the last burst left it | the burst is still to make |

**Watchpost says** "GIRO's burst cut short: N stations asked first next hour" only while GIRO answered, and notes it once in the diagnostics, again only when the count changes (D-297).

## 5. NOAA's round

- **The order:** the solar file, at most daily; GloTEC's index, then its grid; D-RAP; the scales.
- **A 429, an ask run out, or no room** stops the round. A 429 and an ask run out back NOAA off: 10 minutes, then 15 (D-260, D-285). No room backs nothing off (D-298).
- **A 5xx or another failure** misses its file, and the round goes on (D-270).
- **GIRO is asked whatever NOAA answered** (D-239).

## 6. Across restarts

The host keeps the library's state. It holds:
- times;
- NOAA's flux mean;
- the stations' codes;
- the rotation's head;
- the stations left out;
- the kept probe;
- the stations known to hang.

It never holds a GIRO reading. A code the seed no longer names is dropped; the rest is taken part by part (D-213, D-288, D-302, D-307).

## 7. The host's part (watchpost)

- **Each update gets its own 60 s.** A `Deadline` under `MinDeadline` is refused (D-298, D-301).
- **The update is tied to the session's context and the app's.** Leaving the mode or quitting stops it; the app's cancel comes before its 2 s wait, so the hour a burst spent is kept (D-213, R6T-1).
- **The words** are section 4's, and section 2's "GIRO unavailable this hour" only when the library says so.

## 8. The limits this model accepts

Each one is the counter-argument its ruling weighed. Each is to be listed in VALIDATE's item 7 and the changelog's known limits.

- **GIRO dying late in a burst is called down the next hour**, not that one (D-296).
- **A struggling GIRO** that alternates hangs with server errors is never called down. Each ask fails visibly as a station's own (D-305).
- **In the hour before a station is known to hang**, two hanging stations side by side at the update's end can call GIRO down once. From the next hour they are known, and count nothing (D-305, D-310).
- **GIRO dying just as a burst reaches the stations known to hang** is not called down by their timeouts. The other stations' timeouts, or the next hour, catch it (D-310).
- **A station known to hang that recovers mid-hour** is read later in its hour (D-309).

## 9. How this model changes

- **Every red-team brief in this area points here.** A finding is judged against this page.
- **If the code breaks the model,** the code is fixed: a defect, no fork.
- **If the model is wrong,** the change goes to the HUM LEAD whole. It names every section and every rule it touches, and the rulings it supersedes. It is not a fork on one finding.
- **The rulings stay the record of how the model was reached;** this page is the model.

## 10. The code's shape, and P10 (D-306, D-311)

Where P10's invariant density fights the code's clearest shape, the clear shape stands. The package's floor is renewed, or its row ratified, at the density measured. The metric never decides where code lives or how it is written.
