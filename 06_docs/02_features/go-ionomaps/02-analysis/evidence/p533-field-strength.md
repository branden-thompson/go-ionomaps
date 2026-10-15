# ITU-R P.533-14 §4 to §7: the field strength and S/N the lower limit needs, read from the recommendation

**Why:** G6.2b computes "absorbed" for the reference circuit (watchpost D-73, D-91, D-186 to D-189): a band is
absorbed where the circuit's median S/N falls short of +13 dB. The text extraction used in G6.2a lost every
equation image; these are read from the recommendation itself, as `p533-equations.md` read §3.

**How they were read (2026-10-14):** P.533-14's Word edition (ITU, free to read, fetched again for this) carries
some equations as text and the rest as embedded Equation Editor objects whose WMF previews keep each glyph as a
text record with its position; the glyphs were read by position and the equations rebuilt (minus signs drawn in
the Symbol font do not survive as text, so each was restored by the term it joins and checked against the
standard forms it cites). ITU's text and tables are not reproduced; the equations are the method.

| # | Equation | Note |
|---|---|---|
| (11) | fs = 1.05 foE sec i | the E layer's maximum screening frequency for an F2 mode; paths up to 4000 km; foE at the midpoint (to 2000 km) or the higher of the two points 1000 km from each end |
| (12) | i = arcsin(R0 cos ΔF / (R0 + hr)) | i at hr = 110 km; ΔF the F2 mode's elevation from (13); R0 = 6371 km |
| (13) | Δ = arctan(cot(d / 2R0) − (R0 / (R0 + hr)) cosec(d / 2R0)) | d the hop, D / n; hr 110 km for E modes, (14) to (16) for F2 |
| — | H = 1490 / (M(3000)F2 + ΔM) − 316; ΔM = 0.18 / (y − 1.4) + 0.096 (R12 − 25) / 150 | x = foF2 / foE, y = x or 1.8, whichever is larger |
| (14) | x > 3.33, xr = f / foF2 ≥ 1: hr = h or 800 km, smaller; h = A1 + B1 2.4^(−a) for B1 and a ≥ 0, else A1 + B1 | A1 = 140 + (H − 47) E1; B1 = 150 + (H − 17) F1 − A1; E1 = −0.09707 xr³ + 0.6870 xr² − 0.7506 xr + 0.6; F1 = −1.862 xr⁴ + 12.95 xr³ − 32.03 xr² + 33.50 xr − 10.91 (xr ≤ 1.71), 1.21 + 0.2 xr beyond; a = (d − ds) / (H + 140); ds = 160 + (H + 43) G; G = −2.102 xr⁴ + 19.50 xr³ − 63.15 xr² + 90.47 xr − 44.73 (xr ≤ 3.7), 19.25 beyond |
| (15) | x > 3.33, xr < 1: hr = h or 800 km, smaller; h = A2 + B2 b for B2 ≥ 0, else A2 + B2 | A2 = 151 + (H − 47) E2; B2 = 141 + (H − 24) F2 − A2; E2 = 0.1906 Z² + 0.00583 Z + 0.1936; F2 = 0.645 Z² + 0.883 Z + 0.162; Z = xr or 0.1, larger; b = −7.535 df⁴ + 15.75 df³ − 8.834 df² − 0.378 df + 1; df = 0.115 d / (Z (H + 140)) or 0.65, smaller |
| (16) | x ≤ 3.33: hr = 115 + H J + U d or 800 km, smaller | J = −0.7126 y³ + 5.863 y² − 16.13 y + 16.07; U = 8×10⁻⁵ (H − 80)(1 + 11 y^−2.2) + 1.2×10⁻³ H y^−3.6 |
| (17) | Ew = 136.6 + Pt + Gt + 20 log f − Lb, dB(1 µV/m) | Pt in dB(1 kW), Gt in dBi |
| (18) | Lb = 32.45 + 20 log f + 20 log p′ + Li + Lm + Lg + Lh + Lz | Lz = 8.72 dB |
| (19) | p′ = 2R0 Σ over the n hops of sin(d / 2R0) / cos(Δ + d / 2R0) | |
| (20) | Li = n (1 + 0.0067 R12) sec i / (f + fL)² · (1/k) Σⱼ ATnoon,j φn(fv / foE) F(χj) / F(χj,noon) | k penetration points, two a hop at 90 km; i at 110 km |
| (21), (22), (23) | F(χ) = cos^p(0.881 χ) or 0.02, greater; fv = f cos i; fL = fH sin I | at 100 km |
| (24) to (26) | Lm = 0 at or below the mode's basic MUF fb; above it, E modes: 130 ((f / fb) − 1)² or 81 dB, smaller; F2 modes: 36 ((f / fb) − 1)^½ or 62 dB, smaller | ITUHFProp (P.533 version 14.2) uses other forms; this library follows P.533-14 |
| (27) | Lg = 2 (n − 1) dB | |
| (28) | Es = 10 log₁₀ Σ_w 10^(Ew / 10) | the modes' field strengths summed in power |
| (43), (44) | Prw = Ew + Grw − 20 log₁₀ f − 107.2 dBW; Pr = 10 log₁₀ Σ_w 10^(Prw / 10) | |
| (45) | S/N = Pr − Fa − 10 log₁₀ b + 204 | b in Hz; Fa the external noise factor, dB(kT₀b) |

