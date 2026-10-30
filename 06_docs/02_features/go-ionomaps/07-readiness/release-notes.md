# go-ionomaps v0.1.0 — release notes

Up: [requirements](../01-objectives/requirements.md) · [API review](../03-architecture-design/api-review.md) · [release checklist](release-checklist.md) · [G-G1's record](cost-record.md)

The first release. Built and gated with its first host, watchpost 0.19.0, and paired with go-tuiMaps
v0.3.0, which draws its fields.

## What it does

- **Two global fields at 2°**: the F2 layer's critical frequency (foF2) and the maximum usable
  frequency over 3,000 km (MUF(3000)). foF2 is NOAA SWPC's GloTEC grid, the climatology where GloTEC
  gives nothing and over the north-west tropical Pacific, with the readings of the Global Ionospheric
  Radio Observatory's (GIRO's) stations assimilated onto it; M(3000)F2 is a climatology, a Go port of
  NASA's PyIRI method, with the same stations assimilated; MUF(3000) is their product.
- **The answers, from one snapshot**: the readings at places; each amateur band's status over a place's
  near-vertical area or on a path to a place, between its upper limit (ITU-R P.533-14's basic MUF) and
  its lower limit (the reference circuit's signal-to-noise under D-layer absorption), each status naming
  the limit that set it; the hours of the day each band is typically open; and a frequency's reach from
  an origin, its skip zone with it. They are pure functions of the snapshot.
- **The hours ahead** are the climatology's, each built when a host asks for it and marked typical,
  with the typical error by distance band and its basis. Never a forecast.
- **The host supplies the network**: the library opens no connection of its own. It asks NOAA at most
  every ten minutes and GIRO in one burst of at most 40 an hour, never retries a refusal itself, and
  ends every update inside the host's deadline. The update's timing and GIRO's stations follow one
  model, [the update's timing model](../03-architecture-design/update-timing-model.md).
- **It fails out loud**: an input that stops, goes stale, is cut short or changes format is reported -
  an error, an age, or the input named unread - never a silently old or empty field.

## Its API, as reviewed

Reviewed as a host developer meets it before the first candidate ([the API review](../03-architecture-design/api-review.md), watchpost D-216).

- `New(Fetcher, Options) (*Library, error)`; `Library.Update(ctx, UpdateOptions) (Snapshot, error)`,
  each update with its own deadline of at least `MinDeadline` (32 s); `Library.State` and `Restore` keep
  GIRO's throttle and the F10.7 mean across restarts, each part taken on its own.
- `Snapshot`: `Now` (the hour's `Field`s, foF2 and MUF(3000)), `Ahead(k)`, `Readings`, `Bands`, `Path`
  and `Reach`; `Inputs` says what fed it - the GloTEC grid's time, the stations held and how recently
  they reported, what was refused, what was not read; `Reason` says why it is what it is.
- The errors a host tells apart: `ErrNoField`, `ErrNoRoom`, `ErrBadState` (with `ErrBadGIROState` or
  `ErrBadMean`).
- `Sources()`: every source the library reads, with its terms.
- `ionomapstest`: recorded and synthetic days, and a replaying `Fetcher`, for a host's own tests.

## What was measured

| Metric | Target | Measured |
|---|---|---|
| G-M1, free to build on | every source's terms recorded and tested | `Sources()` and `NOTICE`, held by `TestTheNoticeNamesEverySource`; no third-party data committed (`TestNoThirdPartyDataIsCommitted`) |
| G-M2, reproducible | bit-identical on one machine; at most 0.001 MHz apart across amd64 and arm64 | `TestAFieldIsReproducedFromItsInputs` against the committed golden, on both hosted runners |
| G-M3, fidelity (held-out RMS, MHz) | at least 15 % below the climatology in the mainland US, 10 % overall, on foF2 and MUF(3000); within 500 km of a reporting station foF2 at most 0.5, MUF(3000) at most 1.6 | at v0.1.0-rc.17, six days fetched after PLAN (2026-10-12 to 10-17, 1,490 pairs): mainland US foF2 0.46 against 0.71 and MUF(3000) 1.89 against 3.03; all stations 0.80 against 1.07 and 2.90 against 3.75; within 500 km 0.33 and 1.27 - every target met. The Pacific 1.23 against the climatology's 1.22 on foF2 (watchpost D-334) |
| G-M4, fails out loud | 100 % of injected faults reported | 100 %: `TestEveryFaultIsReported`, every input stopped, stale, cut short and reformatted |
| G-G1, cost (watchpost D-347), CPU on an Apple M5 Pro | a cold open of the hour now ≤ 0.5 s; an hour ahead ≤ 50 ms; a refresh ≤ 100 ms; live heap with 24 hours ahead held ≤ 15 MB | [the record](cost-record.md): 0.094 s; 36.6 ms; 91.0 ms; 8.1 MB; all 24 hours ahead 880 ms, peak RSS 29 MB |

## Known limits

- **A refresh costs about 90 ms of CPU**, near G-G1's 100 ms: most of it the distance from every cell to
  every station, the rest the climatology rebuilt for the new moment. A faster refresh is a follow-up
  for v0.1.1.
- **Over the Pacific, foF2 is no better than the climatology** (1.23 against 1.22 MHz held out): few
  stations, and GloTEC measured worse there, so the climatology is the background over the north-west
  tropical Pacific.
- The update's timing model accepts these, as the README says:
  - **A GIRO that stops answering** costs each hour's burst its deadline in timeouts, about three asks,
    and is said by the reporting counts once its newest reading is 3 hours old.
  - **A station that answers slower than 15 s** is never read; it is flagged and asked every 6 hours.
  - **With 39 or more stations not flagged**, the rotation spreads them across hours, and the flagged
    come back one an hour.
  - **A reply with an old reading only** counts as data; the reading's age is in the reporting counts.

## Terms

GIRO's data is under CC BY-NC-SA 4.0, for educational and non-commercial use, and a host acknowledges
each station's data provider; NOAA SWPC's products are in the public domain; the climatology is PyIRI's,
under its MIT licence, and the magnetic coordinates use IGRF-14 - each credited in `NOTICE`, PyIRI's
licence in full. The code is MIT, and the MIT licence covers the code only.
