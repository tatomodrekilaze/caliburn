//go:build windows

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var scopeHWND atomic.Uintptr

// The map is drawn into a memory dc and copied across.
// A direct paint flickers the status line on each tick.

const (
	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmPaint         = 0x000F
	wmErase         = 0x0014
	wmKeyDown       = 0x0100
	wmTimer         = 0x0113
	wsOverlapped    = 0x00CF0000
	wsVisible       = 0x10000000
	cwUseDefault    = 0x80000000
	srcCopy         = 0x00CC0020
	psSolid         = 0
	psGeometric     = 0x00010000
	nullPen         = 8
	hollowBrush     = 5
	opaque          = 2
	defaultCharset  = 1
	antialiasedFont = 4
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	setDPIAware      = user32.NewProc("SetProcessDPIAware")
	registerClassExW = user32.NewProc("RegisterClassExW")
	createWindowExW  = user32.NewProc("CreateWindowExW")
	defWindowProcW   = user32.NewProc("DefWindowProcW")
	getMessageW      = user32.NewProc("GetMessageW")
	translateMessage = user32.NewProc("TranslateMessage")
	dispatchMessageW = user32.NewProc("DispatchMessageW")
	postQuitMessage  = user32.NewProc("PostQuitMessage")
	showWindow       = user32.NewProc("ShowWindow")
	updateWindow     = user32.NewProc("UpdateWindow")
	beginPaint       = user32.NewProc("BeginPaint")
	endPaint         = user32.NewProc("EndPaint")
	getClientRect    = user32.NewProc("GetClientRect")
	invalidateRect   = user32.NewProc("InvalidateRect")
	setTimer         = user32.NewProc("SetTimer")
	killTimer        = user32.NewProc("KillTimer")
	loadCursorW      = user32.NewProc("LoadCursorW")
	loadIconW        = user32.NewProc("LoadIconW")
	messageBoxW      = user32.NewProc("MessageBoxW")
	fillRect         = user32.NewProc("FillRect")
	getDC            = user32.NewProc("GetDC")
	releaseDC        = user32.NewProc("ReleaseDC")
	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	createCompatible = gdi32.NewProc("CreateCompatibleDC")
	createCompatBmp  = gdi32.NewProc("CreateCompatibleBitmap")
	createDIBSection = gdi32.NewProc("CreateDIBSection")
	selectObject     = gdi32.NewProc("SelectObject")
	bitBlt           = gdi32.NewProc("BitBlt")
	deleteObject     = gdi32.NewProc("DeleteObject")
	deleteDC         = gdi32.NewProc("DeleteDC")
	createPen        = gdi32.NewProc("CreatePen")
	extCreatePen     = gdi32.NewProc("ExtCreatePen")
	createSolidBrush = gdi32.NewProc("CreateSolidBrush")
	getStockObject   = gdi32.NewProc("GetStockObject")
	ellipse          = gdi32.NewProc("Ellipse")
	polygon          = gdi32.NewProc("Polygon")
	polyline         = gdi32.NewProc("Polyline")
	moveToEx         = gdi32.NewProc("MoveToEx")
	lineTo           = gdi32.NewProc("LineTo")
	textOutW         = gdi32.NewProc("TextOutW")
	setTextColor     = gdi32.NewProc("SetTextColor")
	setBkMode        = gdi32.NewProc("SetBkMode")
	setBkColor       = gdi32.NewProc("SetBkColor")
	getTextExtent    = gdi32.NewProc("GetTextExtentPoint32W")
	createFontW      = gdi32.NewProc("CreateFontW")
)

var face = mustUTF16("Bahnschrift")

var wndProcPtr = syscall.NewCallback(wndProc)

type point struct {
	x int32
	y int32
}

type rect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

type size struct {
	cx int32
	cy int32
}

type paintStruct struct {
	hdc         uintptr
	erase       int32
	rc          rect
	restore     int32
	incUpdate   int32
	rgbReserved [32]byte
}

type msg struct {
	hwnd    uintptr
	message uint32
	wparam  uintptr
	lparam  uintptr
	time    uint32
	pt      point
	private uint32
}

type wndClass struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menu       *uint16
	className  *uint16
	iconSm     uintptr
}

type logBrush struct {
	style uint32
	color uint32
	hatch uintptr
}

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

func mustUTF16(s string) []uint16 {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return []uint16{0}
	}
	return u
}

func utf16(s string) ([]uint16, error) {
	return syscall.UTF16FromString(s)
}

func makeFaces() faces {
	return faces{title: newFont(-22, 600), body: newFont(-16, 400), mark: newFont(-18, 600)}
}

func (f faces) drop() {
	if f.title != 0 {
		deleteObject.Call(f.title)
	}
	if f.body != 0 {
		deleteObject.Call(f.body)
	}
	if f.mark != 0 {
		deleteObject.Call(f.mark)
	}
}

