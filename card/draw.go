//go:build windows

package main

import (
	"fmt"
	"math"
	"time"
	"unsafe"
)

// The ground is the satellite picture. Brass is ours.
// Broadcasts are marked and listed. They are not tasked.

const (
	worldMinX = -18000.0
	worldMaxX = 160000.0
	worldMinY = -90000.0
	worldMaxY = 28000.0
	lipTop    = 46
	lipBot    = 40
)

func rgb(r, g, b byte) uintptr {
	return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16
}

var (
	glass  = rgb(12, 18, 16)
	paper  = rgb(216, 226, 218)
	dimC   = rgb(128, 148, 138)
	brass  = rgb(214, 164, 72)
	civilC = rgb(186, 228, 236)
	warnC  = rgb(232, 132, 96)
	plate  = rgb(8, 12, 10)
)

type faces struct {
	title uintptr
	body  uintptr
	mark  uintptr
}

var (
	reliefW   int
	reliefH   int
	reliefMem uintptr
	reliefDIB uintptr
	reliefGen uint64
)

func draw(hdc uintptr, w, h int, font faces) {
	mapH := h - lipTop - lipBot
	if mapH < 80 {
		mapH = h
	}
	fill(hdc, 0, 0, w, h, glass)
	ensureRelief(hdc, w, mapH)
	if reliefMem != 0 {
		bitBlt.Call(hdc, 0, lipTop, uintptr(w), uintptr(mapH), reliefMem, 0, 0, srcCopy)
	} else {
		fill(hdc, 0, lipTop, w, mapH, rgb(24, 36, 32))
	}
	paintAir(hdc, w, mapH, font)
	paintBoard(hdc, w, mapH, font)
	paintLips(hdc, w, h, mapH, font)
}

func project(x, y float64, w, h int) (int, int) {
	sx := (x - worldMinX) / (worldMaxX - worldMinX)
	sy := (y - worldMinY) / (worldMaxY - worldMinY)
	return int(sx * float64(w)), h - int(sy*float64(h))
}

func unproject(px, py, w, h int) (float64, float64) {
	x := worldMinX + (float64(px)+0.5)/float64(w)*(worldMaxX-worldMinX)
	y := worldMinY + (1-(float64(py)+0.5)/float64(h))*(worldMaxY-worldMinY)
	return x, y
}

func at(x, y float64, w, h int) (int, int) {
	px, py := project(x, y, w, h)
	return px, py + lipTop
}

func ensureRelief(hdc uintptr, w, h int) {
	if w < 8 || h < 8 {
		return
	}
	viewW.Store(int32(w))
	viewH.Store(int32(h))
	pw, ph, pix, gen := copyMap()
	if pix == nil || pw != w || ph != h || len(pix) < w*h*4 {
		if reliefW != w || reliefH != h {
			clearRelief()
		}
		return
	}
	if reliefDIB != 0 && reliefW == w && reliefH == h && reliefGen == gen {
		return
	}
	clearRelief()
	bi := bitmapInfoHeader{
		size:     40,
		width:    int32(w),
		height:   -int32(h),
		planes:   1,
		bitCount: 32,
	}
	var bits *byte
	dib, _, _ := createDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	mem, _, _ := createCompatible.Call(hdc)
	if dib == 0 || mem == 0 || bits == nil {
		if dib != 0 {
			deleteObject.Call(dib)
		}
		if mem != 0 {
			deleteDC.Call(mem)
		}
		return
	}
	selectObject.Call(mem, dib)
	buf := unsafe.Slice(bits, w*h*4)
	copy(buf, pix[:w*h*4])
	reliefW, reliefH = w, h
	reliefMem, reliefDIB = mem, dib
	reliefGen = gen
}

func clearRelief() {
	if reliefMem != 0 {
		deleteDC.Call(reliefMem)
	}
	if reliefDIB != 0 {
		deleteObject.Call(reliefDIB)
	}
	reliefMem, reliefDIB = 0, 0
	reliefW, reliefH = 0, 0
	reliefGen = 0
}

func shade(x, y float64) (byte, byte, byte) {
	if x < 0 {
		return 14, 46, 54
	}
	h := ground(x, y)
	t := h / 2400
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	t = math.Sqrt(t)
	return byte(22 + t*70), byte(34 + t*78), byte(30 + t*52)
}

