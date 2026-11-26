//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	baseW       = 360
	timerClock  = 1
	cmdShow     = 1
	cmdRefresh  = 2
	cmdSettings = 3
	cmdStartup  = 4
	cmdExit     = 5
	cmdChat     = 6
	hotkeyID    = 1

	WM_RELOAD        = WM_APP + 3
	WM_QUIT_JARVIS   = WM_APP + 6
	WM_SHOW_JARVIS   = WM_APP + 7
	WM_GO_MINI       = WM_APP + 8
	cmdWindow        = 7
	cmdRobots        = 9
	cmdMini          = 8
	WM_HIDE_OVERLAY  = WM_APP + 4
	WM_SHOW_OVERLAY  = WM_APP + 5
	WM_NCHITTEST     = 0x0084
	WM_NCLBUTTONDBLCLK = 0x00A3
	WM_EXITSIZEMOVE  = 0x0232
	WM_MOUSEWHEEL    = 0x020A
	HTCLIENT         = 1
	HTCAPTION        = 2
)

type hit struct {
	r      RECT
	action string // url | close | settings | refresh | chat | zoomin | zoomout
	url    string
}

var (
	hwnd       uintptr
	hInst      uintptr
	appIcon    uintptr
	data       = &Data{}
	cfgMu      sync.Mutex
	cfg        Config
	cfgPath    string
	appDir     string
	exePath    string
	srv        *Server
	curHeight  int32
	hits       []hit
	busy       bool
	hotkeyOK   bool
	wasVisible bool
)

func init() { runtime.LockOSThread() }

func currentCfg() Config {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	return cfg
}