func newFont(height, weight int) uintptr {
	h, _, _ := createFontW.Call(
		uintptr(int32(height)),
		0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		defaultCharset,
		0, 0,
		antialiasedFont,
		0,
		uintptr(unsafe.Pointer(&face[0])),
	)
	return h
}

func alert(s string) {
	t := mustUTF16(s)
	c := mustUTF16("Caliburn")
	messageBoxW.Call(0, uintptr(unsafe.Pointer(&t[0])), uintptr(unsafe.Pointer(&c[0])), 0x10)
}

func invalidate(hwnd uintptr) {
	invalidateRect.Call(hwnd, 0, 0)
}

func armTimer(hwnd uintptr, ms uint32) {
	setTimer.Call(hwnd, 1, uintptr(ms), 0)
}

func stopTimer(hwnd uintptr) {
	killTimer.Call(hwnd, 1)
}

func window() error {
	setDPIAware.Call()
	appFonts = makeFaces()
	defer appFonts.drop()

	inst, _, _ := getModuleHandleW.Call(0)
	cursor, _, _ := loadCursorW.Call(0, 32512)
	icon, _, _ := loadIconW.Call(0, 32512)
	class := mustUTF16("CaliburnCard")
	wc := wndClass{
		style:     0x0003,
		wndProc:   wndProcPtr,
		instance:  inst,
		icon:      icon,
		cursor:    cursor,
		className: &class[0],
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	atom, _, err := registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return err
	}
	title := mustUTF16("Caliburn")
	hwnd, _, err := createWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(&class[0])),
		uintptr(unsafe.Pointer(&title[0])),
		wsOverlapped|wsVisible,
		cwUseDefault, cwUseDefault,
		1180, 780,
		0, 0, inst, 0,
	)
	if hwnd == 0 {
		return err
	}
	scopeHWND.Store(hwnd)
	showWindow.Call(hwnd, 5)
	updateWindow.Call(hwnd)
	armTimer(hwnd, 1000)

	for {
		var m msg
		r, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

func wndProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch message {
	case wmErase:
		return 1
	case wmSize:
		invalidate(hwnd)
		return 0
	case wmTimer:
		onTimer(hwnd)
		return 0
	case wmKeyDown:
		onKey(hwnd, wparam, lparam)
		return 0
	case wmPaint:
		onPaint(hwnd)
		return 0
	case wmDestroy:
		scopeHWND.Store(0)
		stopTimer(hwnd)
		postQuitMessage.Call(0)
		return 0
	}
	r, _, _ := defWindowProcW.Call(hwnd, message, wparam, lparam)
	return r
}

func onPaint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	var rc rect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := int(rc.right - rc.left)
	h := int(rc.bottom - rc.top)
	if w > 0 && h > 0 {
		mem, _, _ := createCompatible.Call(hdc)
		bmp, _, _ := createCompatBmp.Call(hdc, uintptr(w), uintptr(h))
		if mem != 0 && bmp != 0 {
			old, _, _ := selectObject.Call(mem, bmp)
			draw(mem, w, h, appFonts)
			bitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, srcCopy)
			selectObject.Call(mem, old)
		}
		if bmp != 0 {
			deleteObject.Call(bmp)
		}
		if mem != 0 {
			deleteDC.Call(mem)
		}
	}
	endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func dumpPNG(path string, w, h int) error {
	setDPIAware.Call()
	font := makeFaces()
	defer font.drop()

	screen, _, _ := getDC.Call(0)
	if screen == 0 {
		return syscall.EINVAL
	}
	defer releaseDC.Call(0, screen)

	bi := bitmapInfoHeader{
		size:     40,
		width:    int32(w),
		height:   -int32(h),
		planes:   1,
		bitCount: 32,
	}
	var bits *byte
	dib, _, err := createDIBSection.Call(
		screen,
		uintptr(unsafe.Pointer(&bi)),
		0,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if dib == 0 || bits == nil {
		return err
	}
	defer deleteObject.Call(dib)
	mem, _, _ := createCompatible.Call(screen)
	if mem == 0 {
		return syscall.EINVAL
	}
	defer deleteDC.Call(mem)
	old, _, _ := selectObject.Call(mem, dib)
	draw(mem, w, h, font)
	selectObject.Call(mem, old)

	stride := w * 4
	raw := unsafe.Slice(bits, stride*h)
	return writePNG(path, raw, w, h, stride)
}

func writePNG(path string, bgra []byte, w, h, stride int) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*stride + x*4
			img.SetRGBA(x, y, color.RGBA{R: bgra[i+2], G: bgra[i+1], B: bgra[i], A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
