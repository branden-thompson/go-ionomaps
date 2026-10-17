# go-ionomaps

Ionospheric maps for Go programs: maximum usable frequency (MUF) and the F2 layer's critical
frequency (foF2), as fields a terminal map can draw, with each amateur band's status between its upper
limit (MUF, foF2) and its lower limit (the reference circuit's signal-to-noise, under D-layer
absorption).

**Status: v0.1.0, release candidate in preparation.** Built and gated with its first host, watchpost
0.19.0. The design and its record are in `06_docs/02_features/go-ionomaps/`.

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

## Using it

```go
lib, err := ionomaps.New(fetcher, ionomaps.Options{}) // fetcher: the host's HTTP client
if err != nil { ... }
_ = lib.Restore(savedState) // optional: GIRO's throttle across restarts; ErrBadState leaves it as new

snap, err := lib.Update(ctx, ionomaps.UpdateOptions{})
if errors.Is(err, ionomaps.ErrNoField) { ... } // the words say why, and when to try again
if snap.Reason == ionomaps.StationsNext {
	snap, err = lib.Update(ctx, ionomaps.UpdateOptions{}) // GIRO's burst, at once
}

bands, err := snap.Bands([]ionomaps.LatLon{{Lat: 40, Lon: -100}}, 400) // radius in km
for _, b := range bands[0].Bands {
	fmt.Println(b.Band, b.Status, b.Limit, b.LimitMHz) // e.g. "40m open foF2 7.4": the area's upper limit is foF2
}
ahead, err := snap.Ahead(6) // six hours ahead, typical, a snapshot of its own

savedState = lib.State() // keep it for the next start
credits := ionomaps.Sources() // every source, its hosts, terms and citation
```

The full API is in the package documentation (`go doc -all github.com/branden-thompson/go-ionomaps`);
its shape, as reviewed for a host developer, is in
`06_docs/02_features/go-ionomaps/03-architecture-design/api-review.md`.

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
