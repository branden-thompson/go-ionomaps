# ITU-R P.533-14 §3.3 to §3.5: the basic MUF, read from the recommendation

**Why:** wave 1 rebuilt these equations from a garbled PDF extraction and required them checked against the
recommendation before G6 codes them (watchpost `wave1-findings.md`, "Traps"). This is that check.

**How they were read (2026-10-11):** ITU publishes P.533-14 (08/2019) free to read. Its PDF is a scanned image;
its Word edition carries §3's prose as text and equations (4) and (5) as text, and equations (1), (2), (3), (6),
(7) and (8) as embedded Equation Editor objects, whose WMF previews keep each glyph as a text record with its
position. The glyphs were read by position and the equations rebuilt from them. ITU's text and tables are not
reproduced (the terms, wave 1); the equations are the method.

| # | Equation | Note |
|---|---|---|
| (1) | n0E(D)MUF = foE · sec i₁₁₀ | i₁₁₀: the angle of incidence at a mid-hop mirror-reflection height of 110 km, hop d = D/n |
| (2) | hr = 1490 / M(3000)F2 − 176 km, or 500 km, whichever is smaller | the mirror-reflection height at the mid-path control point; sets the lowest-order mode |
| (3) | nF2(D)MUF = [1 + (Cd / C3000)(B − 1)] · foF2 + (fH / 2)(1 − d / dmax) | fH: the electron gyrofrequency at 300 km at the control point |
| (4) | Cd = 0.74 − 0.591 Z − 0.424 Z² − 0.090 Z³ + 0.088 Z⁴ + 0.181 Z⁵ + 0.096 Z⁶, Z = 1 − 2d / dmax | C3000 is Cd at d = 3000 km |
| (5) | dmax = 4780 + (12610 + 2140 / x² − 49720 / x⁴ + 688900 / x⁶)(1 / B − 0.303) | x = foF2 / foE, or 2, whichever is larger; for the basic MUF, dmax is held to at most 4000 km |
| (6) | B = M(3000)F2 − 0.124 + [M(3000)F2² − 4] · [0.0215 + 0.005 sin(7.854 / x − 1.9635)] | |

Paths longer than dmax take the lower of the F2(dmax)MUF values at the two control points of the
recommendation's Table 1a); higher-order modes past dmax scale by Mn / Mn0 (equations (7), (8), not needed
for G6's first answers).

**A check the equations carry:** at d = 3000 km, Cd = C3000, so (3) gives B · foF2 + (fH / 2)(1 − 3000 / dmax),
and B ≈ M(3000)F2: the path MUF at 3000 km is the MUF(3000) the library already draws.

**No worked values:** the recommendation gives no numerical worked example for §3.5, so R-2.7's "checked against
the recommendation's own worked values" has nothing to check against as written: a ruling for G6.

## foE, which P.533 §3.2 takes from ITU-R P.1239-4 (08/2023) §4

Read the same day from P.1239-4's Word edition, whose equations are Word EQ fields (text, Symbol-font letters
standing for λ geographic latitude, δ solar declination, χ solar zenith angle in degrees, Φ the 10.7 cm flux).

| # | Equation | Note |
|---|---|---|
| (12) | foE⁴ = A · B · C · D | foE in MHz |
| (13) | A = 1 + 0.0094 (Φ − 66) | Φ: the monthly mean 10.7 cm flux; for prediction, Φ₁₂, the twelve-month smoothed value |
| (14) | B = cos^m N | N = λ − δ where \|λ − δ\| < 80°, else 80° |
| (15a, b) | m = −1.93 + 1.92 cos λ for \|λ\| < 32°; m = 0.11 − 0.49 cos λ for \|λ\| ≥ 32° | |
| (16a–c) | C = X + Y cos λ; X = 23, Y = 116 for \|λ\| < 32°; X = 92, Y = 35 for \|λ\| ≥ 32° | |
| (17a) | D = cos^p χ, for χ ≤ 73° | p = 1.31 for \|λ\| ≤ 12°, 1.20 beyond |
| (17b, c) | D = cos^p (χ − δχ), δχ = 6.27 × 10⁻¹³ (χ − 50)⁸ degrees, for 73° < χ < 90° | |
| (17d, e) | at night (χ ≥ 90°), D is the greater of 0.072^p exp(−1.4 h) and 0.072^p exp(25.2 − 0.28 χ) | h: hours after sunset; in a polar winter, (17e) |
| (18) | foE⁴ minimum = 0.004 (1 + 0.021 Φ)² | taken at night where (12) to (17e) give less |

P.1239 reports the method's median r.m.s. deviation as 0.11 MHz over 80 000 hourly comparisons at 55 stations.
**Still to settle for G6:** the gyrofrequency fH at 300 km, which needs the field's strength there - IGRF-14 is
evaluated at build time only (`tools/tables`), so a table of fH by cell and year, as the coordinates have, is
the likely shape.
