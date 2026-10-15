package main

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

// field100Magic opens the 100 km field table: the format's name and version.
const field100Magic = "IONOF100"

var errField100 = errors.New("tables: not a 100 km field table, or not whole")

// dipAndStrength is the field at a geodetic place and height (km): its dip
// against the local geodetic vertical, positive downward, degrees, and its
// strength, nT.
func dipAndStrength(md model, lat, lon, h float64) (dip, nT float64) {
	b := md.fieldXYZ(geocentric(lat, lon, h))
	phi, lam := lat*math.Pi/180, lon*math.Pi/180
	up := [3]float64{math.Cos(phi) * math.Cos(lam), math.Cos(phi) * math.Sin(lam), math.Sin(phi)}
	down := -(b[0]*up[0] + b[1]*up[1] + b[2]*up[2])
	nT = norm(b)
	return math.Atan2(down, math.Sqrt(math.Max(nT*nT-down*down, 0))) * 180 / math.Pi, nT
}

// generateField100 is each year's electron gyrofrequency (MHz) and dip
// (degrees) at 100 km at each cell centre on 1 January, from IGRF-14: what
// P.533-14 §5.2.2 reads for the absorption (fL = fH sin I, and p by the
// modified dip).
func generateField100(m igrf, years []int) (fh, dip [][]float32) {
	fh, dip = make([][]float32, len(years)), make([][]float32, len(years))
	for y, year := range years {
		md := m.at(decimalYear(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)))
		fh[y], dip[y] = make([]float32, coordCells), make([]float32, coordCells)
		for i := range coordCells {
			glat, glon := 89-2*float64(i/coordCols), -179+2*float64(i%coordCols)
			d, nT := dipAndStrength(md, glat, glon, 100)
			fh[y][i], dip[y][i] = float32(gyroPerNanotesla*nT), float32(d)
		}
	}
	return fh, dip
}

// writeField100 is the compact form: the magic, the number of years,
// columns and rows, the years, then each year's gyrofrequencies and dips,
// little-endian float32.
func writeField100(years []int, fh, dip [][]float32) []byte {
	b := []byte(field100Magic)
	for _, n := range []int{len(years), coordCols, coordRows} {
		b = binary.LittleEndian.AppendUint32(b, uint32(n))
	}
	for _, y := range years {
		b = binary.LittleEndian.AppendUint32(b, uint32(y))
	}
	for y := range years {
		for _, part := range [][]float32{fh[y], dip[y]} {
			for _, v := range part {
				b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
			}
		}
	}
	return b
}

// readField100 reads the compact form back, refusing one not whole.
func readField100(b []byte) (fh, dip [][]float32, years []int, err error) {
	head := len(field100Magic) + 12
	if len(b) < head || string(b[:len(field100Magic)]) != field100Magic {
		return nil, nil, nil, errField100
	}
	n := int(binary.LittleEndian.Uint32(b[len(field100Magic):]))
	if n < 1 || n > 100 || len(b) != head+4*n+8*n*coordCells {
		return nil, nil, nil, errField100
	}
	years = make([]int, n)
	for i := range years {
		years[i] = int(binary.LittleEndian.Uint32(b[head+4*i:]))
	}
	at := head + 4*n
	next := func() []float32 {
		out := make([]float32, coordCells)
		for i := range out {
			out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[at:]))
			at += 4
		}
		return out
	}
	for range n {
		fh, dip = append(fh, next()), append(dip, next())
	}
	return fh, dip, years, nil
}
