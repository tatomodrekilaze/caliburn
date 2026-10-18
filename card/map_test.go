package main

import (
	"math"
	"testing"
)

func TestPotiSitsOnItsCoordinate(t *testing.T) {
	p := site("Poti")
	lat, lon := metresToLatLon(p.x, p.y)
	if math.Abs(lat-42.146) > 1e-6 || math.Abs(lon-41.672) > 1e-6 {
		t.Fatalf("poti %v %v", lat, lon)
	}
	tx, ty, _, _ := tileXY(lat, lon)
	if tx < 600 || tx > 700 || ty < 300 || ty > 450 {
		t.Fatalf("tile %d %d", tx, ty)
	}
}