func main() {
	exePath, _ = os.Executable()
	if installOrUpdate() {
		return // a cópia instalada foi aberta; esta termina
	}
	justUpdated := len(os.Args) > 1 && os.Args[1] == "--updated"

	_, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16("JarvisEscolar.SingleInstance"))))
	if errno, ok := err.(syscall.Errno); ok && errno == 183 {
		// já está aberto: só mostra
		if h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("JarvisEscolarWindow"))), 0); h != 0 {
			pPostMessageW.Call(h, WM_SHOW_JARVIS, 0, 0)
		}
		return
	}

	appDir = filepath.Join(os.Getenv("APPDATA"), "Jarvis")
	_ = os.MkdirAll(appDir, 0o755)
	cfgPath = filepath.Join(appDir, "settings.json")
	var existed bool
	cfg, existed = loadSettings(cfgPath)
	firstRun := false
	if !existed {
		_ = saveSettings(cfgPath, cfg)
		if _, err := os.Stat(filepath.Join(appDir, "jarvis-settings.txt")); err != nil {
			firstRun = true
			setStartup(true, exePath)
		}
	}

	if pSetProcessDpiAwarenessCx.Find() == nil {
		pSetProcessDpiAwarenessCx.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2
	}

	loadHudFonts()
	hInst, _, _ = pGetModuleHandleW.Call(0)
	for id := uintptr(1); id < 20 && appIcon == 0; id++ {
		appIcon, _, _ = pLoadIconW.Call(hInst, id)
	}
	if appIcon == 0 {
		appIcon, _, _ = pLoadIconW.Call(0, IDI_APPLICATION)
	}
	cursor, _, _ := pLoadCursorW.Call(0, IDC_ARROW)

	className := u16("JarvisEscolarWindow")
	wc := WNDCLASSEXW{
		LpfnWndProc: syscall.NewCallback(wndProc), HInstance: hInst, HCursor: cursor,
		HIcon: appIcon, HIconSm: appIcon, LpszClassName: className,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(ptr(&wc))
	hwnd, _, _ = pCreateWindowExW.Call(WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_LAYERED,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(u16("Jarvis"))), WS_POPUP,
		0, 0, 360, 500, 0, 0, hInst, 0)
	pSetLayeredWindowAttribs.Call(hwnd, 0, 246, LWA_ALPHA)

	// Ganchos usados pelo servidor das janelas e pelo ajudante
	appData = data
	getSettings = currentCfg
	applySettings = func(c Config) error {
		if err := saveSettings(cfgPath, c); err != nil {
			return err
		}
		pPostMessageW.Call(hwnd, WM_RELOAD, 0, 0)
		return nil
	}
	startupGet = startupEnabled
	startupSet = func(on bool) { setStartup(on, exePath) }
	hotkeyAvailable = testHotkey
	hideOverlay = func() { pPostMessageW.Call(hwnd, WM_HIDE_OVERLAY, 0, 0) }
	showOverlay = func() { pPostMessageW.Call(hwnd, WM_SHOW_OVERLAY, 0, 0) }
	openSettingsWindow = func(hash string) { openSettings(hash) }
	goMini = func() { pPostMessageW.Call(hwnd, WM_GO_MINI, 0, 0) }
	refreshNow = func() { go doRefresh() }
	isBusy = func() bool { return busy }
	openURL = func(u string) { shellOpen(u, "") }
	openRobotsWindow = func() {
		if !focusPage(pageTitle("robots", currentCfg().Theme)) {
			openPage("robots", "", 1320, 860)
		}
	}

	// timer, avisos e notificações
	onTimerDone = func(label string) {
		t := "Your timer finished."
		if label != "" {
			t = label + " — time's up!"
		}
		showPopup(Alert{Key: "timer", Icon: "⏱", Title: "Time's up!", Text: t + " Click to stop."})
		playAlarm()
		if currentCfg().Mode == "mini" {
			pPostMessageW.Call(hwnd, WM_SHOW_JARVIS, 0, 0)
		}
	}
	onTimerClear = stopAlarm
	notify = func(a Alert) { showPopup(a) }
	startTimerLoop()
	runAlerts(data, currentCfg)
	updateAssetName = "Jarvis.exe"
	applyUpdate = func(file string) error { return exec.Command(file).Start() }
	startUpdateLoop()

	srv, err = startServer()
	if err != nil {
		messageBox("Jarvis", "Couldn't start the settings window: "+err.Error())
	}

	addTray()
	registerHotkey()

	go func() {
		doRefresh()
		t := time.NewTicker(15 * time.Minute)
		for range t.C {
			doRefresh()
		}
	}()

	if currentCfg().Mode == "mini" {
		showBox()
	} else {
		openHome()
	}
	if justUpdated {
		trayBalloon("Jarvis updated", "You're now on Jarvis "+appVersion+". Your settings were kept.")
	}
	if firstRun {
		trayBalloon("Jarvis is running", "Press "+cfg.Hotkey+" anytime. Drag the box to move it. Settings: right-click my icon near the clock.")
	}
	if !hotkeyOK {
		messageBox("Jarvis", "The shortcut "+cfg.Hotkey+" is already used by another program.\n\nOpen Jarvis Settings (right-click the Jarvis icon near the clock) and pick another one, like Ctrl+Alt+J.")
	}

	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(ptr(&msg), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(ptr(&msg))
		pDispatchMessageW.Call(ptr(&msg))
	}
	removeTray()
}

func doRefresh() {
	busy = true
	pInvalidateRect.Call(hwnd, 0, 0)
	refreshData(data, currentCfg())
	busy = false
	pInvalidateRect.Call(hwnd, 0, 0)
}

// ---------------- Janelas de Settings / Chat (Microsoft Edge em modo app) ----------------

