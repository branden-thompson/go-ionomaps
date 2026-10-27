---
title: "go-ionomaps - the update's timing and GIRO's stations: the model"
date: 2026-10-27
phase: REVIEW
authority: HUM LEAD (watchpost D-316 to D-319)
status: "APPROVED by the HUM LEAD 2026-10-27 (watchpost D-316 to D-319), replacing the model of D-312 and D-314. The one statement of these rules: a finding in this area is judged against it, and weighed by what happens, not by what a probe can construct (D-316)."
---

# The update's timing and GIRO's stations: the model

Up: [design](design.md) · [requirements, R-5.3](../01-objectives/requirements.md) · [rulings](../02-analysis/rulings.md)

## Why this page is short

Rounds 3 to 9 of watchpost 0.19.0's REVIEW red team grew this model one finding at a time - stations known to hang, a short retry, a kept probe, runs of timeouts calling GIRO down, counts of stations not reached - and rounds 7, 8 and 9 each found their Important in the previous round's own fix. The HUM LEAD's direction (watchpost D-316): the data is there, or it isn't; if a station hangs, move on; if it hangs enough, rely on it less; and when too few stations report, say so, as kc2g does with a banner. The rulings before D-316 stand as the record; this page is the model.

## The rules

1. **Each hour's burst asks the stations in turn**, within D-87's budget of 40: the stations not flagged, in rotation from where the last burst stopped, then the flagged stations due again - those that have given data before ahead of codes never heard, and among each the one asked longest ago first. One place in the budget is kept for a flagged station due, so one can always return (D-87; D-316, points 1 and 4).
2. **A station that gives no data is a miss**: a timeout, an error, a status not named, or a reply with no reading. It is counted (`Inputs.StationsFailed`), and the burst moves on to the next station. Nothing a station does stops the burst (D-316, points 2 and 3).
3. **A station that misses 3 asks in a row is flagged**: it goes to the bottom of the rotation and is asked once every 6 hours. One reading clears the flag. A station the seed names with no place, never yet heard, starts flagged (D-316, points 4 and 5; D-317).
4. **GIRO's own refusals stop the burst**: a 429 backs GIRO off by D-39's rule, and a 401 or 403 pauses it a day (D-247; D-318).
5. **An update ends inside its deadline.** Every ask is whole or none: a GIRO ask has 15 s and a NOAA ask 20 s, each with 1 s after it, and none starts without that left. A burst the deadline stops carries on next hour from where it stopped. An update needs at least `MinDeadline`, 32 s. With no room for a first ask, nothing is asked, nothing is backed off, and no hour is spent (D-298, D-301).
6. **The host's cancel stops the burst**; the hour is spent once anything was asked (D-39, D-302).
7. **Thin or stale station data is said.** The snapshot carries the readings used (`Inputs.Stations`), how many of them were taken within the last 3 hours (`Inputs.StationsRecent`), and the newest (`Inputs.NewestReading`). The host shows a notice - the map is a best approximation, and is to be relied on warily - when fewer than half the readings used are that recent, or the newest is more than 3 hours old (D-316, point 6; D-319).

## NOAA's round

The solar file at most daily; GloTEC's index, then its grid; D-RAP; the scales. A 429, or an ask run out, stops the round and backs NOAA off, 10 minutes then 15; a 5xx or another failure misses its file and the round goes on; no room for an ask stops the round with nothing backed off, `Inputs.Unread` naming what the snapshot lacks (D-260, D-270, D-285, D-298, D-304). GIRO is asked whatever NOAA answered (D-239).

## Across restarts

The host keeps the library's state: GIRO's throttle; the rotation's place; each station's misses in a row and, for a flagged one, when it was last asked; when each station last gave data. Never a reading. A code the seed no longer names is dropped (D-213).

## The host's part (watchpost)

- Each update gets its own 60 s; a deadline under `MinDeadline` is refused (D-301).
- The update is tied to the session's context and the app's; quitting cancels it before the app's 2 s wait, so the hour a burst spent is kept (D-213).
- The words: GIRO's refusal or denial as they happen; the stations that missed this hour; and rule 7's notice.

## What this model accepts

- **A GIRO that stops answering** costs each hour's burst its deadline in timeouts, about three asks, and is said by rule 7's notice once its newest reading is 3 hours old. When it answers again, the stations not flagged are read that hour, the flagged ones within 6 hours.
- **A station that answers slower than 15 s** is never read; it is flagged and asked every 6 hours.
- **With 39 or more stations not flagged**, the rotation spreads them across hours, and the flagged come back one an hour, through the place kept for them.
- **A reply with an old reading only** counts as data: the station is up; the reading's age is said by rule 7.

## How this model changes

A finding in this area is judged against this page and weighed by what happens: a case a probe can construct, that has not been seen, is a stated limit, not a new mechanism (D-316). A change to the model goes to the HUM LEAD whole.