func paintAir(hdc uintptr, w, h int, font faces) {
	for _, r := range radars {
		ringWorld(hdc, r.x, r.y, r.reach, w, h, rgb(244, 248, 244))
		x, y := at(r.x, r.y, w, h)
		ring(hdc, x, y, 11, brass, 2)
		disc(hdc, x, y, 4, brass)
	}
	fx, fy := faf()
	tx, ty := threshold()
	x0, y0 := at(fx, fy, w, h)
	x1, y1 := at(tx, ty, w, h)
	dashed(hdc, x0, y0, x1, y1, brass, 2)

	for _, c := range magazine() {
		x, y := at(c.x, c.y, w, h)
		fill(hdc, x-2, y-10, 5, 18, brass)
	}
	for _, s := range []struct{ name, note string }{
		{"Poti", "radar  95 km"},
		{"Supsa", "terminal"},
		{"Kutaisi", "final"},
		{"Zestafoni", "line"},
		{"Akhaltsikhe", "converters  80 km"},
		{"Akhalkalaki", "gauge"},
		{"Zekari", "pass"},
	} {
		p := site(s.name)
		x, y := at(p.x, p.y, w, h)
		ring(hdc, x, y, 8, paper, 2)
		disc(hdc, x, y, 3, paper)
		word := s.name
		if s.note != "" {
			word = s.name + "  " + s.note
		}
		placeName(hdc, x, y, w, s.name, word, font.mark)
	}
	ac, _, _ := liveSnapshot()
	for _, c := range ac {
		x := (c.lon - lon0) * mLon
		y := (c.lat - lat0) * mLat
		px, py := at(x, y, w, h)
		if px < -20 || px > w+20 || py < lipTop-20 || py > lipTop+h+20 {
			continue
		}
		hdg := math.Pi/2 - c.trackDeg*math.Pi/180
		col := civilC
		word := c.call + "  " + levelText(c.altM)
		if c.onFinalBox {
			col = warnC
			word = c.call + "  withheld"
		}
		ring(hdc, px, py, 14, col, 2)
		chevron(hdc, px, py, hdg, col)
		wordW := textWidth(hdc, font.mark, word)
		lx := px + 18
		if lx+wordW > w-20 {
			lx = px - wordW - 24
		}
		tag(hdc, lx, py-30, w, word, font.mark, col, col)
	}
}

func paintBoard(hdc uintptr, w, mapH int, font faces) {
	ac, _, _ := liveSnapshot()
	type line struct {
		left, right string
		col         uintptr
	}
	rows := make([]line, 0, len(ac)+1)
	limit := len(ac)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		c := ac[i]
		right := levelText(c.altM)
		col := civilC
		if c.onFinalBox {
			right = "withheld"
			col = warnC
		}
		rows = append(rows, line{c.call, right, col})
	}
	if len(ac) > limit {
		rows = append(rows, line{fmt.Sprintf("%d more", len(ac)-limit), "", dimC})
	}
	status := airLine()
	lineH := 22
	boxW := textWidth(hdc, font.mark, "Broadcasts")
	if sw := textWidth(hdc, font.body, status); sw > boxW {
		boxW = sw
	}
	for _, row := range rows {
		need := textWidth(hdc, font.mark, row.left) + 28 + textWidth(hdc, font.mark, row.right)
		if need > boxW {
			boxW = need
		}
	}
	boxW += 28
	if boxW > w/2 {
		boxW = w / 2
	}
	boxH := 16 + 22 + len(rows)*lineH + 22
	x := 14
	y := lipTop + mapH - boxH - 12
	if y < lipTop+8 {
		y = lipTop + 8
	}
	fill(hdc, x, y, boxW, boxH, plate)
	fill(hdc, x, y, 4, boxH, brass)
	textAt(hdc, x+14, y+8, "Broadcasts", font.mark, brass, plate, false)
	yy := y + 32
	for _, row := range rows {
		textAt(hdc, x+14, yy, row.left, font.mark, paper, plate, false)
		if row.right != "" {
			rw := textWidth(hdc, font.mark, row.right)
			textAt(hdc, x+boxW-14-rw, yy, row.right, font.mark, row.col, plate, false)
		}
		yy += lineH
	}
	textAt(hdc, x+14, y+boxH-24, status, font.body, paper, plate, false)
}

