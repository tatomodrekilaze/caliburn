package main

import (
	"fmt"
	"math"
	"sort"
)

// One battery. A step is a half second: plots when the scan is due,
// then the aircraft. Reaching the standoff, or losing the track, is
// the order to leave. There is no impact mode.

const (
	dt      = 0.5
	leaveM  = 2500.0
	speed   = 95.0
	sigmaPr = 2200.0
	hardM   = 1500.0
	turnR   = 0.55
)

type truth struct {
	name         string
	kind         string
	x0, y0, z0   float64
	vx, vy       float64
	squawk       string
	followGround bool
	die          *float64
}

func (tr truth) alive(t float64) bool {
	if tr.die == nil {
		return true
	}
	return t < *tr.die
}

func (tr truth) pos(t float64) (float64, float64, float64) {
	x := tr.x0 + tr.vx*t
	y := tr.y0 + tr.vy*t
	if tr.followGround {
		return x, y, ground(x, y) + 160
	}
	return x, y, tr.z0
}

type craft struct {
	id       string
	home     string
	x, y     float64
	phase    string
	hdg      float64
	speed    float64
	minRange float64
	leaveFor float64
	track    int
	truth    string
	blind    int
}

type broadcast struct {
	x, y, z float64
	squawk  string
	name    string
}

type battery struct {
	name     string
	keys     bool
	tEnd     float64
	truths   []truth
	craft    []craft
	tracks   []track
	plots    []plot
	log      []string
	withheld map[int]bool
	pairs    [][2]string
	t        float64
	k        int
	steps    int
	rng      *pyRand
	trk      tracker
}

func newBattery(name string, truths []truth, keys bool, tEnd float64, seed uint32) *battery {
	return &battery{
		name:     name,
		keys:     keys,
		tEnd:     tEnd,
		truths:   truths,
		craft:    magazine(),
		withheld: map[int]bool{},
		steps:    int(tEnd / dt),
		rng:      newPyRand(seed),
		trk:      newTracker(),
	}
}

func magazine() []craft {
	poti := site("Poti")
	akh := site("Akhaltsikhe")
	out := make([]craft, 0, 4)
	for i := 0; i < 3; i++ {
		out = append(out, newCraft(fmt.Sprintf("P%d", i+1), "Poti", poti.x+2000+float64(i)*160, poti.y+500))
	}
	out = append(out, newCraft("A1", "Akhaltsikhe", akh.x-3500, akh.y+2200))
	return out
}

func newCraft(id, home string, x, y float64) craft {
	return craft{id: id, home: home, x: x, y: y, phase: "rail", speed: speed, minRange: 1e9}
}

func (b *battery) more() bool {
	return b.k < b.steps
}

func (b *battery) step() {
	if !b.more() {
		return
	}
	k := b.k
	t := math.Round(float64(k)*dt*1000) / 1000
	b.t = t
	if k%4 == 0 {
		b.plots = scan(b.truths, t, b.rng)
		b.tracks = b.trk.step(b.plots, t)
		live := map[int]track{}
		for _, tr := range b.tracks {
			live[tr.id] = tr
		}
		var pool []track
		busy := map[int]bool{}
		for _, c := range b.craft {
			if c.track != 0 {
				busy[c.track] = true
			}
		}
		for _, tr := range b.tracks {
			if !busy[tr.id] {
				pool = append(pool, tr)
			}
		}
		decision := pair(pool, b.craft, b.broadcasts(t), b.keys)
		truthFor := map[int]string{}
		for _, tr := range b.tracks {
			best := ""
			bestD := 1e9
			for _, th := range b.truths {
				x, y, _ := th.pos(t)
				d := (tr.x-x)*(tr.x-x) + (tr.y-y)*(tr.y-y)
				if d < bestD {
					bestD = d
					best = th.name
				}
			}
			if best != "" && bestD < 3500*3500 {
				truthFor[tr.id] = best
			}
		}
		for _, tid := range decision.withheld {
			b.withheld[tid] = true
		}
		for _, row := range decision.assign {
			taken := false
			for _, c := range b.craft {
				if c.track == row.track {
					taken = true
				}
			}
			if taken {
				continue
			}
			for i := range b.craft {
				if b.craft[i].id != row.craft {
					continue
				}
				launch(&b.craft[i], live[row.track], truthFor[row.track])
				b.pairs = append(b.pairs, [2]string{row.craft, fmt.Sprintf("%d", row.track)})
				b.log = append(b.log, fmt.Sprintf("t %.0f  %s takes track %d", t, row.craft, row.track))
				break
			}
		}
		for i := range b.craft {
			c := &b.craft[i]
			if c.phase != "fly" || c.track == 0 {
				continue
			}
			if _, ok := live[c.track]; !ok {
				c.blind++
			} else {
				c.blind = 0
			}
		}
	}

	byName := map[string]truth{}
	for _, th := range b.truths {
		byName[th.name] = th
	}
	for i := range b.craft {
		c := &b.craft[i]
		var guide *track
		if c.phase == "fly" && c.blind < 2 {
			guide = b.findTrack(c.track)
		}
		was := c.phase
		stepCraft(c, guide, dt)
		if was == "fly" && c.phase == "leave" {
			b.log = append(b.log, fmt.Sprintf("t %.0f  %s leaving", t, c.id))
		}
		if c.truth != "" && c.phase != "rail" {
			th, ok := byName[c.truth]
			if ok {
				x, y, _ := th.pos(t)
				d := math.Hypot(c.x-x, c.y-y)
				if d < c.minRange {
					c.minRange = d
				}
			}
		}
	}
	b.k++
}

