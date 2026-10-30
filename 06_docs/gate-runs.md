# Gate runs

Every run of `scripts/gate` adds a row: when, HEAD, the git tree that was tested (this file left out), the mode, the result (and the legs that failed), the seconds it took and any override of the gate's settings. A commit made from exactly the tested tree is that tree with this file added, so a reader can match the two.

| When (UTC) | HEAD | Tree tested | Mode | Result | Seconds | Overrides |
|---|---|---|---|---|---|---|
| 2026-10-09T02:09:31Z | 329e070 | 3015b5dcb55b | full | FAILED: P10, a2dh p10 check | 33 | - |
| 2026-10-09T02:12:05Z | 329e070 | 728554f7b255 | full | green | 31 | - |
| 2026-10-09T02:25:42Z | 548f326 | 8b2afad63e03 | full | green | 34 | - |
| 2026-10-09T02:32:34Z | abed71a | 8e87e13fde83 | full | green | 65 | - |
| 2026-10-09T02:41:19Z | afceadb | 55cb7119ad66 | full | green | 81 | - |
| 2026-10-09T02:47:23Z | fbb14da | c3221c8977b0 | full | green | 111 | - |
| 2026-10-09T02:53:04Z | 7ddec8a | da97301bf26f | full | FAILED: fuzz FuzzSolarIndicesParser, 30s | 81 | - |
| 2026-10-09T02:56:28Z | 7ddec8a | 78efbfe84a4a | full | FAILED: gofmt | 110 | - |
| 2026-10-09T02:58:32Z | 7ddec8a | e7317314ceba | full | green | 112 | - |
| 2026-10-09T03:20:31Z | de4063c | 4c860598e767 | full | green | 125 | - |
| 2026-10-09T14:01:40Z | 89375d1 | 5880d3ea21aa | full | green | 125 | - |
| 2026-10-09T14:32:17Z | 029dc87 | 2c538bca8e96 | full | green | 170 | - |
| 2026-10-11T16:58:37Z | ef97f06 | 48ab09ac581a | quick | FAILED: P10, a2dh p10 check | 71 | - |
| 2026-10-11T16:59:50Z | ef97f06 | 48ab09ac581a | quick | FAILED: P10, a2dh p10 check | 68 | - |
| 2026-10-11T17:01:20Z | ef97f06 | 7a1aeed333ce | quick | green | 69 | - |
| 2026-10-11T17:06:04Z | ef97f06 | 993c2d00202b | full | green | 184 | - |
| 2026-10-11T21:59:06Z | 20c4025 | aedace24e747 | docs | green | 26 | - |
| 2026-10-12T05:08:47Z | 42dd8b1 | 35b115775483 | full | green | 188 | - |
| 2026-10-14T14:53:25Z | 8befd5b | d60f0ac2d773 | full | green | 245 | - |
| 2026-10-14T15:07:07Z | ac2efcd | 777cabd8d4b7 | full | green | 270 | - |
| 2026-10-14T17:01:32Z | db74065 | d487fb9ed3a7 | full | green | 288 | - |
| 2026-10-14T17:38:46Z | e348be1 | 6e0a7ddf7fc1 | full | green | 298 | - |
| 2026-10-14T20:19:07Z | ad6855f | 2d662dc47fec | full | green | 305 | - |
| 2026-10-14T23:26:24Z | 0d37310 | 7aaed0b1fe22 | full | green | 307 | - |
| 2026-10-14T23:49:31Z | 77c06a0 | ab316e8d5443 | full | green | 352 | - |
| 2026-10-15T00:11:37Z | 1138de8 | f4f4da5faae0 | full | green | 342 | - |
| 2026-10-15T13:29:59Z | a88f045 | 5e8d270bbada | full | green | 356 | - |
| 2026-10-15T14:00:34Z | 7a3ade5 | d497e8dec19d | full | green | 345 | - |
| 2026-10-15T14:19:02Z | b8a2ac2 | b60dbfdeb9c8 | full | green | 346 | - |
| 2026-10-16T18:27:07Z | 0ab4636 | 41f7babbdcd2 | full | green | 376 | - |
| 2026-10-16T20:34:46Z | 32931d9 | 4f72db417e24 | full | FAILED: P10, a2dh p10 check | 372 | - |
| 2026-10-16T20:42:14Z | 32931d9 | add6c2841155 | full | green | 368 | - |
| 2026-10-17T03:44:37Z | 762879a | 73d6ee266c96 | full | green | 431 | - |
| 2026-10-17T14:33:01Z | d7e7f3c | a22b639e306c | full | FAILED: P10, a2dh p10 check | 430 | - |
| 2026-10-17T15:33:20Z | d7e7f3c | 383f78dd87e4 | full | green | 444 | - |
| 2026-10-17T23:34:19Z | 1a9e5c1 | 3246231ed32c | docs | green | 75 | - |
| 2026-10-18T00:29:21Z | d68e356 | 64f81f613ba4 | quick | green | 270 | - |
| 2026-10-18T01:40:54Z | 94fb27e | 61268b58cf2e | docs | green | 54 | - |
| 2026-10-18T03:29:51Z | 660fb91 | e25d2294061f | docs | green | 53 | - |
| 2026-10-18T03:45:22Z | bb7821c | 0be56351dc05 | full | green | 464 | - |
| 2026-10-18T14:28:23Z | 2436c8a | 0fd75b35416d | full | green | 579 | - |
| 2026-10-18T15:32:10Z | 31c2e19 | ef44f04dd713 | full | green | 487 | - |
| 2026-10-19T16:55:06Z | 8f6c085 | 9c23a1610d0f | full | FAILED: tests, race detector on; tests, race detector off | 585 | - |
| 2026-10-19T17:05:32Z | 8f6c085 | 90864e42d4b4 | full | green | 572 | - |
| 2026-10-19T17:11:11Z | 73c4a94 | 7e510a0bbd2a | docs | green | 50 | - |
| 2026-10-19T18:10:17Z | 84b9a84 | 4e3eb041642a | full | green | 439 | - |
| 2026-10-19T22:11:10Z | 11685e6 | 7742fda2e00e | full | green | 502 | - |
| 2026-10-21T14:58:58Z | c84a933 | 3beb356185aa | docs | green | 58 | - |
| 2026-10-22T18:23:22Z | ed5d6bd | 7146e6f7ea96 | full | green | 524 | - |
| 2026-10-22T20:57:43Z | 3be1667 | f416bf00e4ed | full | green | 527 | - |
| 2026-10-24T17:29:16Z | d80d8db | 61773488bb5c | full | green | 490 | - |
| 2026-10-24T18:53:49Z | 00ac871 | 32d597db5a3f | full | green | 426 | - |
| 2026-10-24T21:16:16Z | 749b17a | 32d597db5a3f | docs | FAILED: the docs lane: nothing is staged;  stage the Markdown change first | 0 | - |
| 2026-10-24T21:16:21Z | 749b17a | 32d597db5a3f | docs | FAILED: the docs lane: nothing is staged;  stage the Markdown change first | 0 | - |
| 2026-10-24T21:17:22Z | 749b17a | 485ae07e99c3 | docs | green | 48 | - |
| 2026-10-24T21:20:51Z | 749b17a | 4e0694ac2bfb | docs | green | 40 | - |
| 2026-10-24T23:44:22Z | 3a7f209 | 85433032333e | full | green | 447 | - |
| 2026-10-25T04:08:37Z | 3c2da5a | 9104ff937223 | full | green | 414 | - |
| 2026-10-25T13:03:26Z | 14b38ff | 5a93fbb95657 | full | green | 461 | - |
| 2026-10-25T16:09:01Z | 8d84569 | 5a93fbb95657 | quick | green | 307 | - |
| 2026-10-25T16:15:07Z | 8d84569 | 5a93fbb95657 | quick | green | 317 | - |
| 2026-10-25T17:42:35Z | 8d84569 | 5a93fbb95657 | quick | green | 296 | - |
| 2026-10-25T21:46:00Z | 8d84569 | 30713fb86b92 | full | green | 444 | - |
| 2026-10-25T22:40:15Z | 60cd0b4 | ef46a81b5f5e | full | green | 424 | - |
| 2026-10-25T22:50:10Z | 7b1da47 | 044fc5a89dde | full | FAILED: tests, race detector on; tests, race detector off; every mutant killed by its test (watchpost RL-5); P10, a2dh p10 check | 430 | - |
| 2026-10-25T22:58:52Z | 7b1da47 | e0841a8f08ae | full | green | 418 | - |
| 2026-10-25T23:13:02Z | 57bd7ef | 58b11ff2c466 | docs | green | 42 | - |
| 2026-10-26T01:27:18Z | e2103d7 | 58b11ff2c466 | quick | green | 284 | - |
| 2026-10-26T02:26:41Z | e2103d7 | 4fa4a762e060 | full | green | 504 | - |
| 2026-10-26T04:07:24Z | 08f71ef | 4fa4a762e060 | quick | green | 294 | - |
| 2026-10-26T04:59:37Z | 08f71ef | 72f8ce321657 | full | green | 500 | - |
| 2026-10-26T13:24:51Z | 3b0ab33 | 72f8ce321657 | quick | green | 299 | - |
| 2026-10-26T14:31:45Z | 3b0ab33 | 1136cfdfc7b5 | full | green | 595 | - |
| 2026-10-26T16:34:06Z | d5d4d6c | 1136cfdfc7b5 | quick | FAILED: tests, race detector on | 291 | - |
| 2026-10-27T00:48:22Z | d5d4d6c | c6e8eee7ab9b | full | green | 611 | - |
| 2026-10-27T02:44:01Z | 4ff93ce | cf7bdf3bda4f | full | FAILED: tests, race detector on; tests, race detector off | 657 | - |
| 2026-10-27T02:54:53Z | 4ff93ce | 13b8f3304fc2 | full | green | 609 | - |
| 2026-10-27T04:00:22Z | 68f31f3 | 13b8f3304fc2 | quick | green | 307 | - |
| 2026-10-27T14:14:42Z | 68f31f3 | e46dbcca2ce4 | full | green | 753 | - |
| 2026-10-27T16:10:11Z | 59cb4a9 | e46dbcca2ce4 | quick | green | 308 | - |
| 2026-10-27T17:39:21Z | 59cb4a9 | efa784a41b69 | full | green | 1714 | - |
| 2026-10-27T21:19:02Z | f60cc5c | d5f793a7d80d | full | FAILED: tests, race detector on; tests, race detector off | 519 | - |
| 2026-10-27T21:29:07Z | f60cc5c | 7110bbf9fc22 | full | green | 527 | - |
| 2026-10-28T00:13:16Z | 5396d3f | 2a76d3fa17c6 | full | FAILED: every mutant killed by its test (watchpost RL-5) | 554 | - |
| 2026-10-28T00:23:14Z | 5396d3f | 1a80f8151cb0 | full | green | 530 | - |
| 2026-10-29T19:54:54Z | d71cf55 | b1d0c86a06ad | full | FAILED: tests, race detector on; tests, race detector off | 550 | - |
| 2026-10-29T20:08:34Z | d71cf55 | 85f0843e03d6 | full | green | 513 | - |
| 2026-10-30T01:13:59Z | 37b7d42 | ec165b6460c9 | docs | green | 53 | - |
| 2026-10-30T02:22:59Z | 01f5dbb | b1122523c655 | docs | green | 84 | - |
| 2026-10-30T19:12:21Z | 562930f | 0c3c6c1961f5 | cost | green | 7 | - |
| 2026-10-30T19:22:51Z | 562930f | c627cab548a2 | release | green | 520 | - |
| 2026-10-30T20:15:15Z | 271f770 | d405884d7a72 | docs | green | 42 | - |
| 2026-10-30T20:28:06Z | 0df7ce4 | 3d1593abc30d | docs | green | 41 | - |
