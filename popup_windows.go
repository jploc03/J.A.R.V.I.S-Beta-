//go:build windows

package main

// Pop-ups do Jarvis no estilo HUD (substituem os avisos simples do Windows).

import (
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	popupW        = 380
	popupLife     = 10 * time.Second
	WM_POPUP_NEW  = WM_APP + 20
	WS_EX_NOACT   = 0x08000000
	timerPopupAni = 7
	maxPopups     = 3
)

type popup struct {
	hwnd    uintptr
	a       Alert
	born    time.Time
	alpha   int
	closing bool
	h       int32
}

var (
	popupsMu     sync.Mutex
	popups       []*popup
	popupQueue   []Alert
	popupClassOK bool
	pDestroyWindow = user32.NewProc("DestroyWindow")
)

// showPopup pode ser chamado de qualquer goroutine.
func showPopup(a Alert) {
	popupsMu.Lock()
	popupQueue = append(popupQueue, a)
	popupsMu.Unlock()
	pPostMessageW.Call(hwnd, WM_POPUP_NEW, 0, 0)
}

// createPopups roda na thread da janela principal.
func createPopups() {
	popupsMu.Lock()
	q := popupQueue
	popupQueue = nil
	popupsMu.Unlock()
	if !popupClassOK {
		wc := WNDCLASSEXW{LpfnWndProc: syscall.NewCallback(popupProc), HInstance: hInst, LpszClassName: u16("JarvisPopup")}
		wc.HCursor, _, _ = pLoadCursorW.Call(0, IDC_HAND)
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassExW.Call(ptr(&wc))
		popupClassOK = true
	}
	for _, a := range q {
		p := &popup{a: a, born: time.Now()}
		p.h = int32(104 * dpiScale())
		h, _, _ := pCreateWindowExW.Call(WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_LAYERED|WS_EX_NOACT,
			uintptr(unsafe.Pointer(u16("JarvisPopup"))), uintptr(unsafe.Pointer(u16("Jarvis"))), WS_POPUP,
			0, 0, uintptr(int32(popupW*dpiScale())), uintptr(p.h), 0, 0, hInst, 0)
		if h == 0 {
			continue
		}
		p.hwnd = h
		pSetLayeredWindowAttribs.Call(h, 0, 0, LWA_ALPHA)
		popupsMu.Lock()
		popups = append(popups, p)
		for len(popups) > maxPopups { // fecha o mais antigo
			popups[0].closing = true
			popups = popups[1:]
		}
		popupsMu.Unlock()
		layoutPopups()
		pShowWindow.Call(h, SW_SHOWNOACTIVATE)
		pSetTimer.Call(h, timerPopupAni, 30, 0)
		playNotifySound()
	}
}

func playNotifySound() {
	const SND_ASYNC, SND_ALIAS, SND_NODEFAULT = 0x1, 0x10000, 0x2
	pPlaySoundW.Call(uintptr(unsafe.Pointer(u16("Notification.Default"))), 0, SND_ALIAS|SND_ASYNC|SND_NODEFAULT)
}

// layoutPopups empilha os pop-ups no canto inferior direito.
func layoutPopups() {
	var wa RECT
	pSystemParametersInfoW.Call(SPI_GETWORKAREA, 0, ptr(&wa), 0)
	s := dpiScale()
	m := int32(16 * s)
	w := int32(popupW * s)
	y := wa.Bottom - m
	popupsMu.Lock()
	defer popupsMu.Unlock()
	for i := len(popups) - 1; i >= 0; i-- {
		p := popups[i]
		y -= p.h
		pSetWindowPos.Call(p.hwnd, ^uintptr(0), uintptr(wa.Right-w-m), uintptr(y), uintptr(w), uintptr(p.h), SWP_NOACTIVATE)
		rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(p.h+1), uintptr(int32(10*s)), uintptr(int32(10*s)))
		pSetWindowRgn.Call(p.hwnd, rgn, 1)
		y -= int32(10 * s)
	}
}

func findPopup(h uintptr) *popup {
	popupsMu.Lock()
	defer popupsMu.Unlock()
	for _, p := range popups {
		if p.hwnd == h {
			return p
		}
	}
	return nil
}

func closePopup(p *popup) {
	popupsMu.Lock()
	for i, q := range popups {
		if q == p {
			popups = append(popups[:i], popups[i+1:]...)
			break
		}
	}
	popupsMu.Unlock()
	pKillTimer.Call(p.hwnd, timerPopupAni)
	pDestroyWindow.Call(p.hwnd)
	layoutPopups()
}

var orphanClosing = map[uintptr]bool{}

func popupProc(h, msg, wp, lp uintptr) uintptr {
	switch msg {
	case WM_ERASEBKGND:
		return 1
	case WM_TIMER:
		p := findPopup(h)
		if p == nil { // foi tirado da pilha (muitos pop-ups): some aos poucos
			orphanClosing[h] = true
			pKillTimer.Call(h, timerPopupAni)
			pDestroyWindow.Call(h)
			delete(orphanClosing, h)
			return 0
		}
		age := time.Since(p.born)
		switch {
		case p.closing || age > popupLife:
			p.alpha -= 30
			if p.alpha <= 0 {
				closePopup(p)
				return 0
			}
		case p.alpha < 245:
			p.alpha += 35
			if p.alpha > 245 {
				p.alpha = 245
			}
		}
		pSetLayeredWindowAttribs.Call(h, 0, uintptr(p.alpha), LWA_ALPHA)
		pInvalidateRect.Call(h, 0, 0)
		return 0
	case WM_PAINT:
		paintPopup(h)
		return 0
	case WM_LBUTTONUP:
		p := findPopup(h)
		if p == nil {
			return 0
		}
		x := int32(int16(lp & 0xFFFF))
		y := int32(int16((lp >> 16) & 0xFFFF))
		s := dpiScale()
		if !(x > int32((popupW-34)*s) && y < int32(30*s)) { // fora do X: abre
			switch {
			case p.a.URL != "":
				shellOpen(p.a.URL, "")
			case p.a.Key == "timer":
				timer.Cancel()
				fallthrough
			default:
				if currentCfg().Mode == "mini" {
					showBox()
				} else {
					openHome()
				}
			}
		}
		p.closing = true
		return 0
	case WM_SETCURSOR:
		c, _, _ := pLoadCursorW.Call(0, IDC_HAND)
		pSetCursor.Call(c)
		return 1
	}
	r, _, _ := pDefWindowProcW.Call(h, msg, wp, lp)
	return r
}