func (b *battery) findTrack(id int) *track {
	for i := range b.tracks {
		if b.tracks[i].id == id {
			return &b.tracks[i]
		}
	}
	return nil
}

func (b *battery) broadcasts(t float64) []broadcast {
	var rows []broadcast
	for _, th := range b.truths {
		if th.squawk == "" || !th.alive(t) {
			continue
		}
		x, y, z := th.pos(t)
		rows = append(rows, broadcast{x, y, z, th.squawk, th.name})
	}
	return rows
}

func launch(c *craft, tr track, truthName string) {
	c.phase = "fly"
	c.track = tr.id
	c.truth = truthName
	c.hdg = math.Atan2(tr.y-c.y, tr.x-c.x)
	c.speed = speed
	c.minRange = 1e9
	c.blind = 0
}

func stepCraft(c *craft, tr *track, step float64) {
	if c.phase == "rail" {
		return
	}
	if c.phase == "chute" {
		c.speed += (12 - c.speed) * math.Min(1, step/4)
		c.x += math.Cos(c.hdg) * c.speed * step
		c.y += math.Sin(c.hdg) * c.speed * step
		c.leaveFor += step
		return
	}
	lost := tr == nil
	if tr != nil {
		dist := math.Hypot(tr.x-c.x, tr.y-c.y)
		if dist < c.minRange {
			c.minRange = dist
		}
		if dist < leaveM {
			lost = true
		} else {
			dx := tr.x - c.x
			dy := tr.y - c.y
			scale := (dist - leaveM) / dist
			tx := c.x + dx*scale
			ty := c.y + dy*scale
			want := math.Atan2(ty-c.y, tx-c.x)
			err := pyMod(want-c.hdg+math.Pi, 2*math.Pi) - math.Pi
			c.hdg += math.Max(-turnR*step, math.Min(turnR*step, err))
		}
	}
	if c.phase == "fly" && lost {
		c.phase = "leave"
		c.hdg += 2.4
		c.leaveFor = 0
	}
	c.x += math.Cos(c.hdg) * c.speed * step
	c.y += math.Sin(c.hdg) * c.speed * step
	if c.phase == "leave" {
		c.leaveFor += step
		if c.leaveFor > 10 {
			c.phase = "chute"
		}
	}
}

func standoff(px, py float64, tr track) (float64, float64) {
	dx := tr.x - px
	dy := tr.y - py
	dist := math.Hypot(dx, dy)
	if dist < leaveM+50 {
		return px, py
	}
	scale := (dist - leaveM) / dist
	return px + dx*scale, py + dy*scale
}