func ringWorld(hdc uintptr, cx, cy, radius float64, w, h int, col uintptr) {
	const n = 72
	var pts []point
	for i := 0; i <= n; i++ {
		a := float64(i) / n * 2 * math.Pi
		x, y := at(cx+radius*math.Cos(a), cy+radius*math.Sin(a), w, h)
		pts = append(pts, point{int32(x), int32(y)})
	}
	if len(pts) < 2 {
		return
	}
	pen, _, _ := createPen.Call(psSolid, 2, col)
	if pen == 0 {
		return
	}
	old, _, _ := selectObject.Call(hdc, pen)
	polyline.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	selectObject.Call(hdc, old)
	deleteObject.Call(pen)
}

func placeName(hdc uintptr, x, y, w int, name, word string, font uintptr) {
	tw := textWidth(hdc, font, word)
	tx := x - tw/2
	ty := y - 26
	if name == "Poti" {
		tx = x - tw - 16
		ty = y - 8
	}
	if tx < 8 {
		tx = 8
	}
	if tx+tw > w-8 {
		tx = w - 8 - tw
	}
	if ty < lipTop+4 {
		ty = y + 10
	}
	tag(hdc, tx, ty, w, word, font, paper, paper)
}

func tag(hdc uintptr, x, y, w int, s string, font, fg, accent uintptr) {
	if s == "" {
		return
	}
	tw := textWidth(hdc, font, s)
	th := 18
	if x < 8 {
		x = 8
	}
	if x+tw+16 > w-6 {
		x = w - 6 - tw - 16
	}
	if x < 8 {
		x = 8
	}
	fill(hdc, x-5, y-3, tw+16, th+6, plate)
	fill(hdc, x-5, y-3, 3, th+6, accent)
	textAt(hdc, x+4, y, s, font, fg, plate, false)
}

func paintLips(hdc uintptr, w, h, mapH int, font faces) {
	fill(hdc, 0, 0, w, lipTop, glass)
	fill(hdc, 0, lipTop+mapH, w, lipBot, glass)
	textAt(hdc, 16, 12, "Caliburn", font.title, paper, glass, true)
	tw := textWidth(hdc, font.title, "Caliburn")
	textAt(hdc, 16+tw+18, 18, "Live", font.body, dimC, glass, true)

	clk := georgiaNow().Format("15:04:05")
	sw := textWidth(hdc, font.body, clk)
	textAt(hdc, w-sw-16, 16, clk, font.body, paper, glass, true)
	textAt(hdc, 16, lipTop+mapH+12, airLine(), font.body, paper, glass, true)
}

func airLine() string {
	ac, at, err := liveSnapshot()
	if err != "" && len(ac) == 0 {
		return "No air picture."
	}
	if len(ac) == 0 && at.IsZero() {
		return "Asking for the air picture."
	}
	if !at.IsZero() && timeSince(at) > 30 {
		return "Air picture is stale."
	}
	held := 0
	for _, c := range ac {
		if c.onFinalBox {
			held++
		}
	}
	if held == 1 {
		return "One aircraft is on the Kutaisi final."
	}
	if held > 1 {
		return fmt.Sprintf("%d aircraft are on the Kutaisi final.", held)
	}
	if len(ac) == 1 {
		return "1 broadcasting. Nothing tasked."
	}
	return fmt.Sprintf("%d broadcasting. Nothing tasked.", len(ac))
}

func timeSince(t time.Time) float64 {
	return float64(time.Now().Sub(t)) / float64(time.Second)
}

func fill(hdc uintptr, x, y, w, h int, col uintptr) {
	if w <= 0 || h <= 0 {
		return
	}
	brush, _, _ := createSolidBrush.Call(col)
	if brush == 0 {
		return
	}
	r := rect{int32(x), int32(y), int32(x + w), int32(y + h)}
	fillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
	deleteObject.Call(brush)
}

