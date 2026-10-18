package main

import (
	"math"
	"sort"
)

// Poti watches the sea. Akhaltsikhe watches the yard. A miss in the
// shadow of Zekari is a coast, not a new track.

const (
	scanS      = 2.0
	sigmaR     = 42.0
	sigmaAz    = 0.45 * math.Pi / 180
	gate       = 13.5
	confirmHit = 3
	visMiss    = 4
	coastLimit = 210.0
	accel      = 1.6
)

type plot struct {
	radar    string
	x, y, z  float64
	r, az, t float64
	clutter  bool
}

type track struct {
	id      int
	t       float64
	x, y, z float64
	vx, vy  float64
	hits    int
	missVis int
	coast   int
	dropped bool
	P       [4][4]float64
}

func (tr track) speed() float64 {
	return math.Hypot(tr.vx, tr.vy)
}

func (tr track) sigma() float64 {
	return math.Sqrt(math.Max(1, tr.P[0][0]+tr.P[1][1]))
}

type tent struct {
	x0, y0, t0 float64
	x, y, z, t float64
	hits       int
}

type hyp struct {
	tracks []track
	tents  []tent
	score  float64
	t      float64
	nextID int
}

func (h hyp) clone() hyp {
	h.tracks = append([]track(nil), h.tracks...)
	h.tents = append([]tent(nil), h.tents...)
	return h
}

type tracker struct {
	hyps []hyp
	t    float64
}

func newTracker() tracker {
	return tracker{hyps: []hyp{{nextID: 1}}}
}

func scan(truths []truth, t float64, rng *pyRand) []plot {
	var plots []plot
	for _, r := range radars {
		for _, tr := range truths {
			if !tr.alive(t) {
				continue
			}
			x, y, z := tr.pos(t)
			if !visibleFrom(r, x, y, z) {
				continue
			}
			if rng.random() > 0.93 {
				continue
			}
			dx := x - r.x
			dy := y - r.y
			rngR := math.Hypot(dx, dy)
			az := math.Atan2(dx, dy)
			rngR = math.Max(200, rngR+rng.gauss(0, sigmaR))
			az += rng.gauss(0, sigmaAz)
			plots = append(plots, plot{
				radar: r.name,
				x:     r.x + rngR*math.Sin(az),
				y:     r.y + rngR*math.Cos(az),
				z:     z + rng.gauss(0, 25),
				r:     rngR,
				az:    az,
				t:     t,
			})
		}
		if r.name != "Poti" {
			continue
		}
		for n := 0; n < 5; n++ {
			rr := rng.uniform(2000, 28000)
			az := rng.uniform(-2.4, 1.2)
			plots = append(plots, plot{
				radar:   "Poti",
				x:       r.x + rr*math.Sin(az),
				y:       r.y + rr*math.Cos(az),
				z:       rng.uniform(40, 300),
				r:       rr,
				az:      az,
				t:       t,
				clutter: true,
			})
		}
	}
	sort.SliceStable(plots, func(i, j int) bool {
		if plots[i].radar != plots[j].radar {
			return plots[i].radar < plots[j].radar
		}
		ri, rj := pyRound(plots[i].x), pyRound(plots[j].x)
		if ri != rj {
			return ri < rj
		}
		return pyRound(plots[i].y) < pyRound(plots[j].y)
	})
	return plots
}

func predict(tr *track, dt float64) {
	if dt <= 0 {
		return
	}
	tr.x += tr.vx * dt
	tr.y += tr.vy * dt
	tr.t += dt
	var f [4][4]float64
	for i := 0; i < 4; i++ {
		f[i][i] = 1
	}
	f[0][2] = dt
	f[1][3] = dt
	p := mul(mul(f, tr.P), transpose(f))
	q := accel * accel
	dt2 := dt * dt
	dt3 := dt2 * dt
	dt4 := dt2 * dt2
	q00 := q * dt4 / 4
	q02 := q * dt3 / 2
	q22 := q * dt2
	p[0][0] += q00
	p[0][2] += q02
	p[2][0] += q02
	p[2][2] += q22
	p[1][1] += q00
	p[1][3] += q02
	p[3][1] += q02
	p[3][3] += q22
	tr.P = p
}