func edgePath() string {
	for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		if base == "" {
			continue
		}
		p := filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func openPage(page, hash string, w, h int) {
	if srv == nil {
		return
	}
	url := srv.URL(page) + hash
	if edge := edgePath(); edge != "" {
		s := dpiScale()
		args := []string{"--app=" + url, "--user-data-dir=" + filepath.Join(appDir, "window"),
			"--no-first-run", "--no-default-browser-check", "--disable-features=Translate",
			"--window-size=" + itoa(int(float64(w)*s)) + "," + itoa(int(float64(h)*s))}
		if page == "home" || page == "robots" {
			var wa RECT
			pSystemParametersInfoW.Call(SPI_GETWORKAREA, 0, ptr(&wa), 0)
			ww, wh := int(float64(w)*s), int(float64(h)*s)
			aw, ah := int(wa.Right-wa.Left), int(wa.Bottom-wa.Top)
			if ww > aw {
				ww = aw
			}
			if wh > ah {
				wh = ah
			}
			args[len(args)-1] = "--window-size=" + itoa(ww) + "," + itoa(wh)
			args = append(args, "--window-position="+itoa(int(wa.Left)+(aw-ww)/2)+","+itoa(int(wa.Top)+(ah-wh)/2))
		}
		if page == "chat" {
			var wa RECT
			pSystemParametersInfoW.Call(SPI_GETWORKAREA, 0, ptr(&wa), 0)
			x := int(wa.Right) - int(float64(w+baseW+40)*s)
			if x < 0 {
				x = 0
			}
			args = append(args, "--window-position="+itoa(x)+","+itoa(int(wa.Top)+int(14*s)))
		}
		cmd := exec.Command(edge, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if cmd.Start() == nil {
			go cmd.Wait()
			return
		}
	}
	shellOpen(url, "") // sem Edge: abre no navegador padrão
}

func itoa(n int) string { return strconv.Itoa(n) }

var (
	pFindWindowW          = user32.NewProc("FindWindowW")
	pIsIconic             = user32.NewProc("IsIconic")
	pIsWindow             = user32.NewProc("IsWindow")
	pGetForegroundWindow  = user32.NewProc("GetForegroundWindow")
	pGetWindowThreadPID   = user32.NewProc("GetWindowThreadProcessId")
	pOpenProcess          = kernel32.NewProc("OpenProcess")
	pTerminateProcess     = kernel32.NewProc("TerminateProcess")
	pCloseHandle          = kernel32.NewProc("CloseHandle")
)

// findPage acha a janela do Edge de uma página do Jarvis pelo título.
func findPage(title string) uintptr {
	h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("Chrome_WidgetWin_1"))), uintptr(unsafe.Pointer(u16(title))))
	return h
}

func focusPage(title string) bool {
	h := findPage(title)
	if h == 0 {
		return false
	}
	if r, _, _ := pIsIconic.Call(h); r != 0 {
		pShowWindow.Call(h, 9) // SW_RESTORE
	}
	pSetForegroundWindow.Call(h)
	return true
}

func openSettings(section string) {
	if !focusPage(pageTitle("settings", currentCfg().Theme)) {
		openPage("settings", section, 900, 860)
	}
}
func openChat() {
	if !focusPage(pageTitle("chat", currentCfg().Theme)) {
		openPage("chat", "", 480, 720)
	}
}
func openHome() {
	if !focusPage(pageTitle("home", currentCfg().Theme)) {
		openPage("home", "", 1220, 820)
	}
}

func setMode(m string) {
	cfgMu.Lock()
	cfg.Mode = m
	c := cfg
	cfgMu.Unlock()
	_ = saveSettings(cfgPath, c)
}

// toWindow: do quadrado para a janela normal
func toWindow() {
	setMode("window")
	hideBox()
	openHome()
}

// toMini: da janela normal para o quadrado
func toMini() {
	setMode("mini")
	showBox()
}

// atalho / clique no ícone
func showOrToggle() {
	if currentCfg().Mode == "mini" {
		toggleBox()
		return
	}
	h := findPage(pageTitle("home", currentCfg().Theme))
	if h != 0 {
		if fg, _, _ := pGetForegroundWindow.Call(); fg == h {
			pShowWindow.Call(h, 6) // SW_MINIMIZE
			return
		}
	}
	openHome()
}

// ---------------- Instalar / atualizar ----------------

func installPath() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Jarvis", "Jarvis.exe")
}

