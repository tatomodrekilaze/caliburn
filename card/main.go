//go:build windows

package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// The window is the live scope. The ground is the satellite picture.
// Space asks for a fresh air picture.

var appFonts faces

func main() {
	out, _, _, dump, err := parseDump()
	if err != nil {
		fail(err)
	}
	if dump {
		prepareMap()
		fetchLive()
		mh := 760 - lipTop - lipBot
		setMap(1280, mh, renderMap(1280, mh))
		if err := dumpPNG(out, 1280, 760); err != nil {
			fail(err)
		}
		return
	}
	go maintain()
	if err := window(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "caliburn: %v\n", err)
	if !wantsDump() {
		alert(err.Error())
	}
	os.Exit(1)
}

func wantsDump() bool {
	for _, a := range os.Args[1:] {
		if a == "-dump" {
			return true
		}
	}
	return false
}

func parseDump() (string, int, float64, bool, error) {
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] != "-dump" {
			continue
		}
		out := "frame.png"
		t := 100.0
		which := 0
		rest := args[i+1:]
		if len(rest) > 0 {
			out = rest[0]
		}
		if len(rest) > 1 {
			v, err := strconv.ParseFloat(rest[1], 64)
			if err != nil {
				return "", 0, 0, true, fmt.Errorf("time %q", rest[1])
			}
			t = v
		}
		if len(rest) > 2 {
			n, err := caseIndex(rest[2])
			if err != nil {
				return "", 0, 0, true, err
			}
			which = n
		}
		return out, which, t, true, nil
	}
	return "", 0, 0, false, nil
}

func caseIndex(name string) (int, error) {
	switch name {
	case "nominal", "0":
		return 0, nil
	case "one-key", "1":
		return 1, nil
	case "in-final", "2":
		return 2, nil
	case "lost", "3":
		return 3, nil
	default:
		return 0, fmt.Errorf("no engagement %q", name)
	}
}

func maintain() {
	prepareMap()
	var last time.Time
	var builtW, builtH int
	var builtReady bool
	seen := false
	for {
		hwnd := scopeHWND.Load()
		if hwnd != 0 {
			seen = true
		} else if seen {
			return
		}
		if last.IsZero() || time.Since(last) >= 8*time.Second {
			fetchLive()
			last = time.Now()
			if hwnd != 0 {
				invalidate(hwnd)
			}
		}
		w := int(viewW.Load())
		h := int(viewH.Load())
		ready := tilesReady.Load()
		if w >= 8 && h >= 8 && (w != builtW || h != builtH || (ready && !builtReady)) {
			setMap(w, h, renderMap(w, h))
			builtW, builtH = w, h
			builtReady = ready
			if hwnd != 0 {
				invalidate(hwnd)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func onTimer(hwnd uintptr) {
	invalidate(hwnd)
}

func onKey(hwnd, wparam, lparam uintptr) {
	if wparam != 0x20 {
		return
	}
	if lparam&(1<<30) != 0 {
		return
	}
	go func() {
		fetchLive()
		invalidate(hwnd)
	}()
}
