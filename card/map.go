package main

import (
	"fmt"
	"image"
	xdraw "image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Satellite tiles for the same metre frame the sites use.
// Zoom 10 covers the spine in a few dozen tiles.

const mapZoom = 10

var (
	tileClient = &http.Client{Timeout: 20 * time.Second}
	tilesReady atomic.Bool
	viewW      atomic.Int32
	viewH      atomic.Int32

	mapMu  sync.Mutex
	mapW   int
	mapH   int
	mapPix []byte
	mapGen uint64
)

func frameBox() (south, west, north, east float64) {
	south = lat0 + worldMinY/mLat
	north = lat0 + worldMaxY/mLat
	west = lon0 + worldMinX/mLon
	east = lon0 + worldMaxX/mLon
	return
}

func metresToLatLon(x, y float64) (float64, float64) {
	return lat0 + y/mLat, lon0 + x/mLon
}

func tileXY(lat, lon float64) (int, int, int, int) {
	n := math.Exp2(float64(mapZoom))
	fx := (lon + 180) / 360 * n
	s := math.Sin(lat * math.Pi / 180)
	fy := (1 - math.Log((1+s)/(1-s))/(2*math.Pi)) / 2 * n
	tx := int(math.Floor(fx))
	ty := int(math.Floor(fy))
	px := int((fx - float64(tx)) * 256)
	py := int((fy - float64(ty)) * 256)
	if px < 0 {
		px = 0
	}
	if py < 0 {
		py = 0
	}
	if px > 255 {
		px = 255
	}
	if py > 255 {
		py = 255
	}
	return tx, ty, px, py
}

func cacheDir() string {
	dir := "tiles"
	if exe, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(exe), "tiles")
		if st, err := os.Stat(beside); err == nil && st.IsDir() {
			return beside
		}
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return dir
	}
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func tilePath(x, y int) string {
	return filepath.Join(cacheDir(), fmt.Sprintf("%d", mapZoom), fmt.Sprintf("%d", x), fmt.Sprintf("%d.jpg", y))
}

func tileList() [][2]int {
	south, west, north, east := frameBox()
	x0, yNorth, _, _ := tileXY(north, west)
	x1, ySouth, _, _ := tileXY(south, east)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if ySouth < yNorth {
		yNorth, ySouth = ySouth, yNorth
	}
	var out [][2]int
	for x := x0; x <= x1; x++ {
		for y := yNorth; y <= ySouth; y++ {
			out = append(out, [2]int{x, y})
		}
	}
	return out
}

func prepareMap() {
	need := tileList()
	var missing [][2]int
	for _, t := range need {
		if _, err := os.Stat(tilePath(t[0], t[1])); err != nil {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		if err := download(missing); err != nil {
			fmt.Fprintf(os.Stderr, "map: %v\n", err)
		}
	}
	got := 0
	for _, t := range need {
		if _, err := os.Stat(tilePath(t[0], t[1])); err == nil {
			got++
		}
	}
	if got > 0 {
		tilesReady.Store(true)
	}
}

func download(need [][2]int) error {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for _, t := range need {
		wg.Add(1)
		go func(x, y int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := fetchTile(x, y); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}(t[0], t[1])
	}
	wg.Wait()
	return first
}

func fetchTile(x, y int) error {
	urls := []string{
		fmt.Sprintf("https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/%d/%d/%d", mapZoom, y, x),
		fmt.Sprintf("https://tile.openstreetmap.org/%d/%d/%d.png", mapZoom, x, y),
	}
	var last error
	for _, url := range urls {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			last = err
			continue
		}
		req.Header.Set("User-Agent", "Caliburn/1.0 (local scope)")
		res, err := tileClient.Do(req)
		if err != nil {
			last = err
			continue
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			last = fmt.Errorf("tile %d %d: %s", x, y, res.Status)
			continue
		}
		img, _, err := image.Decode(res.Body)
		res.Body.Close()
		if err != nil {
			last = err
			continue
		}
		path := tilePath(x, y)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		err = encodeJPEG(f, img)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	return last
}

func encodeJPEG(w io.Writer, img image.Image) error {
	return jpeg.Encode(w, img, &jpeg.Options{Quality: 85})
}

func setMap(w, h int, pix []byte) {
	mapMu.Lock()
	mapW, mapH = w, h
	mapPix = pix
	mapGen++
	mapMu.Unlock()
}

func copyMap() (w, h int, pix []byte, gen uint64) {
	mapMu.Lock()
	defer mapMu.Unlock()
	return mapW, mapH, mapPix, mapGen
}

func loadFlat() map[[2]int]*image.RGBA {
	out := map[[2]int]*image.RGBA{}
	for _, t := range tileList() {
		f, err := os.Open(tilePath(t[0], t[1]))
		if err != nil {
			continue
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		b := img.Bounds()
		rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		xdraw.Draw(rgba, rgba.Bounds(), img, b.Min, xdraw.Src)
		out[t] = rgba
	}
	return out
}

// renderMap paints the satellite into a top-down BGRA buffer.
// It runs off the window thread. The paint only copies the bytes.
func renderMap(w, h int) []byte {
	tiles := loadFlat()
	n := math.Exp2(float64(mapZoom))
	xs := make([]float64, w)
	for x := 0; x < w; x++ {
		wx := worldMinX + (float64(x)+0.5)/float64(w)*(worldMaxX-worldMinX)
		lon := lon0 + wx/mLon
		xs[x] = (lon + 180) / 360 * n
	}
	buf := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		wy := worldMinY + (1-(float64(y)+0.5)/float64(h))*(worldMaxY-worldMinY)
		wx0 := worldMinX + 0.5/float64(w)*(worldMaxX-worldMinX)
		lat := lat0 + wy/mLat
		s := math.Sin(lat * math.Pi / 180)
		fy := (1 - math.Log((1+s)/(1-s))/(2*math.Pi)) / 2 * n
		ty := int(math.Floor(fy))
		py := int((fy - float64(ty)) * 256)
		if py < 0 {
			py = 0
		}
		if py > 255 {
			py = 255
		}
		row := y * w * 4
		for x := 0; x < w; x++ {
			tx := int(math.Floor(xs[x]))
			px := int((xs[x] - float64(tx)) * 256)
			if px < 0 {
				px = 0
			}
			if px > 255 {
				px = 255
			}
			r, g, b := byte(14), byte(22), byte(20)
			if img := tiles[[2]int{tx, ty}]; img != nil && px < img.Rect.Dx() && py < img.Rect.Dy() {
				i := img.PixOffset(px, py)
				r, g, b = img.Pix[i], img.Pix[i+1], img.Pix[i+2]
			} else {
				wx := wx0 + float64(x)/float64(w)*(worldMaxX-worldMinX)
				r, g, b = shade(wx, wy)
			}
			i := row + x*4
			buf[i] = b
			buf[i+1] = g
			buf[i+2] = r
			buf[i+3] = 255
		}
	}
	return buf
}