// closeOldJarvis fecha qualquer Jarvis aberto (versões antigas são encerradas à força).
func closeOldJarvis() {
	h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("JarvisEscolarWindow"))), 0)
	if h == 0 {
		return
	}
	pPostMessageW.Call(h, WM_QUIT_JARVIS, 0, 0)
	for i := 0; i < 25; i++ {
		time.Sleep(100 * time.Millisecond)
		if r, _, _ := pIsWindow.Call(h); r == 0 {
			time.Sleep(300 * time.Millisecond)
			return
		}
	}
	// versão antiga que não entende a mensagem: tira o ícone e encerra
	var n NOTIFYICONDATAW
	n.CbSize = uint32(unsafe.Sizeof(n))
	n.HWnd, n.UID = h, 1
	pShellNotifyIconW.Call(NIM_DELETE, ptr(&n))
	var pid uint32
	pGetWindowThreadPID.Call(h, ptr(&pid))
	if pid != 0 {
		if ph, _, _ := pOpenProcess.Call(0x0001|0x00100000, 0, uintptr(pid)); ph != 0 { // TERMINATE | SYNCHRONIZE
			pTerminateProcess.Call(ph, 0)
			pCloseHandle.Call(ph)
		}
	}
	time.Sleep(600 * time.Millisecond)
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return err
	}
	var last error
	for i := 0; i < 20; i++ {
		_ = os.Remove(dst)
		if last = os.Rename(tmp, dst); last == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return last
}

// installOrUpdate: se este exe não é o instalado, fecha o antigo, copia-se para o lugar dele e abre a cópia.
func installOrUpdate() bool {
	dst := installPath()
	if os.Getenv("LOCALAPPDATA") == "" || strings.EqualFold(filepath.Clean(exePath), filepath.Clean(dst)) {
		return false
	}
	verFile := filepath.Join(filepath.Dir(dst), "version.txt")
	if b, err := os.ReadFile(verFile); err == nil && newerVersion(strings.TrimSpace(string(b)), appVersion) {
		if _, err := os.Stat(dst); err == nil {
			// já existe uma versão mais nova instalada: só abre ela
			if h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("JarvisEscolarWindow"))), 0); h != 0 {
				pPostMessageW.Call(h, WM_SHOW_JARVIS, 0, 0)
			} else {
				_ = exec.Command(dst).Start()
			}
			return true
		}
	}
	closeOldJarvis()
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	if err := copyFile(exePath, dst); err != nil {
		return false // não deu para instalar: roda daqui mesmo
	}
	_ = os.WriteFile(verFile, []byte(appVersion), 0o644)
	settingsExist := false
	if _, err := os.Stat(filepath.Join(os.Getenv("APPDATA"), "Jarvis", "settings.json")); err == nil {
		settingsExist = true
	}
	if startupEnabled() || !settingsExist {
		setStartup(true, dst)
	}
	cmd := exec.Command(dst, "--updated")
	if cmd.Start() != nil {
		return false
	}
	return true
}

// ---------------- Atalho ----------------

func registerHotkey() {
	pUnregisterHotKey.Call(hwnd, hotkeyID)
	c := currentCfg()
	mods, vk, ok := parseHotkey(c.Hotkey)
	if !ok {
		mods, vk = MOD_CONTROL|MOD_SHIFT, 'L'
	}
	r, _, _ := pRegisterHotKey.Call(hwnd, hotkeyID, mods|MOD_NOREPEAT, vk)
	hotkeyOK = r != 0
}

// testHotkey verifica se um atalho está livre (registra e desregistra numa thread própria).
func testHotkey(s string) bool {
	mods, vk, ok := parseHotkey(s)
	if !ok {
		return false
	}
	res := make(chan bool)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		r, _, _ := pRegisterHotKey.Call(0, 0x7777, mods|MOD_NOREPEAT, vk)
		if r != 0 {
			pUnregisterHotKey.Call(0, 0x7777)
		}
		res <- r != 0
	}()
	return <-res
}

// ---------------- Tamanho e posição ----------------

func dpiScale() float64 {
	if pGetDpiForWindow.Find() == nil && hwnd != 0 {
		if d, _, _ := pGetDpiForWindow.Call(hwnd); d != 0 {
			return float64(d) / 96
		}
	}
	hdc, _, _ := pGetDC.Call(0)
	d, _, _ := pGetDeviceCaps.Call(hdc, 90)
	pReleaseDC.Call(0, hdc)
	if d == 0 {
		return 1
	}
	return float64(d) / 96
}

func uiScale() float64 { return dpiScale() * currentCfg().Zoom }

