//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	pGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	pSetCursorPos      = user32.NewProc("SetCursorPos")
	pSendInput         = user32.NewProc("SendInput")
	pGetAsyncKeyState  = user32.NewProc("GetAsyncKeyState")
	pVkKeyScanW        = user32.NewProc("VkKeyScanW")
	pStretchBlt        = gdi32.NewProc("StretchBlt")
	pSetStretchBltMode = gdi32.NewProc("SetStretchBltMode")
	pSetBrushOrgEx     = gdi32.NewProc("SetBrushOrgEx")
	pGetDIBits         = gdi32.NewProc("GetDIBits")
)

const (
	INPUT_MOUSE    = 0
	INPUT_KEYBOARD = 1

	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	MOUSEEVENTF_WHEEL      = 0x0800
	MOUSEEVENTF_HWHEEL     = 0x1000

	KEYEVENTF_EXTENDEDKEY = 0x0001
	KEYEVENTF_KEYUP       = 0x0002
	KEYEVENTF_UNICODE     = 0x0004
)

// INPUT do Windows (40 bytes em 64 bits)
type mouseInput struct {
	Type      uint32
	_         uint32
	Dx, Dy    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	_         uint32
	Extra     uintptr
}

type keyInput struct {
	Type  uint32
	_     uint32
	Vk    uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	_     uint32
	Extra uintptr
	_     [8]byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type winScreen struct {
	mu     sync.Mutex
	scaleX float64 // pixels reais por pixel do print
	scaleY float64
}

func init() { screen = &winScreen{scaleX: 1, scaleY: 1} }

func screenSize() (int, int) {
	w, _, _ := pGetSystemMetrics.Call(0) // SM_CXSCREEN
	h, _, _ := pGetSystemMetrics.Call(1) // SM_CYSCREEN
	return int(w), int(h)
}

func fit(w, h, maxW, maxH int) (int, int) {
	s := math.Min(1, math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h)))
	return max(1, int(float64(w)*s)), max(1, int(float64(h)*s))
}

