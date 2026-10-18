package main

import (
	"math"
	"testing"
)

func TestPythonRandom(t *testing.T) {
	r := newPyRand(4)
	want := []float64{0.23604808973743452, 0.1031660342307158, 0.396058242610681, 0.15497227080241027}
	for i, v := range want {
		got := r.random()
		if math.Abs(got-v) > 1e-12 {
			t.Fatalf("random %d got %v want %v", i, got, v)
		}
	}
	g := newPyRand(4)
	gauss := []float64{0.040856, 0.464865, -0.460898, 0.352626}
	for i, v := range gauss {
		got := g.gauss(0, 1)
		if math.Abs(got-v) > 1e-5 {
			t.Fatalf("gauss %d got %v want %v", i, got, v)
		}
	}
}

func TestGround(t *testing.T) {
	kut := site("Kutaisi")
	akh := site("Akhaltsikhe")
	sup := site("Supsa")
	if ground(kut.x, kut.y) >= 400 {
		t.Fatalf("kutaisi ground %v", ground(kut.x, kut.y))
	}
	h := ground(akh.x, akh.y)
	if h <= 800 || h >= 1600 {
		t.Fatalf("akhaltsikhe ground %v", h)
	}
	var poti radar
	for _, r := range radars {
		if r.name == "Poti" {
			poti = r
		}
	}
	if !sees(poti.x, poti.y, poti.z, sup.x, sup.y, ground(sup.x, sup.y)+30) {
		t.Fatal("poti should see supsa")
	}
	if sees(poti.x, poti.y, poti.z, akh.x, akh.y, ground(akh.x, akh.y)+20) {
		t.Fatal("poti should not see the yard")
	}
	th := ridgeTruth(150)
	seen, dark := 0, 0
	for i := 0; i < 180; i += 2 {
		x, y, z := th.pos(float64(i))
		if anyoneSees(x, y, z) {
			seen++
		} else {
			dark++
		}
	}
	if seen <= 8 || dark <= 8 {
		t.Fatalf("ridge seen %d dark %d", seen, dark)
	}
}

func TestEngagements(t *testing.T) {
	nom := openCase(0)
	var early, late, meet []track
	var earlyT, lateT, meetT float64
	var coasted bool
	for nom.more() {
		nom.step()
		if early == nil && nom.t >= 20 {
			early = append([]track(nil), nom.tracks...)
			earlyT = nom.t
		}
		if late == nil && nom.t >= 200 {
			late = append([]track(nil), nom.tracks...)
			lateT = nom.t
		}
		if meet == nil && nom.t >= 90 {
			meet = append([]track(nil), nom.tracks...)
			meetT = nom.t
		}
		if early != nil {
			id, _, _, ok := nearest(early, nom, "ridge", earlyT)
			if ok {
				for _, tr := range nom.tracks {
					if tr.id == id && tr.coast >= 4 {
						coasted = true
					}
				}
			}
		}
	}
	sea := map[string]bool{}
	for _, c := range nom.craft {
		if c.phase == "rail" {
			continue
		}
		sea[c.truth] = true
		if c.home != "Poti" {
			continue
		}
		if c.minRange <= hardM || c.minRange >= 7000 || (c.phase != "leave" && c.phase != "chute") {
			t.Fatalf("craft %+v", c)
		}
	}
	if len(nom.pairs) < 2 || len(sea) != 2 || !sea["sea-a"] || !sea["sea-b"] {
		t.Fatalf("pairs %v flown %v log %v", nom.pairs, sea, nom.log)
	}
	idA, dA, _, okA := nearest(early, nom, "ridge", earlyT)
	idB, dB, _, okB := nearest(late, nom, "ridge", lateT)
	if !okA || !okB || idA != idB || dA >= 2500 || dB >= 4000 || !coasted {
		t.Fatalf("ridge %d/%v %d/%v coast %v", idA, dA, idB, dB, coasted)
	}
	ids := map[int]bool{}
	for _, name := range []string{"sea-a", "sea-b"} {
		id, d, _, ok := nearest(meet, nom, name, meetT)
		if !ok || d >= 2000 {
			t.Fatalf("%s dist %v", name, d)
		}
		ids[id] = true
	}
	if len(ids) != 2 {
		t.Fatalf("sea ids %v", ids)
	}

	key := openCase(1)
	for key.more() {
		key.step()
	}
	if len(key.pairs) != 0 || flying(key) != 0 {
		t.Fatalf("one key launched %v", key.pairs)
	}

	fin := openCase(2)
	for fin.more() {
		fin.step()
	}
	if len(fin.pairs) != 0 || len(fin.withheld) == 0 || flying(fin) != 0 {
		t.Fatalf("final pairs %v withheld %v", fin.pairs, fin.withheld)
	}

	lost := openCase(3)
	for lost.more() {
		lost.step()
	}
	if flying(lost) != 1 {
		t.Fatalf("lost flown %d", flying(lost))
	}
	for _, c := range lost.craft {
		if c.phase == "rail" {
			continue
		}
		if (c.phase != "leave" && c.phase != "chute") || c.minRange <= hardM {
			t.Fatalf("lost %+v", c)
		}
	}

	a := openCase(0)
	b := openCase(0)
	var tracksA, tracksB []track
	var craftA, craftB []craft
	for a.more() && b.more() {
		a.step()
		b.step()
		if a.t >= 60 && tracksA == nil {
			tracksA = append([]track(nil), a.tracks...)
			craftA = append([]craft(nil), a.craft...)
			tracksB = append([]track(nil), b.tracks...)
			craftB = append([]craft(nil), b.craft...)
		}
	}
	if len(a.pairs) != len(b.pairs) {
		t.Fatalf("replay pairs %v %v", a.pairs, b.pairs)
	}
	for i := range a.pairs {
		if a.pairs[i] != b.pairs[i] {
			t.Fatalf("replay %v %v", a.pairs, b.pairs)
		}
	}
	if len(tracksA) != len(tracksB) {
		t.Fatalf("tracks %d %d", len(tracksA), len(tracksB))
	}
	for i := range tracksA {
		if tracksA[i].id != tracksB[i].id || tracksA[i].x != tracksB[i].x || craftA[i].x != craftB[i].x && i < len(craftA) {
			t.Fatalf("frame drift")
		}
	}
}

func flying(b *battery) int {
	n := 0
	for _, c := range b.craft {
		if c.phase != "rail" {
			n++
		}
	}
	return n
}

func nearest(rows []track, b *battery, name string, t float64) (int, float64, int, bool) {
	var tx, ty float64
	found := false
	for _, th := range b.truths {
		if th.name == name {
			tx, ty, _ = th.pos(t)
			found = true
		}
	}
	if !found {
		return 0, 0, 0, false
	}
	bestI := -1
	best := 1e18
	for i, tr := range rows {
		d := (tr.x-tx)*(tr.x-tx) + (tr.y-ty)*(tr.y-ty)
		if d < best {
			best = d
			bestI = i
		}
	}
	if bestI < 0 {
		return 0, 0, 0, false
	}
	return rows[bestI].id, math.Sqrt(best), rows[bestI].coast, true
}
