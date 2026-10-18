package main

import "math"

// Local metres. Origin is near Poti. x grows east, y grows north.

const (
	lat0      = 42.10
	lon0      = 41.70
	mLat      = 111320.0
	ridgePeak = 2350.0
	ridgeSig  = 3400.0
	guriaPeak = 780.0
	guriaSig  = 10500.0
	earthR    = 6371000.0
	clearance = 18.0

	finalM    = 17000.0
	halfWidth = 1400.0
	roofM     = 1500.0
)

var mLon = 111320.0 * math.Cos(lat0*math.Pi/180)

type place struct {
	name string
	x, y float64
}

type radar struct {
	name    string
	x, y, z float64
	reach   float64
}

var (
	sites  map[string]place
	radars []radar
	crest  = [][2]float64{
		{46000, -16000},
		{70000, -28000},
		{95200, -30600},
		{102000, -40000},
	}
	guria = [2]float64{26000, -24000}
)

func init() {
	raw := []struct {
		name     string
		lat, lon float64
	}{
		{"Poti", 42.146, 41.672},
		{"Supsa", 42.016, 41.758},
		{"Kutaisi", 42.1767, 42.4826},
		{"Batumi", 41.6103, 41.5996},
		{"Zestafoni", 42.109, 43.038},
		{"Akhaltsikhe", 41.639, 42.986},
		{"Akhalkalaki", 41.405, 43.486},
		{"Zekari", 41.825, 42.852},
	}
	sites = make(map[string]place, len(raw))
	for _, row := range raw {
		x, y := xy(row.lat, row.lon)
		sites[row.name] = place{row.name, x, y}
	}
	poti := site("Poti")
	akh := site("Akhaltsikhe")
	radars = []radar{
		{"Poti", poti.x + 1800, poti.y - 600, ground(poti.x, poti.y) + 36, 95000},
		{"Akhaltsikhe", akh.x - 2400, akh.y + 1600, ground(akh.x, akh.y) + 28, 80000},
	}
}

func xy(lat, lon float64) (float64, float64) {
	return (lon - lon0) * mLon, (lat - lat0) * mLat
}

func site(name string) place {
	return sites[name]
}

func alongCross(x, y float64, a, b [2]float64) (float64, float64, float64) {
	dx := b[0] - a[0]
	dy := b[1] - a[1]
	length := math.Hypot(dx, dy)
	along := ((x-a[0])*dx + (y-a[1])*dy) / length
	cross := ((x-a[0])*-dy + (y-a[1])*dx) / length
	return along, cross, length
}

func ground(x, y float64) float64 {
	h := 0.0
	if x > 2000 {
		rise := (x - 2000) * 0.0028
		if rise > 380 {
			rise = 380
		}
		h += rise
	}
	dx := x - guria[0]
	dy := y - guria[1]
	h += guriaPeak * math.Exp(-(dx*dx+dy*dy)/(2*guriaSig*guriaSig))

	ridge := 0.0
	for i := 0; i < len(crest)-1; i++ {
		along, cross, length := alongCross(x, y, crest[i], crest[i+1])
		if along < -5000 || along > length+5000 {
			continue
		}
		edge := 1.0
		if along < 0 {
			edge = math.Exp(-(along * along) / (2 * 4000 * 4000))
		} else if along > length {
			over := along - length
			edge = math.Exp(-(over * over) / (2 * 4000 * 4000))
		}
		piece := ridgePeak * edge * math.Exp(-(cross*cross)/(2*ridgeSig*ridgeSig))
		if piece > ridge {
			ridge = piece
		}
	}
	h += ridge

	akh := site("Akhaltsikhe")
	dakh := math.Hypot(x-akh.x, y-akh.y)
	h += 620 * math.Exp(-(dakh*dakh)/(2*18000*18000))

	jav := site("Akhalkalaki")
	djav := math.Hypot(x-jav.x, y-jav.y)
	h += 900 * math.Exp(-(djav*djav)/(2*26000*26000))
	return h
}

func sees(ax, ay, az, bx, by, bz float64) bool {
	dist := math.Hypot(bx-ax, by-ay)
	if dist < 80 {
		return true
	}
	n := int(dist / 450)
	if n < 2 {
		n = 2
	}
	for i := 1; i < n; i++ {
		t := float64(i) / float64(n)
		x := ax + (bx-ax)*t
		y := ay + (by-ay)*t
		zRay := az + (bz-az)*t
		d1 := t * dist
		d2 := (1 - t) * dist
		bulge := d1 * d2 / (2 * earthR)
		if ground(x, y)+clearance > zRay-bulge {
			return false
		}
	}
	return true
}

func visibleFrom(r radar, x, y, z float64) bool {
	if z < ground(x, y)+40 {
		return false
	}
	dist := math.Hypot(x-r.x, y-r.y)
	if dist > r.reach || dist < 400 {
		return false
	}
	return sees(r.x, r.y, r.z, x, y, z)
}

func anyoneSees(x, y, z float64) bool {
	for _, r := range radars {
		if visibleFrom(r, x, y, z) {
			return true
		}
	}
	return false
}

func course() (float64, float64) {
	h := 250 * math.Pi / 180
	return math.Sin(h), math.Cos(h)
}

func threshold() (float64, float64) {
	p := site("Kutaisi")
	return p.x, p.y
}

func faf() (float64, float64) {
	x, y := threshold()
	cx, cy := course()
	return x - cx*finalM, y - cy*finalM
}

func onFinal(x, y, z float64) bool {
	if z > roofM {
		return false
	}
	x0, y0 := faf()
	x1, y1 := threshold()
	dx := x1 - x0
	dy := y1 - y0
	length := math.Hypot(dx, dy)
	along := ((x-x0)*dx + (y-y0)*dy) / length
	cross := ((x-x0)*-dy + (y-y0)*dx) / length
	if along < -400 || along > length+400 {
		return false
	}
	return math.Abs(cross) < halfWidth
}

func pathTouchesFinal(x0, y0, x1, y1, z float64) bool {
	for i := 0; i < 9; i++ {
		t := float64(i) / 8
		if onFinal(x0+(x1-x0)*t, y0+(y1-y0)*t, z) {
			return true
		}
	}
	return false
}