func measR(p plot) (a, b, c, d float64) {
	r := p.r
	if r < 200 {
		r = 200
	}
	ux, uy := math.Sin(p.az), math.Cos(p.az)
	px, py := math.Cos(p.az), -math.Sin(p.az)
	along := sigmaR * sigmaR
	cross := (r * sigmaAz) * (r * sigmaAz)
	a = along*ux*ux + cross*px*px
	b = along*ux*uy + cross*px*py
	c = along*uy*ux + cross*py*px
	d = along*uy*uy + cross*py*py
	return
}

func inv2(a, b, c, d float64) (float64, float64, float64, float64) {
	det := a*d - b*c
	if math.Abs(det) < 1e-9 {
		if det >= 0 {
			det = 1e-9
		} else {
			det = -1e-9
		}
	}
	return d / det, -b / det, -c / det, a / det
}

func mahal(tr track, p plot) float64 {
	mx := p.x - tr.x
	my := p.y - tr.y
	ra, rb, rc, rd := measR(p)
	ia, ib, ic, id := inv2(tr.P[0][0]+ra, tr.P[0][1]+rb, tr.P[1][0]+rc, tr.P[1][1]+rd)
	return mx*(ia*mx+ib*my) + my*(ic*mx+id*my)
}

func update(tr *track, p plot) {
	mx := p.x - tr.x
	my := p.y - tr.y
	ra, rb, rc, rd := measR(p)
	ia, ib, ic, id := inv2(tr.P[0][0]+ra, tr.P[0][1]+rb, tr.P[1][0]+rc, tr.P[1][1]+rd)
	var k [4][2]float64
	for i := 0; i < 4; i++ {
		k[i][0] = tr.P[i][0]*ia + tr.P[i][1]*ic
		k[i][1] = tr.P[i][0]*ib + tr.P[i][1]*id
	}
	tr.x += k[0][0]*mx + k[0][1]*my
	tr.y += k[1][0]*mx + k[1][1]*my
	tr.vx += k[2][0]*mx + k[2][1]*my
	tr.vy += k[3][0]*mx + k[3][1]*my
	var eye [4][4]float64
	for i := 0; i < 4; i++ {
		eye[i][i] = 1
		eye[i][0] -= k[i][0]
		eye[i][1] -= k[i][1]
	}
	tr.P = mul(eye, tr.P)
	for i := 0; i < 4; i++ {
		if tr.P[i][i] < 1 {
			tr.P[i][i] = 1
		}
	}
	tr.z = 0.65*tr.z + 0.35*p.z
	tr.hits++
	tr.missVis = 0
	tr.coast = 0
}

func masked(tr track) bool {
	points := [][3]float64{{tr.x, tr.y, tr.z}}
	for _, ahead := range []float64{3, 7, 12} {
		x := tr.x + tr.vx*ahead
		y := tr.y + tr.vy*ahead
		z := tr.z
		g := ground(x, y)
		if z < g+80 {
			z = g + 160
		}
		points = append(points, [3]float64{x, y, z})
	}
	for _, p := range points {
		if !anyoneSees(p[0], p[1], p[2]) {
			return true
		}
	}
	return false
}

func missCost(tr track) float64 {
	if masked(tr) {
		return 0.35
	}
	return 8
}

type asg struct {
	id int
	pi int
}

