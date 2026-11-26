//go:build windows

package main

import (
	"fmt"
	"strings"
	"time"
	"unsafe"
)

var (
	colBg     = rgb(8, 16, 28)
	colBorder = rgb(70, 170, 220)
	colCyan   = rgb(95, 208, 255)
	colWhite  = rgb(255, 255, 255)
	colText   = rgb(223, 246, 255)
	colDim    = rgb(136, 153, 170)
	colDate   = rgb(143, 216, 255)
	colGreen  = rgb(111, 227, 168)
	colRed    = rgb(255, 154, 154)
	colYellow = rgb(255, 204, 102)
	colLine   = rgb(28, 58, 82)
	colBoxBg  = rgb(12, 38, 60)
	colBoxBd  = rgb(40, 95, 130)
	colBtn    = rgb(10, 124, 194)
)

type painter struct {
	hdc   uintptr
	s     float64
	fonts []uintptr
}

// paleta do quadrado conforme o tema
func applyPalette(theme string) {
	// ordem: destaque, data, borda, linha, caixa, borda da caixa, fundo, botão
	ps := map[string][8][3]uint8{
		"jarvis":  {{95, 208, 255}, {143, 216, 255}, {70, 170, 220}, {28, 58, 82}, {12, 38, 60}, {40, 95, 130}, {8, 16, 28}, {10, 124, 194}},
		"friday":  {{255, 95, 183}, {255, 179, 220}, {230, 80, 165}, {74, 26, 64}, {52, 14, 44}, {140, 40, 110}, {20, 7, 17}, {214, 51, 143}},
		"ironman": {{255, 201, 74}, {255, 227, 160}, {220, 160, 60}, {74, 22, 18}, {48, 14, 12}, {140, 60, 30}, {16, 6, 6}, {184, 29, 29}},
		"emerald": {{52, 245, 164}, {166, 255, 217}, {40, 200, 140}, {18, 66, 50}, {8, 40, 28}, {30, 110, 80}, {4, 18, 13}, {11, 155, 99}},
		"galaxy":  {{169, 139, 255}, {126, 232, 255}, {140, 110, 230}, {48, 34, 106}, {26, 16, 64}, {80, 60, 160}, {10, 7, 26}, {107, 69, 230}},
	}
	q, ok := ps[theme]
	if !ok {
		q = ps["jarvis"]
	}
	c := func(v [3]uint8) uintptr { return rgb(v[0], v[1], v[2]) }
	colCyan, colDate, colBorder, colLine = c(q[0]), c(q[1]), c(q[2]), c(q[3])
	colBoxBg, colBoxBd, colBg, colBtn = c(q[4]), c(q[5]), c(q[6]), c(q[7])
}

func (p *painter) px(v float64) int32 { return int32(v*p.s + 0.5) }

func (p *painter) font(size float64, weight int, face string) uintptr {
	f, _, _ := pCreateFontW.Call(uintptr(-p.px(size)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(u16(face))))
	p.fonts = append(p.fonts, f)
	return f
}

func (p *painter) release() {
	for _, f := range p.fonts {
		pDeleteObject.Call(f)
	}
}

// Desenha texto; devolve a altura usada.
func (p *painter) text(s string, x, y, w, h int32, col, font uintptr, flags uintptr) int32 {
	pSelectObject.Call(p.hdc, font)
	pSetTextColor.Call(p.hdc, col)
	r := RECT{x, y, x + w, y + h}
	str := u16(s)
	if flags&DT_WORDBREAK != 0 {
		c := r
		pDrawTextW.Call(p.hdc, uintptr(unsafe.Pointer(str)), ^uintptr(0), ptr(&c), flags|DT_CALCRECT|DT_NOPREFIX)
		r.Bottom = r.Top + (c.Bottom - c.Top)
	}
	pDrawTextW.Call(p.hdc, uintptr(unsafe.Pointer(str)), ^uintptr(0), ptr(&r), flags|DT_NOPREFIX)
	return r.Bottom - r.Top
}

