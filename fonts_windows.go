//go:build windows

package main

import (
	_ "embed"
	"unsafe"
)

//go:embed orbitron.ttf
var fontOrbitron []byte

//go:embed orbitron-bold.ttf
var fontOrbitronBold []byte

//go:embed rajdhani.ttf
var fontRajdhani []byte

var (
	pAddFontMemResourceEx  = gdi32.NewProc("AddFontMemResourceEx")
	pSetTextCharacterExtra = gdi32.NewProc("SetTextCharacterExtra")
	hudFontsOK             bool
)

// loadHudFonts deixa as fontes futuristas disponíveis só para o Jarvis (sem instalar nada).
func loadHudFonts() {
	ok := true
	for _, f := range [][]byte{fontOrbitron, fontOrbitronBold, fontRajdhani} {
		var n uint32
		r, _, _ := pAddFontMemResourceEx.Call(uintptr(unsafe.Pointer(&f[0])), uintptr(len(f)), 0, uintptr(unsafe.Pointer(&n)))
		if r == 0 {
			ok = false
		}
	}
	hudFontsOK = ok
}

// nomes das fontes (com alternativa se não carregarem)
func faceHud() string {
	if hudFontsOK {
		return "Orbitron Medium"
	}
	return "Segoe UI Light"
}
func faceHudBold() string {
	if hudFontsOK {
		return "Orbitron"
	}
	return "Segoe UI"
}
func faceTech() string {
	if hudFontsOK {
		return "Rajdhani SemiBold"
	}
	return "Segoe UI Semibold"
}
