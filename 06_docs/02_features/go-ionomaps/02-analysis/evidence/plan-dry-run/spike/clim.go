// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import (
	"math"
	"math/rand/v2"
)

// Climatology evaluation spike: a PyIRI-shaped spherical-harmonic model.
// Shape only (confirmed against PyIRI's MIT sh_library.py); coefficients are
// random, never PyIRI's data.
//
//   - per solar level and field: 11 real Fourier terms in UT x 900 real SH
//     terms (lmax 29, 4pi-normalized, Condon-Shortley phase);
//   - GEO -> QD by a 441-term SH evaluation (lmax 20) of QDLat, QDLon_cos and
//     QDLon_sin; MLT from QD longitude minus the sub-solar point's QD longitude;
//   - value at a cell = sum over SH of [sum over Fourier of F_FS(UT) C] Y(QD
//     colatitude, MLT angle); two solar levels evaluated and blended.

const (
	nFS   = 11
	lmaxC = 29
	nSHC  = (lmaxC + 1) * (lmaxC + 1)     // 900
	nPC   = (lmaxC + 1) * (lmaxC + 2) / 2 // 465 Legendre values per cell
	lmaxA = 20
	nSHA  = (lmaxA + 1) * (lmaxA + 1) // 441
	nPA   = (lmaxA + 1) * (lmaxA + 2) / 2
)

type ClimCoeffs struct {
	C [2][2][nFS][nSHC]float64 // [solar level][field: foF2, M3000][Fourier][SH]
	A [3][nSHA]float64         // QDLat, QDLon_cos, QDLon_sin
}

func NewClimCoeffs(seed uint64) *ClimCoeffs {
	r := rand.New(rand.NewPCG(seed, 1))
	cc := &ClimCoeffs{}
	for lev := range 2 {
		for f := range 2 {
			for j := range nFS {
				for k := range nSHC {
					l := math.Floor(math.Sqrt(float64(k)))
					cc.C[lev][f][j][k] = r.NormFloat64() / (1 + l*l)
				}
			}
		}
		cc.C[lev][0][0][0] += 7 + 2*float64(lev) // foF2 mean
		cc.C[lev][1][0][0] += 3.1                // M3000 mean
	}
	for q := range 3 {
		for k := range nSHA {
			cc.A[q][k] = 0.01 * r.NormFloat64()
		}
	}
	return cc
}

// legNorm[l][m]: 4pi normalization, factor 2 for m > 0.
var legNorm [lmaxC + 1][lmaxC + 1]float64

func init() {
	for l := 0; l <= lmaxC; l++ {
		for m := 0; m <= l; m++ {
			k := 2.0
			if m == 0 {
				k = 1
			}
			lg1, _ := math.Lgamma(float64(l - m + 1))
			lg2, _ := math.Lgamma(float64(l + m + 1))
			legNorm[l][m] = math.Sqrt(k * float64(2*l+1) * math.Exp(lg1-lg2))
		}
	}
}

func pIdx(l, m int) int { return l*(l+1)/2 + m }

// legendre fills out[pIdx(l,m)] with normalized P_lm(x), s = sqrt(1-x^2).
func legendre(x, s float64, lmax int, out []float64) {
	pmm := 1.0
	for m := 0; m <= lmax; m++ {
		if m > 0 {
			pmm *= -float64(2*m-1) * s
		}
		out[pIdx(m, m)] = pmm * legNorm[m][m]
		if m == lmax {
			break
		}
		p0, p1 := pmm, x*float64(2*m+1)*pmm
		out[pIdx(m+1, m)] = p1 * legNorm[m+1][m]
		for l := m + 2; l <= lmax; l++ {
			p2 := (float64(2*l-1)*x*p1 - float64(l+m-1)*p0) / float64(l-m)
			out[pIdx(l, m)] = p2 * legNorm[l][m]
			p0, p1 = p1, p2
		}
	}
}

// legA, legB: recursion coefficients (2l-1)/(l-m) and (l+m-1)/(l-m), so the
// fast recursion multiplies instead of dividing.
var legA, legB [lmaxC + 1][lmaxC + 1]float64

func init() {
	for l := 0; l <= lmaxC; l++ {
		for m := 0; m < l; m++ {
			legA[l][m] = float64(2*l-1) / float64(l-m)
			legB[l][m] = float64(l+m-1) / float64(l-m)
		}
	}
}