func assign(tracks []track, plots []plot) (mapping []asg, cost float64, alt []asg, altCost float64, hasAlt bool) {
	var live []track
	for _, tr := range tracks {
		if !tr.dropped {
			live = append(live, tr)
		}
	}
	sort.Slice(live, func(i, j int) bool { return live[i].id < live[j].id })
	if len(live) == 0 {
		return nil, 0, nil, 0, false
	}

	type opt struct {
		d2 float64
		pi int
	}
	options := make([][]opt, len(live))
	for i, tr := range live {
		var opts []opt
		for pi, p := range plots {
			d2 := mahal(tr, p)
			if d2 < gate {
				opts = append(opts, opt{d2, pi})
			}
		}
		sort.Slice(opts, func(a, b int) bool { return opts[a].d2 < opts[b].d2 })
		if len(opts) > 3 {
			opts = opts[:3]
		}
		opts = append(opts, opt{pi: -1})
		options[i] = opts
	}

	var bestCost float64
	var bestKey []int
	var bestMap []asg
	var secondCost float64
	var secondKey []int
	var secondMap []asg
	haveBest := false
	haveSecond := false

	lessKey := func(a, b []int) bool {
		n := len(a)
		if len(b) < n {
			n = len(b)
		}
		for i := 0; i < n; i++ {
			if a[i] != b[i] {
				return a[i] < b[i]
			}
		}
		return len(a) < len(b)
	}
	better := func(c float64, k []int, oc float64, ok []int) bool {
		if c != oc {
			return c < oc
		}
		return lessKey(k, ok)
	}
	consider := func(c float64, mapping []asg) {
		key := make([]int, len(mapping))
		saved := append([]asg(nil), mapping...)
		for i, m := range mapping {
			if m.pi < 0 {
				key[i] = -1
			} else {
				key[i] = m.pi
			}
		}
		if !haveBest || better(c, key, bestCost, bestKey) {
			secondCost, secondKey, secondMap, haveSecond = bestCost, bestKey, bestMap, haveBest
			bestCost, bestKey, bestMap, haveBest = c, key, saved, true
			return
		}
		if !haveSecond || better(c, key, secondCost, secondKey) {
			secondCost, secondKey, secondMap, haveSecond = c, key, saved, true
		}
	}

	var walk func(i int, used map[int]bool, c float64, mapping []asg)
	walk = func(i int, used map[int]bool, c float64, mapping []asg) {
		if i == len(live) {
			consider(c, mapping)
			return
		}
		tr := live[i]
		for _, o := range options[i] {
			if o.pi >= 0 && used[o.pi] {
				continue
			}
			extra := o.d2
			next := used
			if o.pi < 0 {
				extra = missCost(tr)
			} else {
				next = make(map[int]bool, len(used)+1)
				for k, v := range used {
					next[k] = v
				}
				next[o.pi] = true
			}
			walk(i+1, next, c+extra, append(append([]asg{}, mapping...), asg{tr.id, o.pi}))
		}
	}
	walk(0, map[int]bool{}, 0, nil)

	has := haveSecond && secondCost-bestCost < 2 && !sameKey(secondKey, bestKey)
	return bestMap, bestCost, secondMap, secondCost, has
}

