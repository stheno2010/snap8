//go:build windows

package main

import "strings"

// Well-known shell / system window classes that look top-level but should never be moved.
var shellClasses = map[string]bool{
	"Progman":                      true,
	"WorkerW":                      true,
	"Shell_TrayWnd":                true,
	"Shell_SecondaryTrayWnd":       true,
	"NotifyIconOverflowWindow":     true,
	"TopLevelWindowForOverflowXamlIsland":              true,
	"Windows.UI.Core.CoreWindow":                       true,
	"XamlExplorerHostIslandWindow":                     true,
	"ForegroundStaging":                                true,
	"MultitaskingViewFrame":                            true,
	"Windows.UI.Composition.DesktopWindowContentBridge": true,
}

// isAltTabWindow reports whether h is a normal user window — roughly the set Alt+Tab shows.
func isAltTabWindow(h uintptr, restoreMinimized bool) bool {
	if !isWindowVisible(h) {
		return false
	}
	if getAncestor(h, gaRootOwner) != h { // top-level & unowned only
		return false
	}
	if isIconic(h) && !restoreMinimized {
		return false
	}
	if getWindowTextLength(h) == 0 {
		return false
	}
	ex := getWindowExStyle(h)
	tool := ex&wsExToolWindow != 0
	app := ex&wsExAppWindow != 0
	if tool && !app {
		return false
	}
	if cloaked, ok := dwmGetInt(h, dwmwaCloaked); ok && cloaked != 0 {
		return false // other virtual desktop, suspended UWP, hidden frame, ...
	}
	if shellClasses[getClassName(h)] {
		return false
	}
	return true
}

// enumAltTabWindows returns the matching top-level windows in Z order (front-most first).
func enumAltTabWindows(restoreMinimized bool) []uintptr {
	var out []uintptr
	for h := getTopWindow(); h != 0; h = getNextWindow(h) {
		if isAltTabWindow(h, restoreMinimized) {
			out = append(out, h)
		}
	}
	return out
}

func resolveMonitor(target string) uintptr {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "primary":
		return monitorFromPoint(0, 0, monitorDefaultToPrimary)
	case "foreground":
		if fg := getForegroundWindow(); fg != 0 {
			return monitorFromWindow(fg, monitorDefaultToNearest)
		}
		fallthrough
	default: // "cursor"
		if p, ok := getCursorPos(); ok {
			return monitorFromPoint(p.X, p.Y, monitorDefaultToNearest)
		}
		return monitorFromPoint(0, 0, monitorDefaultToPrimary)
	}
}

// divRound returns round(a/b) for b > 0, using integer math (round half up).
func divRound(a, b int32) int32 {
	return int32((int64(a)*2 + int64(b)) / (int64(b) * 2))
}

// cellRect computes the [r,c] cell of a rows x cols grid laid over `area`.
// `gap` is applied around the edges and between cells; rounding is cumulative so
// the cells tile the area exactly.
func cellRect(area rect, rows, cols, r, c, gap, innerW, innerH int32) rect {
	xL := area.Left + (c+1)*gap + divRound(c*innerW, cols)
	xR := area.Left + (c+1)*gap + divRound((c+1)*innerW, cols)
	yT := area.Top + (r+1)*gap + divRound(r*innerH, rows)
	yB := area.Top + (r+1)*gap + divRound((r+1)*innerH, rows)
	return rect{Left: xL, Top: yT, Right: xR, Bottom: yB}
}

func moveWindowToCell(h uintptr, cell rect) bool {
	if isZoomed(h) || isIconic(h) {
		showWindowRestore(h) // un-maximize / un-minimize so the new bounds stick
	}
	// The visible edge sits inside the HWND rect by an invisible resize border.
	// Compensate using the DWM "extended frame bounds" so the visible edge hits the cell.
	var dl, dt, dr, db int32
	if wr, ok := getWindowRect(h); ok {
		if fb, ok2 := dwmGetRect(h, dwmwaExtendedFrameBounds); ok2 {
			dl = fb.Left - wr.Left
			dt = fb.Top - wr.Top
			dr = wr.Right - fb.Right
			db = wr.Bottom - fb.Bottom
		}
	}
	x := cell.Left - dl
	y := cell.Top - dt
	w := cell.width() + dl + dr
	hgt := cell.height() + dt + db
	if w < 1 {
		w = 1
	}
	if hgt < 1 {
		hgt = 1
	}
	return setWindowPos(h, x, y, w, hgt, swpNoZOrder|swpNoActivate|swpFrameChanged)
}

// arrange lays the front-most windows onto the Rows x Cols grid of the target monitor.
// Returns the number of windows that were moved.
func arrange(c config) int {
	rows := c.rows()
	cols := c.cols()
	cells := rows * cols

	wins := enumAltTabWindows(c.restoreMinimized())
	if len(wins) > cells {
		wins = wins[:cells]
	}
	if len(wins) == 0 {
		return 0
	}

	mi, ok := getMonitorInfo(resolveMonitor(c.TargetMonitor))
	if !ok {
		return 0
	}
	area := mi.rcMonitor
	if c.useWorkArea() {
		area = mi.rcWork
	}

	gap := int32(c.clampGap())
	innerW := area.width() - (int32(cols)+1)*gap
	if innerW < 0 {
		innerW = 0
	}
	innerH := area.height() - (int32(rows)+1)*gap
	if innerH < 0 {
		innerH = 0
	}

	placed := 0
	for i, h := range wins {
		r := int32(i / cols) // RowMajor: front window -> top-left, then left to right
		col := int32(i % cols)
		cell := cellRect(area, int32(rows), int32(cols), r, col, gap, innerW, innerH)
		if moveWindowToCell(h, cell) {
			placed++
		}
	}
	return placed
}
