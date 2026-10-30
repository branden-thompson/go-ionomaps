# G-G1's record: the cost of an update on the reference machine

Written by `scripts/gate --cost` (plan task G10.3, watchpost D-345), each figure the median of 5 runs from
the library's test service: a cold open is a new library, NOAA's update and GIRO's burst assimilated, the
hour now; each of the 24 hours ahead is then built, as a host asks for them; a refresh is an update ten
minutes on with a newer grid (watchpost D-347). `scripts/gate --release` refuses this record when it is
missing, names other code or another machine, or misses a target.

| Field | Value |
|---|---|
| Code tree | `d9167e4834d4` |
| Machine | Apple M5 Pro |
| Go | go1.26.10 darwin/arm64 |
| Measured | 2026-10-30T19:12:21Z |
| Cold open, the hour now at 2°, CPU s | 0.094 (target 0.5) |
| An hour ahead, CPU ms | 36.638 (target 50) |
| 24 hours ahead, CPU ms | 879.980 |
| Refresh, CPU ms | 90.950 (target 100) |
| Live heap, 24 hours ahead held, MB | 8.052 (target 15) |
| Peak RSS, MB | 29.031 |
