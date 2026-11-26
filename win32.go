//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pRegisterClassExW         = user32.NewProc("RegisterClassExW")
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDefWindowProcW           = user32.NewProc("DefWindowProcW")
	pGetMessageW              = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pPostMessageW             = user32.NewProc("PostMessageW")
	pShowWindow               = user32.NewProc("ShowWindow")
	pIsWindowVisible          = user32.NewProc("IsWindowVisible")
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	pRegisterHotKey           = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey         = user32.NewProc("UnregisterHotKey")
	pBeginPaint               = user32.NewProc("BeginPaint")
	pEndPaint                 = user32.NewProc("EndPaint")
	pInvalidateRect           = user32.NewProc("InvalidateRect")
	pFillRect                 = user32.NewProc("FillRect")
	pDrawTextW                = user32.NewProc("DrawTextW")
	pSetTimer                 = user32.NewProc("SetTimer")
	pKillTimer                = user32.NewProc("KillTimer")
	pLoadCursorW              = user32.NewProc("LoadCursorW")
	pSetCursor                = user32.NewProc("SetCursor")
	pLoadIconW                = user32.NewProc("LoadIconW")
	pSystemParametersInfoW    = user32.NewProc("SystemParametersInfoW")
	pGetDpiForWindow          = user32.NewProc("GetDpiForWindow")
	pSetWindowRgn             = user32.NewProc("SetWindowRgn")
	pSetLayeredWindowAttribs  = user32.NewProc("SetLayeredWindowAttributes")
	pCreatePopupMenu          = user32.NewProc("CreatePopupMenu")
	pAppendMenuW              = user32.NewProc("AppendMenuW")
	pTrackPopupMenu           = user32.NewProc("TrackPopupMenu")
	pDestroyMenu              = user32.NewProc("DestroyMenu")
	pGetCursorPos             = user32.NewProc("GetCursorPos")
	pScreenToClient           = user32.NewProc("ScreenToClient")
	pMessageBoxW              = user32.NewProc("MessageBoxW")
	pGetDC                    = user32.NewProc("GetDC")
	pReleaseDC                = user32.NewProc("ReleaseDC")
	pSetProcessDpiAwarenessCx = user32.NewProc("SetProcessDpiAwarenessContext")

	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pCreatePen              = gdi32.NewProc("CreatePen")
	pCreateFontW            = gdi32.NewProc("CreateFontW")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pRoundRect              = gdi32.NewProc("RoundRect")
	pGetStockObject         = gdi32.NewProc("GetStockObject")
	pCreateRoundRectRgn     = gdi32.NewProc("CreateRoundRectRgn")
	pGetDeviceCaps          = gdi32.NewProc("GetDeviceCaps")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pCreateMutexW     = kernel32.NewProc("CreateMutexW")

	pShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW    = shell32.NewProc("ShellExecuteW")

	pRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	pRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	pRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	pRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	pRegCloseKey      = advapi32.NewProc("RegCloseKey")
)

const (
	WS_POPUP          = 0x80000000
	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_LAYERED     = 0x00080000
	WM_DESTROY        = 0x0002
	WM_PAINT          = 0x000F
	WM_CLOSE          = 0x0010
	WM_ERASEBKGND     = 0x0014
	WM_SETCURSOR      = 0x0020
	WM_KEYDOWN        = 0x0100
	WM_COMMAND        = 0x0111
	WM_TIMER          = 0x0113
	WM_MOUSEMOVE      = 0x0200
	WM_LBUTTONDOWN    = 0x0201
	WM_LBUTTONUP      = 0x0202
	WM_RBUTTONUP      = 0x0205
	WM_HOTKEY         = 0x0312
	WM_DPICHANGED     = 0x02E0
	WM_APP            = 0x8000
	WM_TRAY           = WM_APP + 1
	WM_DATA           = WM_APP + 2
	WM_NULL           = 0
	SW_HIDE           = 0
	SW_SHOWNOACTIVATE = 4
	SW_SHOW           = 5
	SWP_NOZORDER      = 0x0004
	SWP_NOACTIVATE    = 0x0010
	SPI_GETWORKAREA   = 0x0030
	MOD_SHIFT         = 0x0004
	MOD_CONTROL       = 0x0002
	MOD_ALT           = 0x0001
	MOD_WIN           = 0x0008
	MOD_NOREPEAT      = 0x4000
	DT_RIGHT          = 0x0002
	DT_CENTER         = 0x0001
	DT_VCENTER        = 0x0004
	DT_WORDBREAK      = 0x0010
	DT_SINGLELINE     = 0x0020
	DT_CALCRECT       = 0x0400
	DT_NOPREFIX       = 0x0800
	DT_END_ELLIPSIS   = 0x8000
	SRCCOPY           = 0x00CC0020
	TRANSPARENT       = 1
	NULL_BRUSH        = 5
	LWA_ALPHA         = 0x2
	MF_STRING         = 0x0000
	MF_CHECKED        = 0x0008
	MF_SEPARATOR      = 0x0800
	TPM_RETURNCMD     = 0x0100
	TPM_RIGHTBUTTON   = 0x0002
	NIM_ADD           = 0
	NIM_MODIFY        = 1
	NIM_DELETE        = 2
	NIF_MESSAGE       = 0x1
	NIF_ICON          = 0x2
	NIF_TIP           = 0x4
	NIF_INFO          = 0x10
	IDC_ARROW         = 32512
	IDC_HAND          = 32649
	IDI_APPLICATION   = 32512
	VK_ESCAPE         = 0x1B
	HKEY_CURRENT_USER = 0x80000001
	KEY_ALL_ACCESS    = 0xF003F
	REG_SZ            = 1
)

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         GUID
	HBalloonIcon     uintptr
}

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func ptr[T any](v *T) uintptr { return uintptr(unsafe.Pointer(v)) }

func rgb(r, g, b uint8) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

func copyUTF16(dst []uint16, s string) {
	src, _ := syscall.UTF16FromString(s)
	if len(src) > len(dst) {
		src = src[:len(dst)]
		src[len(src)-1] = 0
	}
	copy(dst, src)
}

func messageBox(title, text string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16(title))), 0x40)
}

func shellOpen(target string, args string) {
	var a uintptr
	if args != "" {
		a = uintptr(unsafe.Pointer(u16(args)))
	}
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16(target))), a, 0, SW_SHOW)
}

// ---- Registro: iniciar com o Windows ----
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func startupEnabled() bool {
	var h uintptr
	if r, _, _ := pRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(runKey))), 0, 0x20019, ptr(&h)); r != 0 {
		return false
	}
	defer pRegCloseKey.Call(h)
	r, _, _ := pRegQueryValueExW.Call(h, uintptr(unsafe.Pointer(u16("Jarvis"))), 0, 0, 0, 0)
	return r == 0
}

func setStartup(on bool, exe string) {
	var h uintptr
	if r, _, _ := pRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(runKey))), 0, KEY_ALL_ACCESS, ptr(&h)); r != 0 {
		return
	}
	defer pRegCloseKey.Call(h)
	if on {
		v, _ := syscall.UTF16FromString(`"` + exe + `"`)
		pRegSetValueExW.Call(h, uintptr(unsafe.Pointer(u16("Jarvis"))), 0, REG_SZ, uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2))
	} else {
		pRegDeleteValueW.Call(h, uintptr(unsafe.Pointer(u16("Jarvis"))))
	}
}