var (
	pGetWindowRect = user32.NewProc("GetWindowRect")
	pGetSysMetrics = user32.NewProc("GetSystemMetrics")
	pGetKeyState   = user32.NewProc("GetKeyState")
)

func placeWindow(height int32) {
	s := uiScale()
	w := int32(baseW * s)
	c := currentCfg()
	var x, y int32
	if c.HasPos {
		x, y = c.WinX, c.WinY
		// mantém dentro da área de todas as telas
		vx, _, _ := pGetSysMetrics.Call(76)
		vy, _, _ := pGetSysMetrics.Call(77)
		vw, _, _ := pGetSysMetrics.Call(78)
		vh, _, _ := pGetSysMetrics.Call(79)
		minX, minY := int32(vx), int32(vy)
		maxX, maxY := int32(vx)+int32(vw)-w/3, int32(vy)+int32(vh)-40
		x = max(minX, min(x, maxX))
		y = max(minY, min(y, maxY))
	} else {
		var wa RECT
		pSystemParametersInfoW.Call(SPI_GETWORKAREA, 0, ptr(&wa), 0)
		m := int32(14 * dpiScale())
		x, y = wa.Right-w-m, wa.Top+m
	}
	pSetWindowPos.Call(hwnd, ^uintptr(0) /*HWND_TOPMOST*/, uintptr(x), uintptr(y), uintptr(w), uintptr(height), SWP_NOACTIVATE)
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(height+1), uintptr(int32(10*s)), uintptr(int32(10*s)))
	pSetWindowRgn.Call(hwnd, rgn, 1)
	curHeight = height
}

func savePosition() {
	var r RECT
	pGetWindowRect.Call(hwnd, ptr(&r))
	cfgMu.Lock()
	cfg.HasPos, cfg.WinX, cfg.WinY = true, r.Left, r.Top
	c := cfg
	cfgMu.Unlock()
	_ = saveSettings(cfgPath, c)
}

func setZoom(delta float64) {
	cfgMu.Lock()
	z := cfg.Zoom + delta
	if z < 0.7 {
		z = 0.7
	}
	if z > 1.8 {
		z = 1.8
	}
	cfg.Zoom = float64(int(z*100+0.5)) / 100
	c := cfg
	cfgMu.Unlock()
	_ = saveSettings(cfgPath, c)
	placeWindow(curHeight)
	pInvalidateRect.Call(hwnd, 0, 0)
}

// ---------------- Mostrar / esconder ----------------

func isVisible() bool { r, _, _ := pIsWindowVisible.Call(hwnd); return r != 0 }

func showBox() {
	if time.Since(data.UpdatedAt) > 5*time.Minute && !busy {
		go doRefresh()
	}
	if curHeight == 0 {
		curHeight = int32(420 * uiScale())
	}
	placeWindow(curHeight)
	pShowWindow.Call(hwnd, SW_SHOW)
	pSetForegroundWindow.Call(hwnd)
	pSetTimer.Call(hwnd, timerClock, 1000, 0)
	pInvalidateRect.Call(hwnd, 0, 0)
}

func hideBox() {
	pKillTimer.Call(hwnd, timerClock)
	pShowWindow.Call(hwnd, SW_HIDE)
}

func toggleBox() {
	if isVisible() {
		hideBox()
	} else {
		showBox()
	}
}

// ---------------- Bandeja ----------------

func trayData() NOTIFYICONDATAW {
	var n NOTIFYICONDATAW
	n.CbSize = uint32(unsafe.Sizeof(n))
	n.HWnd = hwnd
	n.UID = 1
	return n
}

func addTray() {
	n := trayData()
	n.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	n.UCallbackMessage = WM_TRAY
	n.HIcon = appIcon
	copyUTF16(n.SzTip[:], "Jarvis — "+currentCfg().Hotkey)
	pShellNotifyIconW.Call(NIM_ADD, ptr(&n))
}

func removeTray() { n := trayData(); pShellNotifyIconW.Call(NIM_DELETE, ptr(&n)) }

