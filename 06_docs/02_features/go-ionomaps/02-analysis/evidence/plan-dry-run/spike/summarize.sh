#!/bin/sh
# median and spread (min..max) of ns/op per benchmark; B/op and allocs/op
grep '^Benchmark' "$@" | sed 's/-18 / /' | awk '{n=$1; ns[n]=ns[n]" "$3; bo[n]=$5; al[n]=$7; if(!(n in seen)){seen[n]=1; ord[++k]=n}}
END{for(i=1;i<=k;i++){n=ord[i]; m=split(ns[n],a," "); for(x=1;x<=m;x++)for(y=x+1;y<=m;y++)if(a[y]+0<a[x]+0){t=a[x];a[x]=a[y];a[y]=t}
 med=a[int((m+1)/2)]; printf "%-34s median %12.0f ns  min %12.0f  max %12.0f  spread %5.1f%%  %10s B/op %6s allocs\n", n, med, a[1], a[m], (a[m]-a[1])/med*100, bo[n], al[n]}}'
