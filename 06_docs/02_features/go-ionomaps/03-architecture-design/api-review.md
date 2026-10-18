# go-ionomaps v0.1.0's public API, reviewed for a host developer (watchpost D-216)

Up: [design](design.md) · [rulings](../02-analysis/rulings.md)

**What this is.** Before v0.1.0-rc.1 is tagged (watchpost D-206), the public API was read as a host
developer meets it: the names, the shape of each call, and how easily a host invokes it. Every change
is made before the tag, each listed in the table below and in the build log.

## Made

| Before | After | Why |
|---|---|---|
| `Snapshot.Hours []Hour`, documented as up to 25 hours, always one | `Snapshot.Now Hour` | a field that always holds one value should not be a slice a host indexes (`Hours[0]`); the documentation was false (BUILD red team C-11) |
| `Snapshot.Ahead(k) (Hour, error)`, a bare hour a host splices into the snapshot | `Snapshot.Ahead(k) (Snapshot, error)`: the hour k ahead as a snapshot of its own, typical, carrying no live disturbance or offset | the answers are methods on a snapshot, so an hour ahead must be one; the splice carried today's D-RAP disturbance into a typical hour (L-F2) |
| `At`, `Bands`, `Path`, `Reach` each take `at time.Time` and pick the hour that holds it | each answers for its snapshot's hour: `s.Readings(places)`, `s.Bands(origins, radiusKm)`, `s.Path(from, to)`, `s.Reach(origin, mhz)` | a snapshot is one hour now, so the moment is redundant, and its window was a trap (L-F14: one minute before `Computed` was refused). The sun is now the snapshot's own moment (`Now.At`), not the host's clock: an answer reads the fields and the sun of the same moment, and the host already says the snapshot's age and when it is stale (watchpost FR-5.1) |
| `Snapshot.At(points, at)` | `Snapshot.Readings(places)` | "At" read as a time, not as "the readings at these places" |
| `Snapshot.Early Reason` | `Snapshot.Reason Reason` | `s.Reason == ionomaps.TooSoon` reads as what it is |
| `(*Library).Sources()`, which ignored its receiver | `ionomaps.Sources()` | static data reached through an instance made hosts build a library to read a list (C-5) |
| `Band`, `Status`, `Limit` with no names | `String()` on each ("20m", "open", "MUF") | a host keys its rows by band, not by position in a slice, and logs read (L-F8) |
| `Inputs.Refused`, which listed NOAA files fetched but unreadable, nothing refused | `Inputs.Unread`, the meaning unchanged | the name said a refusal where there was none (watchpost round 2 CN2-15, D-229; v0.1.0-rc.3) |
| `Bitset.Set`, exported on an immutable snapshot | unexported; `Field.WithNoData(cells ...int) Field` returns a copy with those cells marked | a host could write into a snapshot documented as immutable (C-20); a host's tests still need no-data cells (L-F11) |
| errors as text only; a stopped update said "too soon" | `ErrNoField`, wrapped, whenever there is no field to give, with the context's own error wrapped beside it when a deadline or cancel stopped the update; the reason `Unfinished` for a snapshot built from what is held after a stop | a host tells a slow link (`context.DeadlineExceeded`) from a closed mode (`context.Canceled`) and from NOAA's refusal without reading text; a library sentinel per cause would have duplicated the context's own (L-F4) |
| - | `UpdateOptions.NoStations`, and `Inputs.NoStations` saying so | GIRO never asked and no reading on the field, for a host GIRO's terms do not cover (watchpost D-214) |
| an hour ahead at the snapshot's moment plus k hours, rebuilt at every ask | the climatology at the start of that UTC hour (`Hour.At` the hour's start), kept by the library and shared by its snapshots | R-1.3 says the hours ahead are the day's cache's hours; at the snapshot's moment no two snapshots shared one, so every ask rebuilt it, about 35 ms (watchpost D-222, P-4, A-23) |
| - | `(*Library).State() []byte` and `(*Library).Restore([]byte) error` | GIRO's throttle state kept across a host's restarts (watchpost D-213). `Restore` is its own call, not an option to `New`, so a state file that fails its checks (`ErrBadState`) never stops a host from starting: the library is left as new and the host carries on |
| `ReachField` under the upper limit only, its `Limit` always the MUF | a cell reached only between the limits, as the band statuses judge it; `Limit` the limit that bounds the reach - the MUF where the lower limit closes no cell under it, else the lower limit that closes the more | the reach said "no skip zone" for a band the chart called absorbed (watchpost D-205) |
| NOAA's unreadable files passed over in silence; GloTEC's gaps uncounted; `GIROPaused` never said; `Rejected` counted since `New` | `Inputs.Refused` names each file not read, `Inputs.GloTECFilled` counts the cells the climatology filled, `GIROPaused` is said while GIRO backs off, and `Rejected` is the last burst's | a host says what the field is made of and why it is not new, from fields rather than guesses (L-F5, L-F6) |
| `TypicalError.Basis` and `Offset.Basis`, a developer's citation a host read aloud | `Basis` in words a listener can be told, `Source` the citation | watchpost said "PLAN's week (watchpost D-134 ...)" with every hour ahead (AX-3) |
| `Field.CellCentre` | removed | no consumer; the box and the cell counts say it (C-20) |