func sameKey(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func apply(h *hyp, mapping []asg, plots []plot, t float64) {
	byID := map[int]int{}
	for i := range h.tracks {
		byID[h.tracks[i].id] = i
	}
	used := map[int]bool{}
	for _, m := range mapping {
		idx, ok := byID[m.id]
		if !ok || h.tracks[idx].dropped {
			continue
		}
		if m.pi < 0 {
			tr := &h.tracks[idx]
			if masked(*tr) {
				tr.coast++
				tr.missVis = 0
				if float64(tr.coast)*scanS > coastLimit {
					tr.dropped = true
				}
			} else {
				tr.coast = 0
				tr.missVis++
				if tr.missVis >= visMiss {
					tr.dropped = true
				}
			}
			continue
		}
		update(&h.tracks[idx], plots[m.pi])
		used[m.pi] = true
	}

	for pi, p := range plots {
		if used[pi] {
			continue
		}
		for i := range h.tracks {
			if h.tracks[i].dropped {
				continue
			}
			if mahal(h.tracks[i], p) < gate {
				update(&h.tracks[i], p)
				used[pi] = true
				break
			}
		}
	}

	var still []tent
	for _, tent := range h.tents {
		if t-tent.t > 8 {
			continue
		}
		grabbed := -1
		bestD := 1e9
		for pi, p := range plots {
			if used[pi] {
				continue
			}
			d := math.Hypot(p.x-tent.x, p.y-tent.y)
			if d < bestD && d < 800 {
				bestD = d
				grabbed = pi
			}
		}
		if grabbed < 0 {
			still = append(still, tent)
			continue
		}
		p := plots[grabbed]
		used[grabbed] = true
		tent.x, tent.y, tent.z, tent.t = p.x, p.y, p.z, t
		tent.hits++
		if tent.hits >= confirmHit {
			dt := t - tent.t0
			if dt < 0.5 {
				dt = 0.5
			}
			vx := (tent.x - tent.x0) / dt
			vy := (tent.y - tent.y0) / dt
			spd := math.Hypot(vx, vy)
			if spd >= 18 && spd <= 230 {
				h.tracks = append(h.tracks, newTrack(h.nextID, tent.x, tent.y, tent.z, vx, vy, t))
				h.nextID++
			}
		} else {
			still = append(still, tent)
		}
	}
	h.tents = still

	for pi, p := range plots {
		if used[pi] {
			continue
		}
		near := false
		for _, tr := range h.tracks {
			if !tr.dropped && math.Hypot(p.x-tr.x, p.y-tr.y) < 500 {
				near = true
				break
			}
		}
		if !near {
			h.tents = append(h.tents, tent{
				x0: p.x, y0: p.y, t0: p.t,
				x: p.x, y: p.y, z: p.z, t: p.t,
				hits: 1,
			})
		}
	}
	kept := h.tracks[:0]
	for _, tr := range h.tracks {
		if !tr.dropped {
			kept = append(kept, tr)
		}
	}
	h.tracks = kept
	h.t = t
}

func newTrack(id int, x, y, z, vx, vy, t float64) track {
	tr := track{id: id, t: t, x: x, y: y, z: z, vx: vx, vy: vy, hits: confirmHit}
	tr.P[0][0] = 80 * 80
	tr.P[1][1] = 80 * 80
	tr.P[2][2] = 8 * 8
	tr.P[3][3] = 8 * 8
	return tr
}

func mul(a, b [4][4]float64) [4][4]float64 {
	var out [4][4]float64
	for i := 0; i < 4; i++ {
		for k := 0; k < 4; k++ {
			aik := a[i][k]
			if aik == 0 {
				continue
			}
			for j := 0; j < 4; j++ {
				out[i][j] += aik * b[k][j]
			}
		}
	}
	return out
}

func transpose(a [4][4]float64) [4][4]float64 {
	var out [4][4]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			out[j][i] = a[i][j]
		}
	}
	return out
}

func (tr *tracker) step(plots []plot, t float64) []track {
	dt := t - tr.t
	var nxt []hyp
	for i := range tr.hyps {
		h := &tr.hyps[i]
		for j := range h.tracks {
			predict(&h.tracks[j], dt)
		}
		mapping, cost, alt, altCost, hasAlt := assign(h.tracks, plots)
		keep := h.clone()
		apply(&keep, mapping, plots, t)
		keep.score -= cost
		nxt = append(nxt, keep)
		if hasAlt {
			other := h.clone()
			apply(&other, alt, plots, t)
			other.score -= altCost
			nxt = append(nxt, other)
		}
	}
	sort.SliceStable(nxt, func(i, j int) bool {
		if nxt[i].score != nxt[j].score {
			return nxt[i].score > nxt[j].score
		}
		return nxt[i].nextID < nxt[j].nextID
	})
	if len(nxt) > 6 {
		nxt = nxt[:6]
	}
	tr.hyps = nxt
	tr.t = t
	return tr.live()
}

func (tr *tracker) live() []track {
	if len(tr.hyps) == 0 {
		return nil
	}
	rows := append([]track(nil), tr.hyps[0].tracks...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	return rows
}
