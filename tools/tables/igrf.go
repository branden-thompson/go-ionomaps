package main

import (
	"bufio"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// igrfRadius is IGRF's reference radius, km.
const igrfRadius = 6371.2

// igrf is an IGRF model as IAGA publishes it: Schmidt semi-normalised
// coefficients in nT at each five-year epoch, and the secular variation, in
// nT a year, after the last.
type igrf struct {
	degree int
	epochs []float64
	g, h   [][][]float64 // [epoch][n][m]
	sg, sh [][]float64   // secular variation [n][m]
}

var errIGRF = errors.New("tables: not IAGA's IGRF coefficient file")

// igrfRow is one coefficient row of IAGA's table: g or h, its degree and
// order, a value at each epoch and the secular variation last.
type igrfRow struct {
	gh   string
	n, o int
	vals []float64
}

// loadIGRF reads IAGA's coefficient table (igrfNNcoeffs.txt): its header
// line of epochs, then one row a coefficient, g or h, degree, order, a value
// at each epoch, and the secular variation last.
func loadIGRF(path string) (igrf, error) {
	f, err := os.Open(path)
	if err != nil {
		return igrf{}, err
	}
	defer func() { _ = f.Close() }() // read only
	var m igrf
	rows, err := m.readRows(bufio.NewScanner(f))
	if err != nil {
		return igrf{}, err
	}
	if len(m.epochs) == 0 || len(rows) == 0 {
		return igrf{}, errIGRF
	}
	if err := m.fill(rows); err != nil {
		return igrf{}, err
	}
	return m, nil
}

// readRows reads the table's epochs into the model, and its coefficient
// rows, the model's degree the highest among them.
func (m *igrf) readRows(sc *bufio.Scanner) ([]igrfRow, error) {
	var rows []igrfRow
	line := 0
	for ; line < maxIGRFLines && sc.Scan(); line++ {
		fields := strings.Fields(sc.Text())
		if len(fields) > 3 && fields[0] == "g/h" {
			for _, e := range fields[3 : len(fields)-1] {
				y, err := strconv.ParseFloat(e, 64)
				if err != nil {
					return nil, errIGRF
				}
				m.epochs = append(m.epochs, y)
			}
			continue
		}
		if len(fields) < 4 || (fields[0] != "g" && fields[0] != "h") {
			continue
		}
		n, err1 := strconv.Atoi(fields[1])
		o, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil {
			return nil, errIGRF
		}
		r := igrfRow{gh: fields[0], n: n, o: o}
		for _, v := range fields[3:] {
			x, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, errIGRF
			}
			r.vals = append(r.vals, x)
		}
		rows = append(rows, r)
		m.degree = max(m.degree, n)
	}
	if line == maxIGRFLines && sc.Scan() {
		return nil, errIGRF // longer than any IAGA table: not one
	}
	return rows, sc.Err()
}

// maxIGRFLines is the longest IGRF table read: IAGA's runs to about two
// hundred lines, its coefficients to degree 13.
const maxIGRFLines = 4096

// fill lays the rows into the model's coefficient tables, at each epoch and
// for the secular variation; a row of the wrong length, or an order past its
// degree, is not IAGA's table.
func (m *igrf) fill(rows []igrfRow) error {
	alloc := func() [][]float64 {
		out := make([][]float64, m.degree+1)
		for n := range out {
			out[n] = make([]float64, n+1)
		}
		return out
	}
	m.g, m.h = make([][][]float64, len(m.epochs)), make([][][]float64, len(m.epochs))
	for e := range m.epochs {
		m.g[e], m.h[e] = alloc(), alloc()
	}
	m.sg, m.sh = alloc(), alloc()
	for _, r := range rows {
		if len(r.vals) != len(m.epochs)+1 || r.o > r.n {
			return errIGRF
		}
		for e := range m.epochs {
			if r.gh == "g" {
				m.g[e][r.n][r.o] = r.vals[e]
			} else {
				m.h[e][r.n][r.o] = r.vals[e]
			}
		}
		if r.gh == "g" {
			m.sg[r.n][r.o] = r.vals[len(m.epochs)]
		} else {
			m.sh[r.n][r.o] = r.vals[len(m.epochs)]
		}
	}
	return nil
}

// decimalYear is a moment as IAGA's decimal year: the year, and the part of
// it past.
func decimalYear(t time.Time) float64 {
	t = t.UTC()
	start := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(t.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)
	return float64(t.Year()) + t.Sub(start).Seconds()/end.Sub(start).Seconds()
}

