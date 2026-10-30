"""One-time export of NRL's refits from PyIRI (watchpost D-114).

Run once, in the pinned PyIRI environment recorded beside this script
(pip-freeze.txt, python-version.txt), never by the build:

    python tools/export/export.py <PyIRI's coefficients directory> tools/tables/export

It writes each table as plain text, one line per (solar level, month,
Fourier term) holding the 900 spherical-harmonic coefficients, each written
with 17 significant digits so it reads back exactly; and a golden of PyIRI's
own foF2 and M(3000)F2 at fixed inputs, the test oracle for the Go port.
Each output's first lines name its source file and that file's SHA-256.
tools/tables reads only these files. Re-run when PyIRI updates.

PyIRI is MIT licensed, Copyright (c) 2023 victoriyaforsythe; the refits are
NRL's (Forsythe et al. 2024, doi:10.1029/2023SW003739), fitted to the CCIR
foF2 and M(3000)F2 maps. Only the refits are exported, never the raw CCIR or
URSI tables (watchpost D-43).
"""

import hashlib
import os
import sys

import netCDF4
import numpy as np
import PyIRI
import PyIRI.sh_library as sh

TABLES = [("foF2_CCIR.nc", "IG12"), ("M3000F2.nc", "R12")]


def sha256(path):
    with open(path, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


def export_table(src, name, level, out_dir):
    path = os.path.join(src, "SH", name)
    with netCDF4.Dataset(path) as ds:
        c = np.asarray(ds["Coefficients"][:], dtype=np.float64)
        levels = np.asarray(ds[level][:], dtype=np.float64)
    n_sol, n_month, n_fs, n_sh = c.shape
    out = os.path.join(out_dir, name.replace(".nc", ".txt"))
    with open(out, "w") as f:
        f.write(f"# source: PyIRI {PyIRI.__version__} coefficients/SH/{name}\n")
        f.write(f"# sha256: {sha256(path)}\n")
        f.write(f"# shape: {n_sol} {n_month} {n_fs} {n_sh} (solar level, month, Fourier term, harmonic)\n")
        f.write(f"# solar index {level}: " + " ".join(repr(float(v)) for v in levels) + "\n")
        for s in range(n_sol):
            for m in range(n_month):
                for k in range(n_fs):
                    f.write(" ".join(format(v, ".17g") for v in c[s, m, k, :]) + "\n")
    return out


def export_golden(out_dir):
    """PyIRI's own monthly foF2 and M(3000)F2 at fixed inputs, in
    quasi-dipole latitude and magnetic local time, for solar levels 0 and
    100: the oracle the Go evaluation must agree with."""
    out = os.path.join(out_dir, "golden.txt")
    qdlat = np.array([-75.0, -40.0, -12.5, 0.0, 7.5, 33.0, 61.0, 88.0])
    mlt = np.array([0.0, 3.5, 7.25, 12.0, 15.75, 18.0, 21.5, 23.0])
    uts = np.array([0.0, 6.5, 13.0, 19.75])
    with open(out, "w") as f:
        f.write(f"# source: PyIRI {PyIRI.__version__} sh_library.IRI_sh_params, coord='MLT', foF2_coeff='CCIR'\n")
        f.write("# columns: month ut qdlat mlt foF2_min foF2_max M3000_min M3000_max\n")
        for month in (1, 4, 7, 10):
            maps = sh.IRI_sh_params(2026, month, uts, mlt, qdlat, foF2_coeff="CCIR", coord="MLT")
            fo, m3 = maps[0], maps[4]
            for t, ut in enumerate(uts):
                for g in range(qdlat.size):
                    f.write(" ".join(format(v, ".17g") for v in (month, ut, qdlat[g], mlt[g],
                            fo[t, g, 0], fo[t, g, 1], m3[t, g, 0], m3[t, g, 1])) + "\n")
    return out


def export_golden_day(out_dir):
    """PyIRI's own day values: the two mid-month tables either side of the
    date blended by the day, then each parameter interpolated in its solar
    index from F10.7 (IG12 for foF2, R12 for M(3000)F2), as
    IRI_density_1day does; the oracle for the Go port's day value."""
    import PyIRI.main_library as ml
    out = os.path.join(out_dir, "golden-day.txt")
    qdlat = np.array([-55.0, -10.0, 0.0, 22.5, 48.0, 70.0])
    mlt = np.array([1.0, 6.0, 11.5, 14.0, 19.0, 22.75])
    uts = np.array([3.0, 15.5])
    with open(out, "w") as f:
        f.write(f"# source: PyIRI {PyIRI.__version__} main_library.day_of_the_month_corr, sh_library.IRI_sh_params (coord='MLT', foF2_coeff='CCIR'), main_library.solar_interpolate\n")
        f.write("# columns: year month day f107 ut qdlat mlt foF2 M3000\n")
        for (year, month, day) in ((2026, 1, 3), (2026, 6, 15), (2026, 12, 28), (2027, 3, 31)):
            t0, t1, fr1, fr2 = ml.day_of_the_month_corr(year, month, day)
            a = sh.IRI_sh_params(t0.year, t0.month, uts, mlt, qdlat, foF2_coeff="CCIR", coord="MLT")
            b = sh.IRI_sh_params(t1.year, t1.month, uts, mlt, qdlat, foF2_coeff="CCIR", coord="MLT")
            blend = a * fr1 + b * fr2
            for f107 in (65.0, 120.0, 250.0):
                fo = ml.solar_interpolate(blend[0][:, :, 0], blend[0][:, :, 1], f107, solidx="IG12", solmin=0, solmax=100)
                m3 = ml.solar_interpolate(blend[4][:, :, 0], blend[4][:, :, 1], f107, solidx="R12", solmin=0, solmax=100)
                for t, ut in enumerate(uts):
                    for g in range(qdlat.size):
                        f.write(" ".join(format(v, ".17g") for v in (year, month, day, f107, ut, qdlat[g], mlt[g],
                                fo[t, g], m3[t, g])) + "\n")
    return out


def main():
    src, out_dir = sys.argv[1], sys.argv[2]
    os.makedirs(out_dir, exist_ok=True)
    for name, level in TABLES:
        print(export_table(src, name, level, out_dir))
    print(export_golden(out_dir))
    print(export_golden_day(out_dir))


if __name__ == "__main__":
    main()
