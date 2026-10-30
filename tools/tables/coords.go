package main

import (
	"encoding/binary"
	"errors"
	"math"
	"runtime"
	"sync"
	"time"
)

// The library's grid (R-1.1): 2° cells, rows from 89°N, columns from 179°W.
const (
	coordCols  = 180
	coordRows  = 90
	coordCells = coordCols * coordRows
	coordMagic = "IONOQD01"
)

var errCoords = errors.New("tables: not a coordinate table, or not whole")

// generateCoords is each year's quasi-dipole latitude and longitude at each
// cell centre, height 0, on 1 January: the cells shared among the CPUs.
func generateCoords(m igrf, years []int) ([][2][]float32, error) {
	out := make([][2][]float32, len(years))
	for y, year := range years {
		md := m.at(decimalYear(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)))
		lat, lon := make([]float32, coordCells), make([]float32, coordCells)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var failed error
		next := make(chan int, coordCells)
		for i := range coordCells {
			next <- i
		}
		close(next)
		for range runtime.NumCPU() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range next {
					glat, glon := 89-2*float64(i/coordCols), -179+2*float64(i%coordCols)
					q, l, ok := md.quasiDipole(glat, glon, 0)
					if !ok {
						mu.Lock()
						failed = errCoords
						mu.Unlock()
						continue
					}
					lat[i], lon[i] = float32(q), float32(l)
				}
			}()
		}
		wg.Wait()
		if failed != nil {
			return nil, failed
		}
		out[y] = [2][]float32{lat, lon}
	}
	return out, nil
}

// writeCoords is the compact form: the magic, the number of years, columns
// and rows, the years, then for each year every cell's latitude and then
// every cell's longitude, little-endian float32.
func writeCoords(years []int, tab [][2][]float32) []byte {
	b := []byte(coordMagic)
	for _, n := range []int{len(years), coordCols, coordRows} {
		b = binary.LittleEndian.AppendUint32(b, uint32(n))
	}
	for _, y := range years {
		b = binary.LittleEndian.AppendUint32(b, uint32(y))
	}
	for y := range years {
		for k := range 2 {
			for _, v := range tab[y][k] {
				b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
			}
		}
	}
	return b
}

// readCoords reads the compact form back.
func readCoords(b []byte) ([][2][]float32, []int, error) {
	head := len(coordMagic) + 12
	if len(b) < head || string(b[:len(coordMagic)]) != coordMagic {
		return nil, nil, errCoords
	}
	n := int(binary.LittleEndian.Uint32(b[len(coordMagic):]))
	cols, rows := int(binary.LittleEndian.Uint32(b[len(coordMagic)+4:])), int(binary.LittleEndian.Uint32(b[len(coordMagic)+8:]))
	if n <= 0 || n > 100 || cols != coordCols || rows != coordRows || len(b) != head+4*n+4*n*2*coordCells {
		return nil, nil, errCoords
	}
	years := make([]int, n)
	for i := range years {
		years[i] = int(binary.LittleEndian.Uint32(b[head+4*i:]))
	}
	at := head + 4*n
	out := make([][2][]float32, n)
	for y := range n {
		for k := range 2 {
			out[y][k] = make([]float32, coordCells)
			for i := range coordCells {
				out[y][k][i] = math.Float32frombits(binary.LittleEndian.Uint32(b[at:]))
				at += 4
			}
		}
	}
	return out, years, nil
}