// legendreFast is legendre with the division-free recursion.
func legendreFast(x, s float64, lmax int, out []float64) {
	pmm := 1.0
	for m := 0; m <= lmax; m++ {
		if m > 0 {
			pmm *= -float64(2*m-1) * s
		}
		out[pIdx(m, m)] = pmm * legNorm[m][m]
		if m == lmax {
			break
		}
		p0, p1 := pmm, x*float64(2*m+1)*pmm
		out[pIdx(m+1, m)] = p1 * legNorm[m+1][m]
		for l := m + 2; l <= lmax; l++ {
			p2 := legA[l][m]*x*p1 - legB[l][m]*p0
			out[pIdx(l, m)] = p2 * legNorm[l][m]
			p0, p1 = p1, p2
		}
	}
}

func realFS(ut float64) (f [nFS]float64) {
	f[0] = 1
	for k := 1; k <= 5; k++ {
		f[2*k-1], f[2*k] = math.Cos(2*math.Pi*float64(k)*ut/24), math.Sin(2*math.Pi*float64(k)*ut/24)
	}
	return f
}

func subsolarGeo(ut float64) (latD, lonD float64) { return -5, (12 - ut) * 15 }

// ---------- before: PyIRI-style basis matrix per cell ----------

// realSHNaive builds the (lmax+1)^2 basis vector; row l(l+1)+m holds
// P_lm cos(m phi), row l(l+1)-m holds P_lm sin(m phi).
func realSHNaive(theta, phi float64, lmax int) []float64 {
	leg := make([]float64, (lmax+1)*(lmax+2)/2)
	legendre(math.Cos(theta), math.Sin(theta), lmax, leg)
	b := make([]float64, (lmax+1)*(lmax+1))
	for l := 0; l <= lmax; l++ {
		base := l * (l + 1)
		b[base] = leg[pIdx(l, 0)]
		for m := 1; m <= l; m++ {
			sm, cm := math.Sincos(float64(m) * phi)
			b[base+m] = leg[pIdx(l, m)] * cm
			b[base-m] = leg[pIdx(l, m)] * sm
		}
	}
	return b
}

