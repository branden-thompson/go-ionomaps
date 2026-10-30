#!/bin/bash
# samples exactly the watchpost process (its pid from the harness's pid file): utc, rss_kb, cpu time
pidfile=$1; out=$2; n=$3; : > $out
for i in $(seq 1 30); do [ -s $pidfile ] && break; sleep 1; done
pid=$(cat $pidfile)
for i in $(seq 1 $n); do
  ps -o rss=,time=,comm= -p $pid 2>/dev/null | awk -v t="$(date -u +%FT%TZ)" '{print t"\t"$1"\t"$2"\t"$3}' >> $out || break
  sleep 30
done