func threatens(tr track, asset place) bool {
	dx := asset.x - tr.x
	dy := asset.y - tr.y
	dist := math.Hypot(dx, dy)
	spd := tr.speed()
	if spd < 15 {
		return false
	}
	closing := (dx*tr.vx + dy*tr.vy) / spd
	if closing < 20 {
		return false
	}
	time := dist / closing
	along := closing * time
	miss := math.Hypot(dx-tr.vx/spd*along, dy-tr.vy/spd*along)
	return time < 420 && miss < 6000
}

func trackLabel(tr track, casts []broadcast) string {
	for _, item := range casts {
		if math.Hypot(tr.x-item.x, tr.y-item.y) < 900 {
			return "civil"
		}
	}
	if onFinal(tr.x, tr.y, tr.z) {
		return "withheld"
	}
	if tr.coast > 0 || tr.sigma() > sigmaPr {
		return "coasting"
	}
	if tr.hits < 6 {
		return "unknown"
	}
	if tr.P[2][2]+tr.P[3][3] > 400 {
		return "unknown"
	}
	return "hostile"
}

type decision struct {
	assign []struct {
		craft string
		track int
	}
	withheld []int
}

func pair(tracks []track, rounds []craft, casts []broadcast, keysOn bool) decision {
	labels := map[int]string{}
	var civil, held []int
	for _, tr := range tracks {
		name := trackLabel(tr, casts)
		labels[tr.id] = name
		if name == "civil" {
			civil = append(civil, tr.id)
		}
		if name == "withheld" {
			held = append(held, tr.id)
		}
	}
	_ = civil
	if !keysOn {
		return decision{withheld: held}
	}
	var hostile []track
	for _, tr := range tracks {
		if labels[tr.id] == "hostile" {
			hostile = append(hostile, tr)
		}
	}
	var free []craft
	for _, c := range rounds {
		if c.phase == "rail" {
			free = append(free, c)
		}
	}
	if len(hostile) == 0 || len(free) == 0 {
		return decision{withheld: held}
	}
	assets := []place{site("Supsa"), site("Akhaltsikhe"), site("Akhalkalaki")}
	type link struct {
		cost  float64
		craft string
		track int
	}
	var legal []link
	for _, tr := range hostile {
		threat := false
		for _, asset := range assets {
			if threatens(tr, asset) {
				threat = true
				break
			}
		}
		if !threat {
			continue
		}
		for _, c := range free {
			sx, sy := standoff(c.x, c.y, tr)
			if pathTouchesFinal(c.x, c.y, sx, sy, 400) || onFinal(sx, sy, 400) {
				continue
			}
			gap := math.Hypot(tr.x-c.x, tr.y-c.y)
			dist := math.Hypot(sx-c.x, sy-c.y)
			opening := ((tr.x-c.x)*tr.vx + (tr.y-c.y)*tr.vy) / math.Max(gap, 1)
			if opening > speed*0.8 || dist/speed > 170 {
				continue
			}
			legal = append(legal, link{dist / speed, c.id, tr.id})
		}
	}
	byTrack := map[int][]link{}
	for _, row := range legal {
		byTrack[row.track] = append(byTrack[row.track], row)
	}
	var trackIDs []int
	for id := range byTrack {
		trackIDs = append(trackIDs, id)
	}
	sort.Ints(trackIDs)
	craftSet := map[string]bool{}
	for _, row := range legal {
		craftSet[row.craft] = true
	}
	var craftIDs []string
	for id := range craftSet {
		craftIDs = append(craftIDs, id)
	}
	sort.Strings(craftIDs)
	if len(trackIDs) == 0 {
		return decision{withheld: held}
	}

	bestCost := 0.0
	var bestKey string
	var best []struct {
		craft string
		track int
	}
	have := false
	skip := 500.0
	nCraft := len(craftIDs)

	var walk func(i int, used map[string]bool, cost float64, mapping []struct {
		craft string
		track int
	})
	walk = func(i int, used map[string]bool, cost float64, mapping []struct {
		craft string
		track int
	}) {
		if i == len(trackIDs) {
			key := pairKey(mapping)
			if !have || cost < bestCost || (cost == bestCost && key < bestKey) {
				bestCost = cost
				bestKey = key
				best = append([]struct {
					craft string
					track int
				}(nil), mapping...)
				have = true
			}
			return
		}
		tid := trackIDs[i]
		for pick := 0; pick <= nCraft; pick++ {
			if pick == nCraft {
				walk(i+1, used, cost+skip, mapping)
				continue
			}
			cid := craftIDs[pick]
			if used[cid] {
				continue
			}
			found := -1.0
			ok := false
			for _, row := range byTrack[tid] {
				if row.craft == cid {
					found = row.cost
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
			next := map[string]bool{}
			for k, v := range used {
				next[k] = v
			}
			next[cid] = true
			row := struct {
				craft string
				track int
			}{cid, tid}
			walk(i+1, next, cost+found, append(append([]struct {
				craft string
				track int
			}{}, mapping...), row))
		}
	}
	walk(0, map[string]bool{}, 0, nil)
	return decision{assign: best, withheld: held}
}

func pairKey(mapping []struct {
	craft string
	track int
}) string {
	rows := append([]struct {
		craft string
		track int
	}(nil), mapping...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].craft != rows[j].craft {
			return rows[i].craft < rows[j].craft
		}
		return rows[i].track < rows[j].track
	})
	s := ""
	for _, row := range rows {
		s += fmt.Sprintf("%s/%d,", row.craft, row.track)
	}
	return s
}