func trayBalloon(title, text string) {
	n := trayData()
	n.UFlags = NIF_INFO
	copyUTF16(n.SzInfoTitle[:], title)
	copyUTF16(n.SzInfo[:], text)
	n.DwInfoFlags = 1
	pShellNotifyIconW.Call(NIM_MODIFY, ptr(&n))
}

func trayMenu() {
	m, _, _ := pCreatePopupMenu.Call()
	add := func(flags, id uintptr, text string) {
		pAppendMenuW.Call(m, flags, id, uintptr(unsafe.Pointer(u16(text))))
	}
	add(MF_STRING, cmdWindow, "Open Jarvis window")
	add(MF_STRING, cmdMini, "Mini square\t"+currentCfg().Hotkey)
	add(MF_STRING, cmdChat, "Ask Jarvis…")
	add(MF_STRING, cmdRobots, "Robot Room…")
	add(MF_STRING, cmdRefresh, "Refresh now")
	add(MF_STRING, cmdSettings, "Settings…")
	st := uintptr(MF_STRING)
	if startupEnabled() {
		st |= MF_CHECKED
	}
	add(st, cmdStartup, "Start with Windows")
	add(MF_SEPARATOR, 0, "")
	add(MF_STRING, cmdExit, "Exit Jarvis")
	var pt POINT
	pGetCursorPos.Call(ptr(&pt))
	pSetForegroundWindow.Call(hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(m, TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	pPostMessageW.Call(hwnd, WM_NULL, 0, 0)
	pDestroyMenu.Call(m)
	switch cmd {
	case cmdWindow:
		toWindow()
	case cmdMini:
		toMini()
	case cmdChat:
		openChat()
	case cmdRobots:
		openRobotsWindow()
	case cmdRefresh:
		go doRefresh()
	case cmdSettings:
		openSettings("")
	case cmdStartup:
		setStartup(!startupEnabled(), exePath)
	case cmdExit:
		removeTray()
		pPostQuitMessage.Call(0)
	}
}

// ---------------- Mensagens ----------------

func hitAtXY(x, y int32) *hit {
	for i := range hits {
		h := &hits[i]
		if x >= h.r.Left && x < h.r.Right && y >= h.r.Top && y < h.r.Bottom {
			return h
		}
	}
	return nil
}

func hitAt(lp uintptr) *hit {
	return hitAtXY(int32(int16(lp&0xFFFF)), int32(int16((lp>>16)&0xFFFF)))
}

func doHit(ht *hit) {
	switch ht.action {
	case "url":
		shellOpen(ht.url, "")
	case "close":
		hideBox()
	case "settings":
		openSettings(ht.url)
	case "refresh":
		go doRefresh()
	case "chat":
		openChat()
	case "window":
		toWindow()
	case "timerstop":
		timer.Cancel()
	case "zoomin":
		setZoom(0.1)
	case "zoomout":
		setZoom(-0.1)
	}
}

func wndProc(h, msg, wp, lp uintptr) uintptr {
	switch msg {
	case WM_HOTKEY:
		showOrToggle()
		return 0
	case WM_POPUP_NEW:
		createPopups()
		return 0
	case WM_QUIT_JARVIS:
		removeTray()
		pPostQuitMessage.Call(0)
		return 0
	case WM_SHOW_JARVIS:
		if currentCfg().Mode == "mini" {
			showBox()
		} else {
			openHome()
		}
		return 0
	case WM_GO_MINI:
		toMini()
		return 0
	case WM_TIMER:
		pInvalidateRect.Call(h, 0, 0)
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_PAINT:
		paint(h)
		return 0
	case WM_NCHITTEST:
		// arrastar a caixa por qualquer lugar que não seja um botão/link
		var pt POINT
		pt.X, pt.Y = int32(int16(lp&0xFFFF)), int32(int16((lp>>16)&0xFFFF))
		pScreenToClient.Call(h, ptr(&pt))
		if hitAtXY(pt.X, pt.Y) != nil {
			return HTCLIENT
		}
		return HTCAPTION
	case WM_NCLBUTTONDBLCLK:
		return 0 // sem maximizar
	case WM_EXITSIZEMOVE:
		savePosition()
		return 0
	case WM_MOUSEWHEEL:
		delta := int16((wp >> 16) & 0xFFFF)
		ctrl, _, _ := pGetKeyState.Call(0x11)
		if ctrl&0x8000 != 0 || wp&0x8 != 0 {
			if delta > 0 {
				setZoom(0.1)
			} else {
				setZoom(-0.1)
			}
		}
		return 0
	case WM_KEYDOWN:
		switch wp {
		case VK_ESCAPE:
			hideBox()
		case 'R':
			go doRefresh()
		case 'S':
			openSettings("")
		case 'A':
			openChat()
		case 'W':
			toWindow()
		case 0xBB, 0x6B: // +
			setZoom(0.1)
		case 0xBD, 0x6D: // -
			setZoom(-0.1)
		}
		return 0
	case WM_SETCURSOR:
		var pt POINT
		pGetCursorPos.Call(ptr(&pt))
		pScreenToClient.Call(h, ptr(&pt))
		if hitAtXY(pt.X, pt.Y) != nil {
			c, _, _ := pLoadCursorW.Call(0, IDC_HAND)
			pSetCursor.Call(c)
			return 1
		}
		c, _, _ := pLoadCursorW.Call(0, 32646) // IDC_SIZEALL (mover)
		pSetCursor.Call(c)
		return 1
	case WM_LBUTTONUP:
		if ht := hitAt(lp); ht != nil {
			doHit(ht)
		}
		return 0
	case WM_TRAY:
		switch lp & 0xFFFF {
		case WM_LBUTTONUP:
			showOrToggle()
		case 0x0405: // NIN_BALLOONUSERCLICK: clicou na notificação
			if timer.View().State == "done" {
				timer.Cancel()
			}
			if currentCfg().Mode == "mini" {
				showBox()
			} else {
				openHome()
			}
		case WM_RBUTTONUP:
			trayMenu()
		}
		return 0
	case WM_RELOAD:
		old := currentCfg()
		n, _ := loadSettings(cfgPath)
		cfgMu.Lock()
		n.HasPos, n.WinX, n.WinY, n.Zoom = cfg.HasPos, cfg.WinX, cfg.WinY, cfg.Zoom
		cfg = n
		cfgMu.Unlock()
		if n.Hotkey != old.Hotkey {
			registerHotkey()
			if !hotkeyOK {
				messageBox("Jarvis", "The shortcut "+n.Hotkey+" is already used by another program. Try another one.")
			}
		}
		go doRefresh()
		return 0
	case WM_HIDE_OVERLAY:
		wasVisible = isVisible()
		hideBox()
		return 0
	case WM_SHOW_OVERLAY:
		if wasVisible {
			showBox()
		}
		return 0
	case WM_DPICHANGED:
		if isVisible() {
			placeWindow(curHeight)
		}
		return 0
	case WM_CLOSE:
		hideBox()
		return 0
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(h, msg, wp, lp)
	return r
}


// ---------------- Som do alarme ----------------

var (
	winmm       = syscall.NewLazyDLL("winmm.dll")
	pPlaySoundW = winmm.NewProc("PlaySoundW")
	alarmMu     sync.Mutex
	alarmGen    int
)

func playAlarm() {
	const SND_ASYNC, SND_LOOP, SND_ALIAS = 0x1, 0x8, 0x10000
	alarmMu.Lock()
	alarmGen++
	gen := alarmGen
	alarmMu.Unlock()
	r, _, _ := pPlaySoundW.Call(uintptr(unsafe.Pointer(u16("Notification.Looping.Alarm"))), 0, SND_ALIAS|SND_ASYNC|SND_LOOP)
	if r == 0 {
		pPlaySoundW.Call(uintptr(unsafe.Pointer(u16("SystemExclamation"))), 0, SND_ALIAS|SND_ASYNC)
	}
	go func() { // para sozinho depois de 1 minuto
		time.Sleep(time.Minute)
		alarmMu.Lock()
		same := gen == alarmGen
		alarmMu.Unlock()
		if same {
			stopAlarm()
		}
	}()
}

func stopAlarm() { pPlaySoundW.Call(0, 0, 0) }
