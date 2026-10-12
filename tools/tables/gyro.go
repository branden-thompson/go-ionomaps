package main

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

// gyroMagic opens the gyrofrequency table: the format's name and version.
const gyroMagic = "IONOFH01"

// gyroPerNanotesla is the electron gyrofrequency per nanotesla of field,
// MHz: e / (2π mₑ), 27.99249 Hz/nT (CODATA 2018).
const gyroPerNanotesla = 27.99249e-6

var errGyro = errors.New("tables: not a gyrofrequency table, or not whole")

// generateGyro is each year's electron gyrofrequency at 300 km, MHz, at
// each cell centre on 1 January: the field's strength there from IGRF-14.
func generateGyro(m igrf, years []int) [][]float32 {
	out := make([][]float32, len(years))
	for y, year := range years {
		md := m.at(decimalYear(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)))
		cells := make([]float32, coordCells)
		for i := range cells {
			glat, glon := 89-2*float64(i/coordCols), -179+2*float64(i%coordCols)
			cells[i] = float32(gyroPerNanotesla * norm(md.fieldXYZ(geocentric(glat, glon, 300))))
		}
		out[y] = cells
	}
	return out
}

// writeGyro is the compact form: the magic, the number of years, columns
// and rows, the years, then each year's cells, little-endian float32.
func writeGyro(years []int, tab [][]float32) []byte {
	b := []byte(gyroMagic)
	for _, n := range []int{len(years), coordCols, coordRows} {
		b = binary.LittleEndian.AppendUint32(b, uint32(n))
	}
	for _, y := range years {
		b = binary.LittleEndian.AppendUint32(b, uint32(y))
	}
	for y := range years {
		for _, v := range tab[y] {
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
		}
	}
	return b
}

// readGyro reads the compact form back, refusing one not whole.
func readGyro(b []byte) ([][]float32, []int, error) {
	head := len(gyroMagic) + 12
	if len(b) < head || string(b[:len(gyroMagic)]) != gyroMagic {
		return nil, nil, errGyro
	}
	n := int(binary.LittleEndian.Uint32(b[len(gyroMagic):]))
	if n < 1 || n > 100 || len(b) != head+4*n+4*n*coordCells {
		return nil, nil, errGyro
	}
	years := make([]int, n)
	for i := range years {
		years[i] = int(binary.LittleEndian.Uint32(b[head+4*i:]))
	}
	out := make([][]float32, n)
	at := head + 4*n
	for y := range out {
		out[y] = make([]float32, coordCells)
		for i := range out[y] {
			out[y][i] = math.Float32frombits(binary.LittleEndian.Uint32(b[at:]))
			at += 4
		}
	}
	return out, years, nil
}