// coefficients are the model's at a decimal year: linear between epochs, and
// after the last epoch, the last plus its secular variation.
func (m igrf) coefficients(year float64) (g, h [][]float64) {
	last := len(m.epochs) - 1
	g, h = make([][]float64, m.degree+1), make([][]float64, m.degree+1)
	e, w := last, 0.0
	for i := range last {
		if year < m.epochs[i+1] {
			e, w = i, (year-m.epochs[i])/(m.epochs[i+1]-m.epochs[i])
			break
		}
	}
	for n := 0; n <= m.degree; n++ {
		g[n], h[n] = make([]float64, n+1), make([]float64, n+1)
		for o := 0; o <= n; o++ {
			if e == last {
				dt := year - m.epochs[last]
				g[n][o] = m.g[last][n][o] + m.sg[n][o]*dt
				h[n][o] = m.h[last][n][o] + m.sh[n][o]*dt
				continue
			}
			g[n][o] = m.g[e][n][o] + w*(m.g[e+1][n][o]-m.g[e][n][o])
			h[n][o] = m.h[e][n][o] + w*(m.h[e+1][n][o]-m.h[e][n][o])
		}
	}
	return g, h
}

// model is the field at one moment: the coefficients interpolated once.
type model struct {
	degree int
	g, h   [][]float64
}

// at is the model at a decimal year.
func (m igrf) at(year float64) model {
	g, h := m.coefficients(year)
	return model{degree: m.degree, g: g, h: h}
}

// field is the main field at a geocentric radius in km, colatitude and
// longitude in radians: its radial, colatitude and longitude components in
// nT, B = -∇V.
func (md model) field(r, theta, phi float64) (br, bt, bp float64) {
	g, h := md.g, md.h
	p, dp := schmidt(md.degree, theta)
	s := math.Sin(theta)
	if math.Abs(s) < 1e-10 {
		s = 1e-10 // at a pole, where the longitude term's limit is taken
	}
	for n := 1; n <= md.degree; n++ {
		rn := math.Pow(igrfRadius/r, float64(n+2))
		for o := 0; o <= n; o++ {
			c, sn := math.Cos(float64(o)*phi), math.Sin(float64(o)*phi)
			gh := g[n][o]*c + h[n][o]*sn
			br += float64(n+1) * rn * gh * p[n][o]
			bt -= rn * gh * dp[n][o]
			bp += rn * float64(o) * (g[n][o]*sn - h[n][o]*c) * p[n][o] / s
		}
	}
	return br, bt, bp
}

// schmidt is the Schmidt semi-normalised associated Legendre functions of
// cos(theta), without the Condon-Shortley phase, and their derivatives in
// theta, to a degree.
func schmidt(degree int, theta float64) (p, dp [][]float64) {
	z, s := math.Cos(theta), math.Sin(theta)
	p, dp = make([][]float64, degree+1), make([][]float64, degree+1)
	for n := range p {
		p[n], dp[n] = make([]float64, n+1), make([]float64, n+1)
	}
	// Unnormalised first, then scaled: P_m^m, P_(m+1)^m, then upward in n.
	p[0][0] = 1
	for m := 1; m <= degree; m++ {
		p[m][m] = float64(2*m-1) * s * p[m-1][m-1]
	}
	for m := 0; m < degree; m++ {
		p[m+1][m] = float64(2*m+1) * z * p[m][m]
	}
	for m := 0; m <= degree; m++ {
		for n := m + 2; n <= degree; n++ {
			p[n][m] = (float64(2*n-1)*z*p[n-1][m] - float64(n+m-1)*p[n-2][m]) / float64(n-m)
		}
	}
	// dP_n^m/dθ = (n z P_n^m - (n+m) P_(n-1)^m) / s, before scaling.
	for n := 1; n <= degree; n++ {
		for m := 0; m <= n; m++ {
			prev := 0.0
			if m <= n-1 {
				prev = p[n-1][m]
			}
			if math.Abs(s) < 1e-10 {
				dp[n][m] = 0
				continue
			}
			dp[n][m] = (float64(n)*z*p[n][m] - float64(n+m)*prev) / s
		}
	}
	for n := 1; n <= degree; n++ {
		for m := 1; m <= n; m++ {
			k := math.Sqrt(2 * ratio(n, m))
			p[n][m] *= k
			dp[n][m] *= k
		}
	}
	return p, dp
}

// ratio is (n-m)!/(n+m)!, as a product.
func ratio(n, m int) float64 {
	r := 1.0
	for i := n - m + 1; i <= n+m; i++ {
		r /= float64(i)
	}
	return r
}