func disc(hdc uintptr, x, y, r int, col uintptr) {
	brush, _, _ := createSolidBrush.Call(col)
	pen, _, _ := getStockObject.Call(nullPen)
	if brush == 0 {
		return
	}
	oldB, _, _ := selectObject.Call(hdc, brush)
	oldP, _, _ := selectObject.Call(hdc, pen)
	ellipse.Call(hdc, uintptr(x-r), uintptr(y-r), uintptr(x+r), uintptr(y+r))
	selectObject.Call(hdc, oldB)
	selectObject.Call(hdc, oldP)
	deleteObject.Call(brush)
}

func ring(hdc uintptr, x, y, r int, col uintptr, width int) {
	pen, _, _ := createPen.Call(psSolid, uintptr(width), col)
	brush, _, _ := getStockObject.Call(hollowBrush)
	if pen == 0 {
		return
	}
	oldP, _, _ := selectObject.Call(hdc, pen)
	oldB, _, _ := selectObject.Call(hdc, brush)
	ellipse.Call(hdc, uintptr(x-r), uintptr(y-r), uintptr(x+r), uintptr(y+r))
	selectObject.Call(hdc, oldP)
	selectObject.Call(hdc, oldB)
	deleteObject.Call(pen)
}

func chevron(hdc uintptr, x, y int, hdg float64, col uintptr) {
	local := [][2]float64{{13, 0}, {-9, 6}, {-4, 0}, {-9, -6}}
	c := math.Cos(hdg)
	s := math.Sin(hdg)
	pts := make([]point, len(local))
	for i, p := range local {
		wx := p[0]*c - p[1]*s
		wy := p[0]*s + p[1]*c
		pts[i] = point{int32(math.Round(float64(x) + wx)), int32(math.Round(float64(y) - wy))}
	}
	brush, _, _ := createSolidBrush.Call(col)
	pen, _, _ := getStockObject.Call(nullPen)
	if brush == 0 {
		return
	}
	oldB, _, _ := selectObject.Call(hdc, brush)
	oldP, _, _ := selectObject.Call(hdc, pen)
	polygon.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	selectObject.Call(hdc, oldB)
	selectObject.Call(hdc, oldP)
	deleteObject.Call(brush)
}

func segment(hdc uintptr, x0, y0, x1, y1 int, col uintptr, width int) {
	pen, _, _ := createPen.Call(psSolid, uintptr(width), col)
	if pen == 0 {
		return
	}
	old, _, _ := selectObject.Call(hdc, pen)
	moveToEx.Call(hdc, uintptr(x0), uintptr(y0), 0)
	lineTo.Call(hdc, uintptr(x1), uintptr(y1))
	selectObject.Call(hdc, old)
	deleteObject.Call(pen)
}

func dashed(hdc uintptr, x0, y0, x1, y1 int, col uintptr, width int) {
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	dist := math.Hypot(dx, dy)
	if dist < 2 {
		return
	}
	ux, uy := dx/dist, dy/dist
	const on = 6.0
	const off = 5.0
	for pos := 0.0; pos < dist; pos += on + off {
		b := pos + on
		if b > dist {
			b = dist
		}
		segment(hdc,
			int(float64(x0)+ux*pos), int(float64(y0)+uy*pos),
			int(float64(x0)+ux*b), int(float64(y0)+uy*b),
			col, width)
	}
}

func textAt(hdc uintptr, x, y int, s string, font, col, bg uintptr, solid bool) {
	if s == "" || font == 0 {
		return
	}
	u, err := utf16(s)
	if err != nil || len(u) < 2 {
		return
	}
	old, _, _ := selectObject.Call(hdc, font)
	if solid {
		setBkMode.Call(hdc, opaque)
	} else {
		setBkMode.Call(hdc, 1)
	}
	setBkColor.Call(hdc, bg)
	setTextColor.Call(hdc, col)
	textOutW.Call(hdc, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1))
	selectObject.Call(hdc, old)
}

func textWidth(hdc uintptr, font uintptr, s string) int {
	if s == "" || font == 0 {
		return 0
	}
	u, err := utf16(s)
	if err != nil || len(u) < 2 {
		return 0
	}
	old, _, _ := selectObject.Call(hdc, font)
	var sz size
	getTextExtent.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	selectObject.Call(hdc, old)
	return int(sz.cx)
}
