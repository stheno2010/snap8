//go:build windows

// Command snap8 arranges the front-most windows onto an evenly-divided
// Rows x Cols grid (default 2x4 = 8 cells) of the target monitor, then exits.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type config struct {
	Rows             int    `json:"Rows"`
	Cols             int    `json:"Cols"`
	Gap              int    `json:"Gap"`
	UseWorkArea      *bool  `json:"UseWorkArea"`
	TargetMonitor    string `json:"TargetMonitor"`
	RestoreMinimized *bool  `json:"RestoreMinimized"`
}

func defaultConfig() config {
	return config{Rows: 2, Cols: 4, Gap: 0, TargetMonitor: "Cursor"}
}

func clampInt(v, lo, hi, def int) int {
	if v >= lo && v <= hi {
		return v
	}
	return def
}

func (c config) rows() int     { return clampInt(c.Rows, 1, 20, 2) }
func (c config) cols() int     { return clampInt(c.Cols, 1, 20, 4) }
func (c config) clampGap() int { return clampInt(c.Gap, 0, 200, 0) }
func (c config) useWorkArea() bool { return c.UseWorkArea == nil || *c.UseWorkArea }
func (c config) restoreMinimized() bool {
	return c.RestoreMinimized == nil || *c.RestoreMinimized
}

func configPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "snap8.json")
	}
	return "snap8.json"
}

// loadConfig reads snap8.json next to the exe. Missing or broken file -> defaults.
func loadConfig() config {
	def := defaultConfig()
	data, err := os.ReadFile(configPath())
	if err != nil {
		return def
	}
	c := def
	if json.Unmarshal(data, &c) != nil {
		return def
	}
	if c.TargetMonitor == "" {
		c.TargetMonitor = "Cursor"
	}
	return c
}

func main() {
	setPerMonitorV2DPI() // so monitor/window coordinates are real physical pixels

	if arrange(loadConfig()) == 0 {
		os.Exit(1) // exit code 1: no windows were arranged
	}
}