func dotN(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func apexNaive(cc *ClimCoeffs, latD, lonD float64) (qdLatD, qdLonD float64) {
	b := realSHNaive((90-latD)*deg, lonD*deg, lmaxA)
	qlat := clamp(latD+dotN(b, cc.A[0][:]), -90, 90)
	qc := math.Cos(lonD*deg) + dotN(b, cc.A[1][:])
	qs := math.Sin(lonD*deg) + dotN(b, cc.A[2][:])
	return qlat, math.Atan2(qs, qc) / deg
}

// ClimHourNaive evaluates one hour, both fields, both solar levels blended
// with weight w toward the high level.
func ClimHourNaive(g *Grid, cc *ClimCoeffs, ut, w float64) (fo, m3 []float64) {
	ffs := realFS(ut)
	var coef [2][2][]float64
	for lev := range 2 {
		for f := range 2 {
			c := make([]float64, nSHC)
			for j := range nFS {
				for k := range nSHC {
					c[k] += ffs[j] * cc.C[lev][f][j][k]
				}
			}
			coef[lev][f] = c
		}
	}
	sLat, sLon := subsolarGeo(ut)
	_, sQDLon := apexNaive(cc, sLat, sLon)
	fo = make([]float64, g.Cols*g.Rows)
	m3 = make([]float64, g.Cols*g.Rows)
	for r := range g.Rows {
		for c := range g.Cols {
			qlat, qlon := apexNaive(cc, g.latD[r], g.lonD[c])
			mlt := math.Mod((qlon-sQDLon+180)/15+48, 24)
			b := realSHNaive((90-qlat)*deg, mlt*15*deg, lmaxC)
			var v [2][2]float64
			for lev := range 2 {
				for f := range 2 {
					v[lev][f] = dotN(b, coef[lev][f])
				}
			}
			i := r*g.Cols + c
			fo[i] = (1-w)*v[0][0] + w*v[1][0]
			m3[i] = (1-w)*v[0][1] + w*v[1][1]
		}
	}
	return fo, m3
}

// ---------- after ----------

// sincosM fills cm[m], sm[m] = cos(m phi), sin(m phi) for m = 0..n by angle
// addition: one Sincos per cell instead of one per m.
func sincosM(phi float64, n int, cm, sm []float64) {
	s1, c1 := math.Sincos(phi)
	cm[0], sm[0] = 1, 0
	for m := 1; m <= n; m++ {
		cm[m] = cm[m-1]*c1 - sm[m-1]*s1
		sm[m] = sm[m-1]*c1 + cm[m-1]*s1
	}
}

func apexFast(cc *ClimCoeffs, latD, lonD float64) (qdLatD, qdLonD float64) {
	var leg [nPA]float64
	var cm, sm [lmaxA + 1]float64
	s, x := math.Sincos((90 - latD) * deg)
	legendreFast(x, s, lmaxA, leg[:])
	sincosM(lonD*deg, lmaxA, cm[:], sm[:])
	var q [3]float64
	for l := 0; l <= lmaxA; l++ {
		base, pl := l*(l+1), pIdx(l, 0)
		for k := range 3 {
			q[k] += leg[pl] * cc.A[k][base]
		}
		for m := 1; m <= l; m++ {
			p := leg[pl+m]
			pc, ps := p*cm[m], p*sm[m]
			for k := range 3 {
				q[k] += pc*cc.A[k][base+m] + ps*cc.A[k][base-m]
			}
		}
	}
	qlat := clamp(latD+q[0], -90, 90)
	return qlat, math.Atan2(sm[1]+q[2], cm[1]+q[1]) / deg
}

// ClimDay is the once-per-day cache: each cell's QD coordinates and,
// optionally, its Legendre values P_lm(cos QD colatitude).
type ClimDay struct {
	g      *Grid
	x, s   []float64 // cos and sin of QD colatitude
	qdLonD []float64
	P64    []float64 // cells*465, if cached as float64
	P32    []float32 // cells*465, if cached as float32
}

// Legendre cache modes.
const (
	LegNone = iota
	Leg64
	Leg32
)

func NewClimDay(g *Grid, cc *ClimCoeffs, mode int) *ClimDay {
	n := g.Cols * g.Rows
	d := &ClimDay{g: g, x: make([]float64, n), s: make([]float64, n), qdLonD: make([]float64, n)}
	switch mode {
	case Leg64:
		d.P64 = make([]float64, n*nPC)
	case Leg32:
		d.P32 = make([]float32, n*nPC)
	}
	var leg [nPC]float64
	for r := range g.Rows {
		for c := range g.Cols {
			i := r*g.Cols + c
			qlat, qlon := apexFast(cc, g.latD[r], g.lonD[c])
			d.s[i], d.x[i] = math.Sincos((90 - qlat) * deg)
			d.qdLonD[i] = qlon
			switch mode {
			case Leg64:
				legendreFast(d.x[i], d.s[i], lmaxC, d.P64[i*nPC:(i+1)*nPC])
			case Leg32:
				legendreFast(d.x[i], d.s[i], lmaxC, leg[:])
				p := d.P32[i*nPC : (i+1)*nPC]
				for k, v := range leg {
					p[k] = float32(v)
				}
			}
		}
	}
	return d
}

// CacheBytes reports the cache's memory.
func (d *ClimDay) CacheBytes() int {
	return 8*(len(d.x)+len(d.s)+len(d.qdLonD)+len(d.P64)) + 4*len(d.P32)
}

type ClimWork struct {
	coef   [2][nSHC]float64
	leg    [nPC]float64
	cm, sm [lmaxC + 1]float64
}

// Hour evaluates one hour into fo and m3 (len cells). The solar-level blend is
// folded into the coefficients (the model is linear in them), and the Fourier
// terms are combined once per hour.
func (d *ClimDay) Hour(cc *ClimCoeffs, ut, w float64, wk *ClimWork, fo, m3 []float64) {
	g := d.g
	ffs := realFS(ut)
	for f := range 2 {
		c := &wk.coef[f]
		*c = [nSHC]float64{}
		for j := range nFS {
			a, b := (1-w)*ffs[j], w*ffs[j]
			lo, hi := &cc.C[0][f][j], &cc.C[1][f][j]
			for k := range nSHC {
				c[k] += a*lo[k] + b*hi[k]
			}
		}
	}
	cF, cM := &wk.coef[0], &wk.coef[1]
	sLat, sLon := subsolarGeo(ut)
	_, sQDLon := apexFast(cc, sLat, sLon)
	n := g.Cols * g.Rows
	P := wk.leg[:]
	for i := 0; i < n; i++ {
		switch {
		case d.P64 != nil:
			P = d.P64[i*nPC : (i+1)*nPC]
		case d.P32 != nil:
			src := d.P32[i*nPC : (i+1)*nPC]
			for k, v := range src {
				wk.leg[k] = float64(v)
			}
		default:
			legendreFast(d.x[i], d.s[i], lmaxC, wk.leg[:])
		}
		sincosM((d.qdLonD[i]-sQDLon+180)*deg, lmaxC, wk.cm[:], wk.sm[:])
		vf, vm := 0.0, 0.0
		for l := 0; l <= lmaxC; l++ {
			base, pl := l*(l+1), pIdx(l, 0)
			vf += P[pl] * cF[base]
			vm += P[pl] * cM[base]
			for m := 1; m <= l; m++ {
				p := P[pl+m]
				pc, ps := p*wk.cm[m], p*wk.sm[m]
				vf += pc*cF[base+m] + ps*cF[base-m]
				vm += pc*cM[base+m] + ps*cM[base-m]
			}
		}
		fo[i], m3[i] = vf, vm
	}
}
