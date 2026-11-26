package main

import (
	"fmt"
	"strings"
)

const (
	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008
)

// parseHotkey entende textos como "Ctrl+Shift+L" e devolve (modificadores, tecla virtual).
func parseHotkey(s string) (mods, vk uintptr, ok bool) {
	for _, p := range strings.Split(strings.ReplaceAll(s, " ", ""), "+") {
		up := strings.ToUpper(p)
		switch {
		case up == "CTRL" || up == "CONTROL":
			mods |= modControl
		case up == "SHIFT":
			mods |= modShift
		case up == "ALT":
			mods |= modAlt
		case up == "WIN" || up == "WINDOWS":
			mods |= modWin
		case len(up) == 1 && ((up[0] >= 'A' && up[0] <= 'Z') || (up[0] >= '0' && up[0] <= '9')):
			vk = uintptr(up[0])
		case len(up) >= 2 && up[0] == 'F':
			var n int
			if _, err := fmt.Sscanf(up[1:], "%d", &n); err == nil && n >= 1 && n <= 24 {
				vk = uintptr(0x70 + n - 1)
			}
		case up == "SPACE":
			vk = 0x20
		case up == "":
		default:
			return 0, 0, false
		}
	}
	return mods, vk, vk != 0 && mods != 0
}