func (p *painter) measure(s string, font uintptr) (int32, int32) {
	pSelectObject.Call(p.hdc, font)
	var r RECT
	pDrawTextW.Call(p.hdc, uintptr(unsafe.Pointer(u16(s))), ^uintptr(0), ptr(&r), DT_CALCRECT|DT_SINGLELINE|DT_NOPREFIX)
	return r.Right - r.Left, r.Bottom - r.Top
}

func (p *painter) fill(x, y, w, h int32, col uintptr) {
	b, _, _ := pCreateSolidBrush.Call(col)
	r := RECT{x, y, x + w, y + h}
	pFillRect.Call(p.hdc, ptr(&r), b)
	pDeleteObject.Call(b)
}

func (p *painter) roundBox(x, y, w, h, rad int32, fillCol, borderCol uintptr) {
	b, _, _ := pCreateSolidBrush.Call(fillCol)
	pen, _, _ := pCreatePen.Call(0, uintptr(p.px(1)), borderCol)
	ob, _, _ := pSelectObject.Call(p.hdc, b)
	op, _, _ := pSelectObject.Call(p.hdc, pen)
	pRoundRect.Call(p.hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), uintptr(rad), uintptr(rad))
	pSelectObject.Call(p.hdc, ob)
	pSelectObject.Call(p.hdc, op)
	pDeleteObject.Call(b)
	pDeleteObject.Call(pen)
}

func hhmm(t time.Time) string { return t.Format("15:04") }