func paintPopup(h uintptr) {
	p := findPopup(h)
	var ps PAINTSTRUCT
	screenDC, _, _ := pBeginPaint.Call(h, ptr(&ps))
	defer pEndPaint.Call(h, ptr(&ps))
	if p == nil {
		return
	}
	applyPalette(currentCfg().Theme)
	s := dpiScale()
	W, H := int32(popupW*s), p.h
	mem, _, _ := pCreateCompatibleDC.Call(screenDC)
	bmp, _, _ := pCreateCompatibleBitmap.Call(screenDC, uintptr(W), uintptr(H))
	old, _, _ := pSelectObject.Call(mem, bmp)
	defer func() { pSelectObject.Call(mem, old); pDeleteObject.Call(bmp); pDeleteDC.Call(mem) }()
	pt := &painter{hdc: mem, s: s}
	defer pt.release()
	pSetBkMode.Call(mem, TRANSPARENT)

	// fundo com leve gradiente da cor do tema
	acc := colCyan
	ar, ag, ab := float64(acc&0xFF), float64((acc>>8)&0xFF), float64((acc>>16)&0xFF)
	br, bg, bb := float64(colBg&0xFF), float64((colBg>>8)&0xFF), float64((colBg>>16)&0xFF)
	for i := int32(0); i < 24; i++ {
		f := 0.16 * (1 - float64(i)/24)
		pt.fill(W*i/24, 0, W/24+2, H, rgb(uint8(br+(ar-br)*f), uint8(bg+(ag-bg)*f), uint8(bb+(ab-bb)*f)))
	}
	// barra de destaque à esquerda
	pt.fill(0, 0, pt.px(3), H, acc)

	fHead := pt.font(8.5, 700, faceHudBold())
	fTitle := pt.font(16, 600, faceTech())
	fText := pt.font(12.5, 400, "Segoe UI")
	fIcon := pt.font(24, 400, "Segoe UI Emoji")
	fX := pt.font(14, 400, "Segoe UI")

	// ícone num círculo brilhante
	cx, cy, r := pt.px(38), H/2-pt.px(4), pt.px(22)
	pt.roundBox(cx-r, cy-r, 2*r, 2*r, 2*r, colBoxBg, acc)
	pt.text(p.a.Icon, cx-r, cy-r, 2*r, 2*r, acc, fIcon, DT_CENTER|DT_VCENTER|DT_SINGLELINE)

	x := pt.px(72)
	tw := W - x - pt.px(36)
	dotted, _ := assistantName(currentCfg().Theme)
	pSetTextCharacterExtra.Call(mem, uintptr(pt.px(1.5)))
	pt.text(dotted+"  ·  "+time.Now().Format("15:04"), x, pt.px(10), tw, pt.px(14), acc, fHead, DT_SINGLELINE)
	pSetTextCharacterExtra.Call(mem, 0)
	y := pt.px(26)
	y += pt.text(p.a.Title, x, y, tw, pt.px(22), colWhite, fTitle, DT_SINGLELINE|DT_END_ELLIPSIS)
	pt.text(p.a.Text, x, y+pt.px(1), tw, H-y-pt.px(14), colText, fText, DT_WORDBREAK|DT_END_ELLIPSIS)
	pt.text("×", W-pt.px(30), pt.px(4), pt.px(24), pt.px(24), colDim, fX, DT_CENTER|DT_VCENTER|DT_SINGLELINE)

	// barra de tempo
	left := 1 - float64(time.Since(p.born))/float64(popupLife)
	if left < 0 {
		left = 0
	}
	pt.fill(pt.px(3), H-pt.px(3), int32(float64(W-pt.px(3))*left), pt.px(3), acc)

	// borda + cantos HUD
	pen, _, _ := pCreatePen.Call(0, uintptr(pt.px(1)), colBorder)
	nb, _, _ := pGetStockObject.Call(NULL_BRUSH)
	op, _, _ := pSelectObject.Call(mem, pen)
	ob, _, _ := pSelectObject.Call(mem, nb)
	pRoundRect.Call(mem, 0, 0, uintptr(W), uintptr(H), uintptr(pt.px(10)), uintptr(pt.px(10)))
	pSelectObject.Call(mem, op)
	pSelectObject.Call(mem, ob)
	pDeleteObject.Call(pen)
	cl, ct := pt.px(14), pt.px(2)
	pt.fill(W-cl-pt.px(3), pt.px(3), cl, ct, acc)
	pt.fill(W-ct-pt.px(3), pt.px(3), ct, cl, acc)
	pt.fill(W-cl-pt.px(3), H-pt.px(5), cl, ct, acc)

	pBitBlt.Call(screenDC, 0, 0, uintptr(W), uintptr(H), mem, 0, 0, SRCCOPY)
}
