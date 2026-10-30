"""A sample of PyIRI's Apex.nc coordinates, the test oracle for the
library's own quasi-dipole coordinates (watchpost D-107).

Run once, in the pinned PyIRI environment (pip-freeze.txt beside this
script), never by the build:

    python tools/export/apex_sample.py tools/tables/apex/apex-sample.txt

At every third cell centre of the library's 2-degree grid (89N to 89S,
179W to 179E), geographic to quasi-dipole for 2026-06-01, through PyIRI's
Apex_geo_qd, which evaluates Apex.nc's spherical-harmonic fit (MIT,
generated with ApexPy); and PyIRI's magnetic local time there at 00:00 and
12:00 UT, with its subsolar point at each.
"""

import datetime as dt
import hashlib
import os
import sys

import numpy as np
import PyIRI
import PyIRI.sh_library as sh


def main():
    out = sys.argv[1]
    lats = 89.0 - 2.0 * np.arange(0, 90, 3)
    lons = -179.0 + 2.0 * np.arange(0, 180, 3)
    glat, glon = np.meshgrid(lats, lons, indexing="ij")
    when = dt.datetime(2026, 6, 1)
    qlat, qlon = sh.Apex_geo_qd(glat.ravel(), np.mod(glon.ravel(), 360), when, "GEO_2_QD")
    hours = (0, 12)
    mlts = [sh.Apex(glat.ravel(), np.mod(glon.ravel(), 360), when + dt.timedelta(hours=h), "GEO_2_MLT")[1] for h in hours]
    suns = [sh.find_subsolar(when + dt.timedelta(hours=h), adjust_type="to180") for h in hours]
    src = os.path.join(PyIRI.coeff_dir, "Apex", "Apex.nc")
    with open(out, "w") as f:
        f.write(f"# source: PyIRI {PyIRI.__version__} sh_library.Apex_geo_qd, coefficients/Apex/Apex.nc\n")
        f.write(f"# sha256: {hashlib.sha256(open(src, 'rb').read()).hexdigest()}\n")
        f.write(f"# date: {when.date()}, height 0 (geodetic)\n")
        for h, (slon, slat) in zip(hours, suns):
            f.write(f"# subsolar at {h:02d}:00 UT: lon {float(slon)!r} lat {float(slat)!r}\n")
        f.write("# columns: glat glon qdlat qdlon mlt_00UT mlt_12UT\n")
        for a, b, c, d, m0, m12 in zip(glat.ravel(), glon.ravel(), qlat, qlon, mlts[0], mlts[1]):
            f.write(" ".join(format(float(v), ".17g") for v in (a, b, c, d, m0, m12)) + "\n")
    print(out)


if __name__ == "__main__":
    main()
