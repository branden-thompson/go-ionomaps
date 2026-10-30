"""IGRF-14's field at fixed places and dates, from ppigrf (MIT), the test
oracle for tools/tables' own IGRF evaluation (watchpost D-107).

Run once, in an environment with ppigrf 2.1.0 (igrf-pip-freeze.txt beside
this script), never by the build:

    python tools/export/igrf_golden.py tools/tables/igrf/golden-igrf.txt

ppigrf ships IGRF-14 with the same coefficients as IAGA's
igrf14coeffs.txt, value for value (checked when this was written). Each line
is a geocentric radius in km, a colatitude and a longitude in degrees, a
date, and the field's radial, colatitude and longitude components in nT.
"""

import datetime as dt
import sys

import numpy as np
import ppigrf


def main():
    out = sys.argv[1]
    places = [(6371.2, 10.0, 0.0), (6371.2, 45.0, 120.0), (6371.2, 90.0, -75.0), (6371.2, 135.0, 200.0),
              (6371.2, 170.0, 300.0), (6800.0, 60.0, 30.0), (7500.0, 100.0, -150.0), (6371.2, 0.5, 45.0)]
    dates = [dt.datetime(2026, 1, 1), dt.datetime(2028, 7, 2), dt.datetime(2015, 1, 1), dt.datetime(1950, 1, 1),
             dt.datetime(2012, 7, 1), dt.datetime(1983, 3, 15)]  # the last two between epochs
    with open(out, "w") as f:
        f.write(f"# source: ppigrf {ppigrf.__version__ if hasattr(ppigrf, '__version__') else '2.1.0'} igrf_gc, IGRF-14\n")
        f.write("# columns: r_km colat_deg lon_deg year month day Br_nT Btheta_nT Bphi_nT\n")
        for d in dates:
            for (r, th, ph) in places:
                br, bt, bp = ppigrf.igrf_gc(r, th, ph, d)
                f.write(" ".join(format(float(v), ".17g") for v in (r, th, ph, d.year, d.month, d.day,
                        np.ravel(br)[0], np.ravel(bt)[0], np.ravel(bp)[0])) + "\n")
    print(out)


if __name__ == "__main__":
    main()
