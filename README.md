# go-ionomaps

Ionospheric maps for Go programs: maximum usable frequency (MUF) and the F2 layer's critical
frequency (foF2), as fields a terminal map can draw, with each amateur band's status between its upper
limit (MUF, foF2) and its lower limit (the reference circuit's signal-to-noise, under D-layer
absorption).

**Status: v0.1.0, in release candidates** (the newest tag `v0.1.0-rc.N`). Built and gated with its first
host, watchpost 0.19.0. The design and its record are in `06_docs/02_features/go-ionomaps/`.

## How it works

- **The field.** foF2 over a background derived from NOAA SWPC's GloTEC (the climatology where GloTEC
  gives nothing), M(3000)F2 over a climatology (a Go port of NASA's PyIRI method), and station readings
  from the Global Ionospheric Radio Observatory (GIRO) assimilated onto both. MUF(3000) is their
  product. 2° cells over the whole globe.
- **The answers.** For one snapshot: the readings at places, each band's status over a place's
  near-vertical area or on a path (ITU-R P.533-14's basic MUF above, its reference circuit below), the
  hours of the day each band is typically open, and a frequency's reach from an origin. They are pure
  functions of the snapshot: no fetch, safe off the UI goroutine.
- **The hours ahead** are the climatology's, labelled typical, never a forecast.
- **The host supplies the network.** The library opens no connection of its own; it asks NOAA at most
  every ten minutes and GIRO in one burst of at most 40 an hour, and never retries a refusal itself.
  Each NOAA ask has 20 s and each GIRO ask 15 s, in real time. A NOAA ask that runs out, or that the
  host's client gives up on first, stops that round and backs NOAA off. A GIRO station that gives no
  data is a miss, counted, and the burst moves on; one that misses three bursts running is asked
  every 6 hours until it gives data again. GIRO is paused only when it refuses: a 429 backs it off, a
  401 or 403 pauses it a day. Every ask leaves the update a 1 s margin after it. The whole is one
  model: [the update's timing and GIRO's stations](06_docs/02_features/go-ionomaps/03-architecture-design/update-timing-model.md).
- **An update ends inside its deadline**, which is to leave it at least `MinDeadline` (32 s); give
  each `Update` its own. No NOAA ask starts without its 20 s and the margin left: the round ends
  there, nothing backed off, and `Inputs.Unread` names that file and any the snapshot lacks - a young
  file held stands in. A GIRO burst the deadline stops carries on next hour from where it stopped.
  With no room for any ask, nothing is asked and the error wraps `ErrNoRoom`. `Inputs.StationsHeld`,
  `StationsRecent` and `NewestReading` say how many stations report and how recently, so a host can
  say when the map rests on too few. The host's `Fetch` is to return
  once its context ends; the limits rest on it.

## Known limits

The update's timing model accepts these:
- **A GIRO that stops answering** costs each hour's burst its deadline in timeouts, about three asks, and is said by the reporting counts once its newest reading is 3 hours old. When it answers again, the stations not flagged are read that hour, the flagged within 6 hours.
- **A station that answers slower than 15 s** is never read; it is flagged and asked every 6 hours.
- **With 39 or more stations not flagged**, the rotation spreads them across hours, and the flagged come back one an hour.
- **A reply with an old reading only** counts as data; the reading's age is in the reporting counts.

## Using it

```go
lib, err := ionomaps.New(fetcher, ionomaps.Options{}) // fetcher: the host's HTTP client
if err != nil { ... }
_ = lib.Restore(savedState) // optional: GIRO's throttle and the F10.7 mean across restarts; each part is taken on its own, ErrBadGIROState or ErrBadMean naming one refused

update := func() (ionomaps.Snapshot, error) { // each update its own deadline
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return lib.Update(ctx, ionomaps.UpdateOptions{})
}
snap, err := update()
if errors.Is(err, ionomaps.ErrNoField) { ... } // the words say why, and when to try again
if snap.Reason == ionomaps.StationsNext {
	snap, err = update() // GIRO's burst, at once
}

bands, err := snap.Bands([]ionomaps.LatLon{{Lat: 40, Lon: -100}}, 400) // radius in km
for _, b := range bands[0].Bands {
	fmt.Println(b.Band, b.Status, b.Limit, b.LimitMHz) // e.g. "40m open foF2 7.4": the area's upper limit is foF2
}
ahead, err := snap.Ahead(6) // six hours ahead, typical, a snapshot of its own

savedState = lib.State() // keep it for the next start
credits := ionomaps.Sources() // every source, its hosts, terms and citation
```

**Testing against recorded days.** `ionomapstest` serves days recorded as the evidence's `dry_fetch.py`
writes them - GloTEC grids, GIRO replies, NOAA's solar file - to the library at a clock, nothing dated
after it, as `net/http/httptest` serves a handler; `ionomapstest.Write` makes synthetic days for a test,
since no GIRO or NOAA file may be committed. A station not recorded answers as GIRO answers one with no
data. `Options.Answers` gives any input - `"GloTEC"`, `"solar"`, `"D-RAP"`, `"scales"` or a station's code
- a status, a body or an error in place of the recorded one, so a host's tests can drive the library into
each state it must say; `DRAPTable` and `ScalesFile` build the two files `dry_fetch.py` does not keep, so
a replay can be `Fresh`.

```go
rec, err := ionomapstest.Open(dir, solarFile)
replay := rec.Replay(ionomapstest.Options{})
lib, err := ionomaps.New(replay, ionomaps.Options{Clock: replay.Now})
replay.SetNow(t)
snap, err := lib.Update(ctx, ionomaps.UpdateOptions{})
```

The full API is in the package documentation (`go doc -all github.com/branden-thompson/go-ionomaps`);
its shape, as reviewed for a host developer, is in
`06_docs/02_features/go-ionomaps/03-architecture-design/api-review.md`.

## Working on the library

`scripts/gate` is the gate every commit passes: gofmt, vet, the tests with and without the race
detector, the nested modules' own, fuzzing, the pinned vulnerability scan at the module's floor
toolchain, cross-compiles and the mutants (`06_docs/mutants/mutants.json`, each a change its test must
catch). `--quick` skips the slow legs, `--docs` is the lane for a staged change of Markdown alone, and
`--release` adds the release steps: no measured constant over a year old. Every leg runs on the floor
toolchain `go.mod` names. P10 runs where the li-A2DH harness is set up, with its two local files
(`.a2dh.yml` and `.a2dh-p10-exemptions.yml`, whose rows are mirrored in `06_docs/p10-ledger.md`), and is
named NOT RUN elsewhere. `tools/gm3` is G-M3's instrument over recorded days (`go run ./tools/gm3 -h`).

## Licence and data terms

The code is MIT (see `LICENSE`). The licence covers the code only and does not relicense any data.
Sources and their terms are in `NOTICE`. In short:
- **GIRO's live readings are for non-commercial use.** GIRO offers them "only for educational and
  non-commercial research purposes". A commercial user needs their own arrangement with GIRO, or runs on
  the GloTEC and climatology background alone: `UpdateOptions{NoStations: true}` asks GIRO nothing.
- No GIRO data is in this repository, and none ever will be. The throttle state the library hands a
  host holds times, never a reading.
- NOAA's products are public domain; PyIRI's coefficient tables ship under its MIT licence, given in
  full in `NOTICE`.

Research for the design fetched GIRO and NOAA data under GIRO's and NOAA's terms; that data was kept outside
this repository.