func meet() (point, a, b [2]float64) {
	sup := site("Supsa")
	point = [2]float64{10000, -2000}
	dx := sup.x - point[0]
	dy := sup.y - point[1]
	dist := math.Hypot(dx, dy)
	ux, uy := dx/dist, dy/dist
	px, py := uy, -ux
	along, side := 46.0, 16.0
	a = [2]float64{ux*along + px*side, uy*along + py*side}
	b = [2]float64{ux*along - px*side, uy*along - py*side}
	return
}

func seaPair() (truth, truth) {
	point, va, vb := meet()
	const tMeet = 80.0
	a := truth{name: "sea-a", kind: "hostile", x0: point[0] - va[0]*tMeet, y0: point[1] - va[1]*tMeet, z0: 180, vx: va[0], vy: va[1]}
	b := truth{name: "sea-b", kind: "hostile", x0: point[0] - vb[0]*tMeet, y0: point[1] - vb[1]*tMeet, z0: 220, vx: vb[0], vy: vb[1]}
	return a, b
}

func ridgeTruth(spd float64) truth {
	dx, dy := 88000.0, -46000.0
	length := math.Hypot(dx, dy)
	hx, hy := dx/length, dy/length
	return truth{
		name: "ridge", kind: "hostile",
		x0: 38400 - hx*8000, y0: -15800 - hy*8000,
		vx: hx * spd, vy: hy * spd,
		followGround: true,
	}
}

func arrival() truth {
	x, y := faf()
	cx, cy := course()
	return truth{name: "arrival", kind: "civil", x0: x, y0: y, z0: 640, vx: cx * 72, vy: cy * 72, squawk: "4261"}
}

func insideFinal() truth {
	x, y := faf()
	cx, cy := course()
	return truth{name: "in-final", kind: "hostile", x0: x + cx*4000, y0: y + cy*4000, z0: 280, vx: cx * 42, vy: cy * 42}
}

func oneSea(die float64) truth {
	point, va, _ := meet()
	const tMeet = 90.0
	d := die
	return truth{
		name: "sea-a", kind: "hostile",
		x0: point[0] - va[0]*tMeet, y0: point[1] - va[1]*tMeet, z0: 200,
		vx: va[0], vy: va[1], die: &d,
	}
}

func openCase(which int) *battery {
	switch which {
	case 1:
		a, b := seaPair()
		return newBattery("one-key", []truth{a, b, arrival()}, false, 36, 4)
	case 2:
		return newBattery("in-final", []truth{insideFinal(), arrival()}, true, 36, 4)
	case 3:
		return newBattery("lost", []truth{oneSea(26)}, true, 70, 4)
	default:
		a, b := seaPair()
		return newBattery("nominal", []truth{a, b, ridgeTruth(150), arrival()}, true, 260, 4)
	}
}

func caseTitle(name string) string {
	switch name {
	case "nominal":
		return "Nominal"
	case "one-key":
		return "One key"
	case "in-final":
		return "On the final"
	case "lost":
		return "Track lost"
	default:
		return name
	}
}