// grab copia a região (sx,sy,sw,sh) da tela, reduzida para (tw,th), e devolve JPEG.
func grab(sx, sy, sw, sh, tw, th int) ([]byte, error) {
	screenDC, _, _ := pGetDC.Call(0)
	if screenDC == 0 {
		return nil, errors.New("no screen")
	}
	defer pReleaseDC.Call(0, screenDC)
	mem, _, _ := pCreateCompatibleDC.Call(screenDC)
	defer pDeleteDC.Call(mem)
	bmp, _, _ := pCreateCompatibleBitmap.Call(screenDC, uintptr(tw), uintptr(th))
	if bmp == 0 {
		return nil, errors.New("couldn't create bitmap")
	}
	defer pDeleteObject.Call(bmp)
	old, _, _ := pSelectObject.Call(mem, bmp)
	pSetStretchBltMode.Call(mem, 4) // HALFTONE
	pSetBrushOrgEx.Call(mem, 0, 0, 0)
	r, _, _ := pStretchBlt.Call(mem, 0, 0, uintptr(tw), uintptr(th), screenDC, uintptr(sx), uintptr(sy), uintptr(sw), uintptr(sh), SRCCOPY|0x40000000)
	pSelectObject.Call(mem, old)
	if r == 0 {
		return nil, errors.New("screen capture failed (is the screen locked?)")
	}
	hdr := bitmapInfoHeader{Width: int32(tw), Height: -int32(th), Planes: 1, BitCount: 32}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	buf := make([]byte, tw*th*4)
	// BITMAPINFO = header + 1 RGBQUAD; reservamos espaço extra
	bi := struct {
		H   bitmapInfoHeader
		Pad [4]uint32
	}{H: hdr}
	n, _, _ := pGetDIBits.Call(mem, bmp, 0, uintptr(th), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	if n == 0 {
		return nil, errors.New("couldn't read the screen")
	}
	img := image.NewRGBA(image.Rect(0, 0, tw, th))
	for i := 0; i < len(buf); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = buf[i+2], buf[i+1], buf[i], 255
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (s *winScreen) Capture(maxW, maxH int) ([]byte, int, int, error) {
	w, h := screenSize()
	tw, th := fit(w, h, maxW, maxH)
	b, err := grab(0, 0, w, h, tw, th)
	if err != nil {
		return nil, 0, 0, err
	}
	s.mu.Lock()
	s.scaleX, s.scaleY = float64(w)/float64(tw), float64(h)/float64(th)
	s.mu.Unlock()
	return b, tw, th, nil
}

// Zoom: região em coordenadas do último print; não muda o mapeamento.
func (s *winScreen) Zoom(reg []float64) ([]byte, error) {
	s.mu.Lock()
	sx, sy := s.scaleX, s.scaleY
	s.mu.Unlock()
	x0, y0 := int(reg[0]*sx), int(reg[1]*sy)
	x1, y1 := int(reg[2]*sx), int(reg[3]*sy)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	w, h := screenSize()
	x0, y0 = max(0, x0), max(0, y0)
	x1, y1 = min(w, x1), min(h, y1)
	if x1-x0 < 4 || y1-y0 < 4 {
		return nil, errors.New("zoom region too small")
	}
	// amplia até caber em 1366x900
	rw, rh := x1-x0, y1-y0
	sc := math.Min(1366/float64(rw), 900/float64(rh))
	tw, th := max(1, int(float64(rw)*sc)), max(1, int(float64(rh)*sc))
	return grab(x0, y0, rw, rh, tw, th)
}

func (s *winScreen) real(v any) (int32, int32, error) {
	arr, ok := v.([]any)
	if !ok || len(arr) != 2 {
		return 0, 0, errors.New("missing coordinate")
	}
	x, ok1 := arr[0].(float64)
	y, ok2 := arr[1].(float64)
	if !ok1 || !ok2 {
		return 0, 0, errors.New("bad coordinate")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return int32(math.Round(x * s.scaleX)), int32(math.Round(y * s.scaleY)), nil
}

func (s *winScreen) EscPressed() bool {
	r, _, _ := pGetAsyncKeyState.Call(VK_ESCAPE)
	return r&0x8001 != 0
}

func sendMouse(flags uint32, data int32) {
	in := mouseInput{Type: INPUT_MOUSE, Flags: flags, MouseData: uint32(data)}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func sendKey(vk uint16, up bool) {
	in := keyInput{Type: INPUT_KEYBOARD, Vk: vk}
	if up {
		in.Flags |= KEYEVENTF_KEYUP
	}
	if extendedKeys[vk] {
		in.Flags |= KEYEVENTF_EXTENDEDKEY
	}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func sendUnicode(r rune) {
	for _, u := range utf16.Encode([]rune{r}) {
		ins := [2]keyInput{
			{Type: INPUT_KEYBOARD, Scan: u, Flags: KEYEVENTF_UNICODE},
			{Type: INPUT_KEYBOARD, Scan: u, Flags: KEYEVENTF_UNICODE | KEYEVENTF_KEYUP},
		}
		pSendInput.Call(2, uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
	}
}

var extendedKeys = map[uint16]bool{0x21: true, 0x22: true, 0x23: true, 0x24: true, 0x25: true, 0x26: true, 0x27: true, 0x28: true,
	0x2D: true, 0x2E: true, 0x5B: true, 0x5C: true, 0x6F: true, 0x90: true}

var keyNames = map[string]uint16{
	"return": 0x0D, "enter": 0x0D, "kp_enter": 0x0D, "tab": 0x09, "escape": 0x1B, "esc": 0x1B, "backspace": 0x08, "back_space": 0x08,
	"delete": 0x2E, "del": 0x2E, "insert": 0x2D, "space": 0x20, "up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
	"home": 0x24, "end": 0x23, "page_up": 0x21, "pageup": 0x21, "prior": 0x21, "page_down": 0x22, "pagedown": 0x22, "next": 0x22,
	"ctrl": 0x11, "control": 0x11, "control_l": 0x11, "control_r": 0x11, "shift": 0x10, "shift_l": 0x10, "shift_r": 0x10,
	"alt": 0x12, "alt_l": 0x12, "alt_r": 0x12, "super": 0x5B, "super_l": 0x5B, "win": 0x5B, "windows": 0x5B, "cmd": 0x5B, "meta": 0x5B,
	"capslock": 0x14, "caps_lock": 0x14, "print": 0x2C, "menu": 0x5D, "pause": 0x13,
	"minus": 0xBD, "plus": 0xBB, "equal": 0xBB, "comma": 0xBC, "period": 0xBE, "slash": 0xBF, "semicolon": 0xBA,
	"apostrophe": 0xDE, "bracketleft": 0xDB, "bracketright": 0xDD, "backslash": 0xDC, "grave": 0xC0,
}

// keyCombo transforma "ctrl+shift+t" em teclas virtuais (modificadores primeiro).
func keyCombo(text string) ([]uint16, error) {
	var out []uint16
	parts := strings.Split(text, "+")
	if strings.HasSuffix(text, "++") { // "ctrl++"
		parts = append(parts[:len(parts)-2], "plus")
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		l := strings.ToLower(p)
		if vk, ok := keyNames[l]; ok {
			out = append(out, vk)
			continue
		}
		if len(l) >= 2 && l[0] == 'f' {
			var n int
			if _, err := fmt.Sscanf(l[1:], "%d", &n); err == nil && n >= 1 && n <= 24 {
				out = append(out, uint16(0x70+n-1))
				continue
			}
		}
		if r := []rune(p); len(r) == 1 {
			v, _, _ := pVkKeyScanW.Call(uintptr(r[0]))
			if int16(v) == -1 {
				return nil, fmt.Errorf("unknown key %q", p)
			}
			if v&0x100 != 0 {
				out = append(out, 0x10)
			}
			if v&0x200 != 0 {
				out = append(out, 0x11)
			}
			if v&0x400 != 0 {
				out = append(out, 0x12)
			}
			out = append(out, uint16(v&0xFF))
			continue
		}
		return nil, fmt.Errorf("unknown key %q", p)
	}
	if len(out) == 0 {
		return nil, errors.New("no key given")
	}
	return out, nil
}

func pressCombo(keys []uint16) {
	for _, k := range keys {
		sendKey(k, false)
	}
	time.Sleep(30 * time.Millisecond)
	for i := len(keys) - 1; i >= 0; i-- {
		sendKey(keys[i], true)
	}
}

func modifiers(input map[string]any) ([]uint16, error) {
	t, _ := input["text"].(string)
	if strings.TrimSpace(t) == "" {
		return nil, nil
	}
	return keyCombo(t)
}

func num(input map[string]any, key string, def float64) float64 {
	if v, ok := input[key].(float64); ok {
		return v
	}
	return def
}

func (s *winScreen) Do(action string, input map[string]any) (string, error) {
	moveTo := func() error {
		if c, ok := input["coordinate"]; ok && c != nil {
			x, y, err := s.real(c)
			if err != nil {
				return err
			}
			pSetCursorPos.Call(uintptr(x), uintptr(y))
			time.Sleep(40 * time.Millisecond)
		}
		return nil
	}
	click := func(down, up uint32, times int) (string, error) {
		mods, err := modifiers(input)
		if err != nil {
			return "", err
		}
		if err := moveTo(); err != nil {
			return "", err
		}
		for _, m := range mods {
			sendKey(m, false)
		}
		for i := 0; i < times; i++ {
			sendMouse(down, 0)
			sendMouse(up, 0)
			time.Sleep(60 * time.Millisecond)
		}
		for i := len(mods) - 1; i >= 0; i-- {
			sendKey(mods[i], true)
		}
		return "OK", nil
	}

	switch action {
	case "left_click":
		return click(MOUSEEVENTF_LEFTDOWN, MOUSEEVENTF_LEFTUP, 1)
	case "right_click":
		return click(MOUSEEVENTF_RIGHTDOWN, MOUSEEVENTF_RIGHTUP, 1)
	case "middle_click":
		return click(MOUSEEVENTF_MIDDLEDOWN, MOUSEEVENTF_MIDDLEUP, 1)
	case "double_click":
		return click(MOUSEEVENTF_LEFTDOWN, MOUSEEVENTF_LEFTUP, 2)
	case "triple_click":
		return click(MOUSEEVENTF_LEFTDOWN, MOUSEEVENTF_LEFTUP, 3)
	case "mouse_move":
		if err := moveTo(); err != nil {
			return "", err
		}
		return "OK", nil
	case "left_mouse_down":
		sendMouse(MOUSEEVENTF_LEFTDOWN, 0)
		return "OK", nil
	case "left_mouse_up":
		sendMouse(MOUSEEVENTF_LEFTUP, 0)
		return "OK", nil
	case "left_click_drag":
		sx, sy, err := s.real(input["start_coordinate"])
		if err != nil {
			return "", err
		}
		ex, ey, err := s.real(input["coordinate"])
		if err != nil {
			return "", err
		}
		mods, err := modifiers(input)
		if err != nil {
			return "", err
		}
		for _, m := range mods {
			sendKey(m, false)
		}
		pSetCursorPos.Call(uintptr(sx), uintptr(sy))
		time.Sleep(50 * time.Millisecond)
		sendMouse(MOUSEEVENTF_LEFTDOWN, 0)
		for i := 1; i <= 15; i++ {
			x := sx + (ex-sx)*int32(i)/15
			y := sy + (ey-sy)*int32(i)/15
			pSetCursorPos.Call(uintptr(x), uintptr(y))
			time.Sleep(15 * time.Millisecond)
		}
		sendMouse(MOUSEEVENTF_LEFTUP, 0)
		for i := len(mods) - 1; i >= 0; i-- {
			sendKey(mods[i], true)
		}
		return "OK", nil
	case "cursor_position":
		var pt POINT
		pGetCursorPos.Call(ptr(&pt))
		s.mu.Lock()
		x, y := float64(pt.X)/s.scaleX, float64(pt.Y)/s.scaleY
		s.mu.Unlock()
		return fmt.Sprintf("X=%d, Y=%d", int(x), int(y)), nil
	case "scroll":
		if err := moveTo(); err != nil {
			return "", err
		}
		mods, err := modifiers(input)
		if err != nil {
			return "", err
		}
		n := int32(num(input, "scroll_amount", 3))
		if n < 1 {
			n = 1
		}
		if n > 30 {
			n = 30
		}
		dir, _ := input["scroll_direction"].(string)
		for _, m := range mods {
			sendKey(m, false)
		}
		for i := int32(0); i < n; i++ {
			switch dir {
			case "up":
				sendMouse(MOUSEEVENTF_WHEEL, 120)
			case "left":
				sendMouse(MOUSEEVENTF_HWHEEL, -120)
			case "right":
				sendMouse(MOUSEEVENTF_HWHEEL, 120)
			default:
				sendMouse(MOUSEEVENTF_WHEEL, -120)
			}
			time.Sleep(20 * time.Millisecond)
		}
		for i := len(mods) - 1; i >= 0; i-- {
			sendKey(mods[i], true)
		}
		return "OK", nil
	case "type":
		t, _ := input["text"].(string)
		for _, r := range t {
			if s.EscPressed() {
				return "", errors.New("stopped by the student")
			}
			switch r {
			case '\n':
				pressCombo([]uint16{0x0D})
			case '\r':
			case '\t':
				pressCombo([]uint16{0x09})
			default:
				sendUnicode(r)
			}
			time.Sleep(8 * time.Millisecond)
		}
		return "OK", nil
	case "key":
		t, _ := input["text"].(string)
		keys, err := keyCombo(t)
		if err != nil {
			return "", err
		}
		rep := int(num(input, "repeat", 1))
		if rep < 1 {
			rep = 1
		}
		if rep > 100 {
			rep = 100
		}
		for i := 0; i < rep; i++ {
			pressCombo(keys)
			time.Sleep(40 * time.Millisecond)
		}
		if len(keys) == 1 && keys[0] == VK_ESCAPE {
			s.EscPressed() // não confundir com o Esc do aluno
		}
		return "OK", nil
	case "hold_key":
		t, _ := input["text"].(string)
		keys, err := keyCombo(t)
		if err != nil {
			return "", err
		}
		d := math.Min(num(input, "duration", 1), 10)
		for _, k := range keys {
			sendKey(k, false)
		}
		time.Sleep(time.Duration(d * float64(time.Second)))
		for i := len(keys) - 1; i >= 0; i-- {
			sendKey(keys[i], true)
		}
		return "OK", nil
	case "wait":
		d := math.Min(num(input, "duration", 1), 30)
		end := time.Now().Add(time.Duration(d * float64(time.Second)))
		for time.Now().Before(end) {
			if s.EscPressed() {
				return "", errors.New("stopped by the student")
			}
			time.Sleep(100 * time.Millisecond)
		}
		return "OK", nil
	}
	return "", fmt.Errorf("action %q is not supported", action)
}