**R12 from the library's F10.7 (ITU-R P.371-9 (08/2023), equation (4), read from its Word edition's equation
markup):** Φ12 = 63.7 + 0.728 R12 + 8.9 × 10⁻⁴ R12².

**The noise (ITU-R P.372-18, Table 1, as the G6.2 desk survey read it):** man-made noise Fam = c − d log f, f in
MHz: City 76.8, 27.7; Residential 72.5, 27.7; Rural 67.2, 27.7; Quiet rural 53.6, 28.6; galactic noise 52.0, 23.0,
heard above foF2 only. Atmospheric noise needs ITU's noise-map data and is not modelled (watchpost D-187).

## §5.3 and §5.4: paths past 7000 km (G6.2c, watchpost D-191)

Read 2026-10-15 from the same Word edition: these equations are Office Math markup, rebuilt with their fractions,
powers and roots; eq. (42) from its embedded preview's glyphs; Tables 3, 4 and 5 from the document's tables.

| # | Equation | Note |
|---|---|---|
| — | fM's hops: the fewest of at most 4000 km, one more while the elevation (13) from 300 km is under 3° | control points dM/2 from each end |
| (29) | fBM = fz + (f4 − fz) fD; f4 = 1.1 foF2 M(3000)F2; fz = foF2 + fH/2 | at each control point; the lower is the path's |
| (30) | fD = ((((((C6 dM + C5) dM + C4) dM + C3) dM + C2) dM + C1) dM + C0) dM | C6 −2.40074637494790e−24, C5 25.8520201885984e−21, C4 −92.4986988833091e−18, C3 102.342990689362e−15, C2 22.0776941764705e−12, C1 87.4376851991085e−9, C0 29.1996868566837e−6 |
| (31), (32) | fM = K fBM; K = 1.2 + W fBM/fBM,noon + X (∛(fBM,noon/fBM) − 1) + Y (fBM,min/fBM,noon)² | the lower of the two points' fM; W, X, Y from Table 3 (east-west 0.1, 1.2, 0.6; north-south 0.2, 0.2, 0.4), read between by the bearing at the path's centre |
| (33) | fL = (5.3 √[(1 + 0.009 R12) Σ cos^0.5 χ / (cos i90 ln(9.5×10⁶ / p′))] − fH)(Aw + 1) | the penetration points of hops of at most 3000 km from 300 km; cos^0.5 χ none past 90°; R12 not capped |
| (34), (35) | χ from the subsolar latitude for mid-month (Table 4) and the hour angle | the library takes the sun's own position each hour (A-14) |
| — | Aw: none from 0° to 30° and at 90°, Table 5's at 60° (northern J .30 F .15 M .03 A–A 0 S .01 O .03 N .15 D .30; southern J–M 0 A .03 M .15 J .30 J .30 A .15 S .03 O–D 0), linear between | at the path's middle |
| (36) | fLN = √(D/3000) | the night LUF: each hour's fL at least this |
| (37), (38) | where fL falls through 2 fLN at hour tr: fL(tr) = e^−0.23 fL(tr−1) (dt (1 − e^−0.23) + e^−0.23), dt = (2 fLN − fL(tr)) / (fL(tr−1) − fL(tr)); then fL(tr+n) = fL(tr+n−1) e^−0.23 for n = 1 to 3; each kept where larger | |
| (39) | Etl = E0 [1 − (fM+fH)²/((fM+fH)² + (fL+fH)²) ((fL+fH)²/(f+fH)² + (f+fH)²/(fM+fH)²)] − 30 + Pt + Gtl + Gap − Ly | fH the mean at the two control points; Ly = −0.14 dB |
| (40) | E0 = 139.6 − 20 log p′ | p′ from (19), (13), 300 km, fM's hops |
| (41) | Gap = 10 log(D / (R0 sin(D/R0))), at most 15 dB | |
| (42) | Eti = 100 log10 Xi; Xi = Xs + (D − 7000)/2000 (Xl − Xs); Xs = 10^(0.01 Es); Xl = 10^(0.01 El) | 7000 to 9000 km |

**Where ITUHFProp differs from the recommendation, the library follows the recommendation:** Table 5's northern
September (0.01 printed, 0.00 held) and Ly (−0.14 dB printed; ITUHFProp uses its own constant).