func paint(h uintptr) {
	var ps PAINTSTRUCT
	screen, _, _ := pBeginPaint.Call(h, ptr(&ps))
	defer pEndPaint.Call(h, ptr(&ps))

	applyPalette(currentCfg().Theme)
	s := uiScale()
	W := int32(baseW * s)
	maxH := int32(1400 * s)
	mem, _, _ := pCreateCompatibleDC.Call(screen)
	bmp, _, _ := pCreateCompatibleBitmap.Call(screen, uintptr(W), uintptr(maxH))
	oldBmp, _, _ := pSelectObject.Call(mem, bmp)
	defer func() {
		pSelectObject.Call(mem, oldBmp)
		pDeleteObject.Call(bmp)
		pDeleteDC.Call(mem)
	}()

	p := &painter{hdc: mem, s: s}
	defer p.release()
	pSetBkMode.Call(mem, TRANSPARENT)
	p.fill(0, 0, W, maxH, colBg)
	if !currentCfg().NoAnimation {
		data.mu.Lock()
		wx := data.Weather
		data.mu.Unlock()
		top := skyTopColor(sceneFor(wx, time.Now()))
		gh := curHeight
		if gh <= 0 {
			gh = int32(400 * s)
		}
		const bands = 48
		for i := int32(0); i < bands; i++ {
			f := float64(i) / bands
			bgR, bgG, bgB := float64(colBg&0xFF), float64((colBg>>8)&0xFF), float64((colBg>>16)&0xFF)
			r := uint8(float64(top[0])*(1-f) + bgR*f)
			g := uint8(float64(top[1])*(1-f) + bgG*f)
			b := uint8(float64(top[2])*(1-f) + bgB*f)
			p.fill(0, gh*i/bands, W, gh/bands+2, rgb(r, g, b))
		}
	}

	fClock := p.font(34, 500, faceHud())
	fSec := p.font(14, 500, faceHud())
	fBig := p.font(22, 500, faceHud())
	fNorm := p.font(13, 400, "Segoe UI")
	fBold := p.font(13.5, 600, "Segoe UI Semibold")
	fSmall := p.font(11.5, 400, "Segoe UI")
	fHead := p.font(9.5, 700, faceHudBold())
	fNext := p.font(19, 600, faceTech())
	fTime := p.font(14, 600, faceTech())
	fDate := p.font(14, 600, faceTech())
	fX := p.font(16, 400, "Segoe UI")
	fBtn := p.font(9, 700, faceHudBold())

	now := time.Now()
	data.mu.Lock()
	evs := append([]Event(nil), data.Events...)
	w := data.Weather
	state, cerr, upd := data.CanvasState, data.CanvasErr, data.UpdatedAt
	data.mu.Unlock()
	sum := analyse(evs, now)

	hits = hits[:0]
	pad := p.px(18)
	inner := W - 2*pad
	y := p.px(14)

	// Fechar
	xw, xh := p.measure("×", fX)
	xr := RECT{W - p.px(10) - xw - p.px(6), p.px(4), W - p.px(6), p.px(4) + xh + p.px(4)}
	p.text("×", xr.Left, xr.Top, xr.Right-xr.Left, xr.Bottom-xr.Top, colDim, fX, DT_CENTER|DT_SINGLELINE|DT_VCENTER)
	hits = append(hits, hit{r: xr, action: "close"})

	// Relógio
	clk := hhmm(now)
	cw, ch := p.measure(clk, fClock)
	for _, o := range [][2]int32{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		p.text(clk, pad+o[0], y+o[1], cw+p.px(4), ch, rgb(20, 90, 130), fClock, DT_SINGLELINE)
	}
	p.text(clk, pad, y, cw+p.px(4), ch, colWhite, fClock, DT_SINGLELINE)
	_, sh := p.measure(":00", fSec)
	p.text(fmt.Sprintf(":%02d", now.Second()), pad+cw+p.px(3), y+ch-sh-p.px(5), p.px(40), sh, colCyan, fSec, DT_SINGLELINE)

	// Tempo (direita)
	wx := pad + p.px(120)
	ww := W - pad - p.px(20) - wx
	wy := y + p.px(2)
	if w.OK {
		wy += p.text(fmt.Sprintf("%.0f°", w.Temp), wx, wy, ww, p.px(32), colWhite, fBig, DT_RIGHT|DT_SINGLELINE)
		wy += p.text(weatherText(w.Code), wx, wy, ww, p.px(18), colDate, fSmall, DT_RIGHT|DT_SINGLELINE)
		wy += p.text(fmt.Sprintf("%.0f° / %.0f°  ·  rain %d%%", w.TodayMin, w.TodayMax, w.TodayRain), wx, wy, ww, p.px(18), colDim, fSmall, DT_RIGHT|DT_SINGLELINE)
		p.text(fmt.Sprintf("Tomorrow %.0f°  %s", w.TomMax, strings.ToLower(weatherText(w.TomCode))), wx, wy, ww, p.px(18), colDim, fSmall, DT_RIGHT|DT_SINGLELINE|DT_END_ELLIPSIS)
	} else {
		p.text("weather\nloading…", wx, wy+p.px(6), ww, p.px(40), colDim, fSmall, DT_RIGHT|DT_WORDBREAK)
	}
	y += ch + p.px(2)
	y += p.text(strings.ToUpper(now.Format("Monday 2 January")), pad, y+p.px(3), p.px(200), p.px(20), colDate, fDate, DT_SINGLELINE) + p.px(3)
	y += p.text(greeting(now)+userSuffix()+" · "+currentCfg().City, pad, y, p.px(200), p.px(18), colDim, fSmall, DT_SINGLELINE)
	if wy+p.px(18) > y {
		y = wy + p.px(18)
	}

	section := func(title, right string) {
		y += p.px(10)
		p.fill(pad, y, inner, p.px(1), colLine)
		p.fill(pad, y, p.px(26), p.px(2), colCyan)
		y += p.px(10)
		pSetTextCharacterExtra.Call(mem, uintptr(p.px(2)))
		p.text(title, pad, y, inner, p.px(16), colCyan, fHead, DT_SINGLELINE)
		pSetTextCharacterExtra.Call(mem, 0)
		p.text(right, pad, y, inner, p.px(16), colDim, fSmall, DT_RIGHT|DT_SINGLELINE)
		y += p.px(20)
	}

	// Lição de casa
	section("HOMEWORK", fmt.Sprintf("%d item%s", len(sum.HwToday)+len(sum.HwTomorrow), plural(len(sum.HwToday)+len(sum.HwTomorrow))))
	hwList := func(label string, list []Event) {
		y += p.text(label, pad, y, inner, p.px(18), colDim, fSmall, DT_SINGLELINE)
		if len(list) == 0 {
			y += p.text("✓  Nothing due", pad+p.px(8), y, inner, p.px(20), colGreen, fNorm, DT_SINGLELINE) + p.px(4)
			return
		}
		for _, e := range list {
			top := y
			t := hhmm(e.Start)
			if t == "00:00" {
				t = "—"
			} else if t == "23:59" {
				t = "end of day"
			}
			tagTxt, tagCol := e.Source, colDim
			if e.Done {
				tagTxt, tagCol = "✓ done", colGreen
			} else if e.Missing || (e.Start.Before(now) && t != "—") {
				tagTxt, tagCol = "late", colRed
			}
			tw, th := p.measure(tagTxt, fSmall)
			tagX := pad + inner - tw - p.px(12)
			p.roundBox(tagX, y+p.px(3), tw+p.px(12), th+p.px(4), p.px(10), blend(tagCol), blend(tagCol))
			p.text(tagTxt, tagX, y+p.px(5), tw+p.px(12), th, tagCol, fSmall, DT_CENTER|DT_SINGLELINE)
			colX := pad + p.px(70)
			if t == "end of day" {
				p.text(t, pad+p.px(4), y+p.px(2), p.px(66), p.px(18), colDate, fSmall, DT_SINGLELINE)
			} else {
				p.text(t, pad+p.px(4), y, p.px(66), p.px(20), colDate, fTime, DT_SINGLELINE)
			}
			titleCol := colWhite
			if e.Done {
				titleCol = colDim
			}
			y += p.text(e.Title, colX, y, tagX-colX-p.px(6), p.px(20), titleCol, fBold, DT_SINGLELINE|DT_END_ELLIPSIS)
			if e.Course != "" {
				y += p.text(e.Course, colX, y, tagX-colX-p.px(6), p.px(17), colDim, fSmall, DT_SINGLELINE|DT_END_ELLIPSIS)
			}
			y += p.px(5)
			if e.URL != "" {
				hits = append(hits, hit{r: RECT{pad, top, pad + inner, y}, action: "url", url: e.URL})
			}
		}
	}
	hwList("Today", sum.HwToday)
	hwList("Tomorrow", sum.HwTomorrow)

	// Próxima aula
	right := ""
	if sum.RestToday > 0 {
		right = fmt.Sprintf("%d more today", sum.RestToday)
	}
	section("NEXT CLASS", right)
	if sum.Current != nil {
		y += p.text("Now: "+sum.Current.Title+" until "+hhmm(sum.Current.End), pad, y, inner, p.px(20), colYellow, fNorm, DT_SINGLELINE|DT_END_ELLIPSIS) + p.px(4)
	}
	if n := sum.Next; n != nil {
		boxTop := y
		bx := pad + p.px(12)
		bw := inner - p.px(24)
		yy := y + p.px(9)
		// mede primeiro, desenha a caixa, depois o texto por cima
		when := "today"
		if !sameDay(n.Start, now) {
			when = n.Start.Format("Monday")
			if sameDay(n.Start, now.AddDate(0, 0, 1)) {
				when = "tomorrow"
			}
		}
		line2 := when + " at " + hhmm(n.Start)
		if !n.End.IsZero() {
			line2 += "–" + hhmm(n.End)
		}
		if n.Location != "" {
			line2 += "  ·  " + n.Location
		}
		_, h1 := p.measure("Ag", fNext)
		_, h2 := p.measure("Ag", fNorm)
		boxH := p.px(9)*2 + h1 + h2*2 + p.px(2)
		p.roundBox(pad, boxTop, inner, boxH, p.px(16), colBoxBg, colBoxBd)
		yy += p.text(n.Title, bx, yy, bw, h1, colWhite, fNext, DT_SINGLELINE|DT_END_ELLIPSIS)
		yy += p.text(line2, bx, yy, bw, h2, colText, fNorm, DT_SINGLELINE|DT_END_ELLIPSIS)
		p.text(countdown(n.Start.Sub(now)), bx, yy, bw, h2, colCyan, fNorm, DT_SINGLELINE)
		y = boxTop + boxH
	} else {
		y += p.text("No classes found. Add your timetable in Settings.", pad+p.px(4), y, inner, p.px(40), colDim, fNorm, DT_WORDBREAK)
		hits = append(hits, hit{r: RECT{pad, y - p.px(36), pad + inner, y}, action: "settings", url: "#classes"})
	}

	// Avisos
	warn := func(msg string, action string, anchor string) {
		y += p.px(10)
		top := y
		_ = top
		tmp := RECT{0, 0, inner - p.px(20), 0}
		pSelectObject.Call(mem, fSmall)
		pDrawTextW.Call(mem, uintptr(unsafe.Pointer(u16(msg))), ^uintptr(0), ptr(&tmp), DT_WORDBREAK|DT_CALCRECT|DT_NOPREFIX)
		bh := tmp.Bottom + p.px(14)
		p.roundBox(pad, y, inner, bh, p.px(12), rgb(40, 34, 16), rgb(90, 72, 30))
		p.text(msg, pad+p.px(10), y+p.px(7), inner-p.px(20), bh, colYellow, fSmall, DT_WORDBREAK)
		hits = append(hits, hit{r: RECT{pad, y, pad + inner, y + bh}, action: action, url: anchor})
		y += bh
	}
	switch state {
	case "notoken":
		warn("To see your homework, add your Canvas token. Click here to open Settings (the steps are inside).", "settings", "#canvas")
	case "badtoken":
		warn("Canvas refused the token (wrong or expired). Click here to paste a new one in Settings.", "settings", "#canvas")
	case "error":
		warn("Couldn't reach Canvas ("+cerr+"). Click to try again.", "refresh", "")
	}
	if !hotkeyOK {
		warn("Shortcut "+currentCfg().Hotkey+" is used by another program. Click to change it in Settings.", "settings", "#general")
	}

	// Timer
	if tv := timer.View(); tv.State != "idle" {
		y += p.px(10)
		top := y
		txt := "Timer  " + fmtDur(time.Duration(tv.LeftMs)*time.Millisecond)
		col := rgb(255, 159, 10)
		switch tv.State {
		case "paused":
			txt += "  (paused)"
		case "done":
			txt, col = "Time's up!  Click to stop", rgb(255, 105, 97)
		default:
			txt += "  · ends " + time.Now().Add(time.Duration(tv.LeftMs)*time.Millisecond).Format("15:04")
		}
		if tv.Label != "" && tv.State != "done" {
			txt += "  · " + tv.Label
		}
		_, th := p.measure("Ag", fBold)
		bh := th + p.px(12)
		p.roundBox(pad, y, inner, bh, p.px(12), rgb(40, 28, 8), rgb(110, 70, 10))
		// barra de progresso
		if tv.State != "done" && tv.TotalMs > 0 {
			frac := float64(tv.LeftMs) / float64(tv.TotalMs)
			p.fill(pad+p.px(6), y+bh-p.px(4), int32(float64(inner-p.px(12))*frac), p.px(2), col)
		}
		p.text(txt, pad+p.px(10), y, inner-p.px(20), bh, col, fBold, DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS)
		y += bh
		act := "window"
		if tv.State == "done" {
			act = "timerstop"
		}
		hits = append(hits, hit{r: RECT{pad, top, pad + inner, y}, action: act})
	}

	// Rodapé: status + botões
	y += p.px(12)
	status := "Loading…"
	if busy {
		status = "Updating…"
	} else if !upd.IsZero() {
		status = "Updated " + hhmm(upd)
	}
	p.text(status, pad, y, inner, p.px(16), colDim, fSmall, DT_SINGLELINE)
	p.text("drag to move · Esc hide", pad, y, inner, p.px(16), colDim, fSmall, DT_RIGHT|DT_SINGLELINE)
	y += p.px(16) + p.px(8)
	type btn struct {
		label, action string
		primary       bool
	}
	btns := []btn{{"ASK", "chat", true}, {"WINDOW", "window", false}, {"SETTINGS", "settings", false}, {"−", "zoomout", false}, {"+", "zoomin", false}}
	bx := pad
	gap := p.px(6)
	_, bh := p.measure("Ag", fBtn)
	bh += p.px(10)
	for _, b := range btns {
		tw, _ := p.measure(b.label, fBtn)
		bw := tw + p.px(20)
		if b.label == "−" || b.label == "+" {
			bw = bh + p.px(4)
		}
		fillC, borderC, textC := colBg, colBoxBd, colCyan
		if b.primary {
			fillC, borderC, textC = colBtn, colBtn, colWhite
		}
		p.roundBox(bx, y, bw, bh, p.px(12), fillC, borderC)
		p.text(b.label, bx, y, bw, bh, textC, fBtn, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		hits = append(hits, hit{r: RECT{bx, y, bx + bw, y + bh}, action: b.action})
		bx += bw + gap
	}
	y += bh + p.px(14)

	// Borda
	pen, _, _ := pCreatePen.Call(0, uintptr(p.px(1.5)), colBorder)
	nb, _, _ := pGetStockObject.Call(NULL_BRUSH)
	op, _, _ := pSelectObject.Call(mem, pen)
	ob, _, _ := pSelectObject.Call(mem, nb)
	pRoundRect.Call(mem, 0, 0, uintptr(W), uintptr(y), uintptr(p.px(10)), uintptr(p.px(10)))
	pSelectObject.Call(mem, op)
	pSelectObject.Call(mem, ob)
	pDeleteObject.Call(pen)
	// cantos brilhantes estilo HUD
	cl, ct := p.px(18), p.px(2)
	in := p.px(4)
	for _, c := range [][2]int32{{in, in}, {W - in, in}, {in, y - in}, {W - in, y - in}} {
		x0, y0 := c[0], c[1]
		dx, dy := int32(1), int32(1)
		if x0 > W/2 {
			dx = -1
		}
		if y0 > y/2 {
			dy = -1
		}
		hx := x0
		if dx < 0 {
			hx = x0 - cl
		}
		vy := y0
		if dy < 0 {
			vy = y0 - cl
		}
		hy := y0
		if dy < 0 {
			hy = y0 - ct
		}
		vx := x0
		if dx < 0 {
			vx = x0 - ct
		}
		p.fill(hx, hy, cl, ct, colCyan)
		p.fill(vx, vy, ct, cl, colCyan)
	}

	pBitBlt.Call(screen, 0, 0, uintptr(W), uintptr(y), mem, 0, 0, SRCCOPY)
	if y != curHeight {
		placeWindow(y)
		pInvalidateRect.Call(h, 0, 0)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// Mistura uma cor com o fundo (fundo das etiquetas)
func blend(c uintptr) uintptr {
	r, g, b := c&0xFF, (c>>8)&0xFF, (c>>16)&0xFF
	br, bg, bb := colBg&0xFF, (colBg>>8)&0xFF, (colBg>>16)&0xFF
	return rgb(uint8((r*22+br*78)/100), uint8((g*22+bg*78)/100), uint8((b*22+bb*78)/100))
}

func fmtDur(d time.Duration) string {
	t := int((d + time.Second - 1) / time.Second)
	if t < 0 {
		t = 0
	}
	h, m, sec := t/3600, t%3600/60, t%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d", m, sec)
}

// cor do topo do quadrado conforme o céu (escura o bastante para o texto)
func skyTopColor(sky, cond string) [3]uint8 {
	switch sky {
	case "night":
		if cond == "rain" || cond == "storm" || cond == "cloudy" || cond == "fog" {
			return [3]uint8{16, 22, 32}
		}
		return [3]uint8{10, 22, 56}
	case "sunset", "dawn":
		return [3]uint8{112, 48, 72}
	}
	switch cond {
	case "rain", "storm":
		return [3]uint8{40, 54, 70}
	case "cloudy", "fog", "snow":
		return [3]uint8{52, 66, 82}
	case "partly":
		return [3]uint8{22, 70, 112}
	}
	return [3]uint8{14, 78, 140}
}

func userSuffix() string {
	if n := currentCfg().UserName; n != "" {
		return ", " + n
	}
	return ""
}
