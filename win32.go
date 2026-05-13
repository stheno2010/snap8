//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// ---- Win32 bindings (no external deps; everything goes through LazyDLL) ----

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	dwmapi = syscall.NewLazyDLL("dwmapi.dll")

	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetTopWindow                  = user32.NewProc("GetTopWindow")
	pGetWindow                     = user32.NewProc("GetWindow")
	pIsWindowVisible               = user32.NewProc("IsWindowVisible")
	pIsIconic                      = user32.NewProc("IsIconic")
	pIsZoomed                      = user32.NewProc("IsZoomed")
	pGetAncestor                   = user32.NewProc("GetAncestor")
	pGetWindowTextLengthW          = user32.NewProc("GetWindowTextLengthW")
	pGetWindowTextW                = user32.NewProc("GetWindowTextW")
	pGetClassNameW                 = user32.NewProc("GetClassNameW")
	pGetWindowLongPtrW             = user32.NewProc("GetWindowLongPtrW")
	pGetWindowRect                 = user32.NewProc("GetWindowRect")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pSetWindowPos                  = user32.NewProc("SetWindowPos")
	pMonitorFromPoint              = user32.NewProc("MonitorFromPoint")
	pMonitorFromWindow             = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	pGetCursorPos                  = user32.NewProc("GetCursorPos")
	pGetForegroundWindow           = user32.NewProc("GetForegroundWindow")

	pDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
)

const (
	gwHwndNext = 2

	gwlExStyle     = ^uintptr(19) // -20 as uintptr
	wsExToolWindow = 0x00000080
	wsExAppWindow  = 0x00040000

	gaRootOwner = 3

	dwmwaCloaked             = 14
	dwmwaExtendedFrameBounds = 9

	monitorDefaultToNearest = 2
	monitorDefaultToPrimary = 1

	swRestore = 9

	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 == (HANDLE)-4
	dpiPerMonitorAwareV2 = ^uintptr(3)
)

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }

type point struct{ X, Y int32 }

type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

func boolCall(p *syscall.LazyProc, args ...uintptr) bool {
	r, _, _ := p.Call(args...)
	return r != 0
}

func isWindowVisible(h uintptr) bool { return boolCall(pIsWindowVisible, h) }
func isIconic(h uintptr) bool        { return boolCall(pIsIconic, h) }
func isZoomed(h uintptr) bool        { return boolCall(pIsZoomed, h) }
func getForegroundWindow() uintptr   { r, _, _ := pGetForegroundWindow.Call(); return r }

func getTopWindow() uintptr        { r, _, _ := pGetTopWindow.Call(0); return r }
func getNextWindow(h uintptr) uintptr { r, _, _ := pGetWindow.Call(h, gwHwndNext); return r }

func getAncestor(h uintptr, flags uintptr) uintptr {
	r, _, _ := pGetAncestor.Call(h, flags)
	return r
}

func getWindowExStyle(h uintptr) uint64 {
	r, _, _ := pGetWindowLongPtrW.Call(h, gwlExStyle)
	return uint64(r)
}

func getWindowTextLength(h uintptr) int {
	r, _, _ := pGetWindowTextLengthW.Call(h)
	return int(int32(r))
}

func getWindowText(h uintptr) string {
	n := getWindowTextLength(h)
	if n <= 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func getClassName(h uintptr) string {
	var buf [256]uint16
	r, _, _ := pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:r])
}

func dwmGetInt(h uintptr, attr uintptr) (val int32, ok bool) {
	var v int32
	hr, _, _ := pDwmGetWindowAttribute.Call(h, attr, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return v, hr == 0
}

func dwmGetRect(h uintptr, attr uintptr) (r rect, ok bool) {
	var v rect
	hr, _, _ := pDwmGetWindowAttribute.Call(h, attr, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return v, hr == 0
}

func getWindowRect(h uintptr) (rect, bool) {
	var r rect
	ok := boolCall(pGetWindowRect, h, uintptr(unsafe.Pointer(&r)))
	return r, ok
}

func showWindowRestore(h uintptr) { pShowWindow.Call(h, swRestore) }

func setWindowPos(h uintptr, x, y, cx, cy int32, flags uintptr) bool {
	r, _, _ := pSetWindowPos.Call(h, 0, uintptr(x), uintptr(y), uintptr(cx), uintptr(cy), flags)
	return r != 0
}

// POINT is 8 bytes, passed by value in a single register on amd64.
func packPoint(x, y int32) uintptr {
	return uintptr(uint32(x)) | uintptr(uint32(y))<<32
}

func monitorFromPoint(x, y int32, flags uintptr) uintptr {
	r, _, _ := pMonitorFromPoint.Call(packPoint(x, y), flags)
	return r
}

func monitorFromWindow(h uintptr, flags uintptr) uintptr {
	r, _, _ := pMonitorFromWindow.Call(h, flags)
	return r
}

func getCursorPos() (point, bool) {
	var p point
	ok := boolCall(pGetCursorPos, uintptr(unsafe.Pointer(&p)))
	return p, ok
}

func getMonitorInfo(hMon uintptr) (monitorInfo, bool) {
	var mi monitorInfo
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	r, _, _ := pGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi)))
	return mi, r != 0
}

func setPerMonitorV2DPI() {
	if pSetProcessDpiAwarenessContext.Find() == nil {
		pSetProcessDpiAwarenessContext.Call(dpiPerMonitorAwareV2)
	}
}
