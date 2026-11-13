//go:build windows

package tray

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"smart-print-agent/internal/autostart"
)

//go:embed icon.ico
var embeddedTrayIcon []byte

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procLoadImageW          = user32.NewProc("LoadImageW")

	procShell_NotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW  = kernel32.NewProc("GetModuleHandleW")
)

const (
	WM_USER          = 0x0400
	WM_TRAYICON      = WM_USER + 1
	WM_LBUTTONUP     = 0x0202
	WM_LBUTTONDBLCLK = 0x0203
	WM_RBUTTONUP     = 0x0205
	WM_COMMAND       = 0x0111
	WM_DESTROY       = 0x0002

	NIM_ADD    = 0x00000000
	NIM_MODIFY = 0x00000001
	NIM_DELETE = 0x00000002

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004
	NIF_INFO    = 0x00000010

	NIIF_INFO = 0x00000001

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800
	MF_CHECKED   = 0x00000008
	MF_UNCHECKED = 0x00000000

	TPM_RIGHTBUTTON = 0x0002
	TPM_BOTTOMALIGN = 0x0020

	IMAGE_ICON     = 1
	LR_LOADFROMFILE = 0x00000010
	LR_DEFAULTSIZE  = 0x00000040

	IDI_APPLICATION = 32512

	ID_MENU_SETTINGS  = 1001
	ID_MENU_PDF_DIR   = 1002
	ID_MENU_AUTOSTART = 1003
	ID_MENU_EXIT      = 1004
)

type WNDCLASSEXW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type NOTIFYICONDATAW struct {
	cbSize            uint32
	_                 uint32 // padding
	hWnd              syscall.Handle
	uID               uint32
	uFlags            uint32
	uCallbackMessage  uint32
	_                 uint32 // padding
	hIcon             syscall.Handle
	szTip             [128]uint16
	dwState           uint32
	dwStateMask       uint32
	szInfo            [256]uint16
	uTimeoutOrVersion uint32
	szInfoTitle       [64]uint16
	dwInfoFlags       uint32
	guidItem          [16]byte
	hBalloonIcon      syscall.Handle
}

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	_       uint32 // padding
}

var (
	gHwnd       syscall.Handle
	gNid        NOTIFYICONDATAW
	gPort       int
	gPdfDir     string
	gHospital   string
	gClassName  = "SmartPrintAgentTrayWindow"
)

// StartTray menjalankan System Tray icon di latar belakang Windows
func StartTray(port int, pdfDir string, hospital string) {
	gPort = port
	gPdfDir = pdfDir
	gHospital = hospital

	// Jalankan di goroutine terpisah yang terkunci pada OS thread
	go runTrayMessageLoop()
}

func loadTrayIcon() syscall.Handle {
	// 1. Ekstrak icon bawaan ke folder temp Windows jika tersedia
	tempIconPath := filepath.Join(os.TempDir(), "smart_print_agent.ico")
	if len(embeddedTrayIcon) > 0 {
		_ = os.WriteFile(tempIconPath, embeddedTrayIcon, 0644)
	}

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	candidates := []string{
		tempIconPath,
		filepath.Join(exeDir, "icon.ico"),
		filepath.Join(exeDir, "assets", "icon.ico"),
		filepath.Join(".", "assets", "icon.ico"),
		filepath.Join(".", "bin", "icon.ico"),
		"icon.ico",
	}

	for _, path := range candidates {
		if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
			pathPtr, err := syscall.UTF16PtrFromString(path)
			if err == nil {
				hIcon, _, _ := procLoadImageW.Call(
					0,
					uintptr(unsafe.Pointer(pathPtr)),
					IMAGE_ICON,
					0,
					0,
					LR_LOADFROMFILE|LR_DEFAULTSIZE,
				)
				if hIcon != 0 {
					return syscall.Handle(hIcon)
				}
			}
		}
	}

	// Fallback ke icon default Windows jika file tidak ditemukan
	hIcon, _, _ := procLoadIconW.Call(0, uintptr(IDI_APPLICATION))
	return syscall.Handle(hIcon)
}

func runTrayMessageLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	classNamePtr, _ := syscall.UTF16PtrFromString(gClassName)

	hIcon := loadTrayIcon()

	wndClass := WNDCLASSEXW{
		cbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		style:         0,
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     syscall.Handle(hInst),
		hIcon:         hIcon,
		hIconSm:       hIcon,
		lpszClassName: classNamePtr,
	}

	r1, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wndClass)))
	if r1 == 0 {
		return
	}

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(classNamePtr)),
		uintptr(unsafe.Pointer(classNamePtr)),
		0, 0, 0, 0, 0,
		0, 0,
		hInst,
		0,
	)
	if hwnd == 0 {
		return
	}
	gHwnd = syscall.Handle(hwnd)

	// Inisialisasi System Tray Icon
	gNid = NOTIFYICONDATAW{
		cbSize:           uint32(unsafe.Sizeof(gNid)),
		hWnd:             gHwnd,
		uID:              1,
		uFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP | NIF_INFO,
		uCallbackMessage: WM_TRAYICON,
		hIcon:            syscall.Handle(hIcon),
		dwInfoFlags:      NIIF_INFO,
	}

	tip := fmt.Sprintf("%s - Smart Print Agent (Port %d)", gHospital, gPort)
	tipUTF16, _ := syscall.UTF16FromString(tip)
	copy(gNid.szTip[:], tipUTF16)

	infoTitle, _ := syscall.UTF16FromString(gHospital)
	copy(gNid.szInfoTitle[:], infoTitle)

	infoText, _ := syscall.UTF16FromString(fmt.Sprintf("Print Agent aktif di port %d. Klik icon untuk pengaturan.", gPort))
	copy(gNid.szInfo[:], infoText)

	procShell_NotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&gNid)))

	// Pesan loop Windows
	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	// Bersihkan icon saat aplikasi keluar
	procShell_NotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&gNid)))
}

func wndProc(hwnd syscall.Handle, msg uint32, wparam uintptr, lparam uintptr) uintptr {
	switch msg {
	case WM_TRAYICON:
		switch lparam {
		case WM_LBUTTONUP, WM_LBUTTONDBLCLK:
			// Klik Kiri: Buka Dashboard Setting di browser
			openDashboard()

		case WM_RBUTTONUP:
			// Klik Kanan: Tampilkan Popup Menu
			showContextMenu(hwnd)
		}
		return 0

	case WM_COMMAND:
		cmdID := int(wparam & 0xFFFF)
		switch cmdID {
		case ID_MENU_SETTINGS:
			openDashboard()

		case ID_MENU_PDF_DIR:
			openFolder(gPdfDir)

		case ID_MENU_AUTOSTART:
			if autostart.IsEnabled() {
				_ = autostart.Disable()
			} else {
				_ = autostart.Enable()
			}

		case ID_MENU_EXIT:
			procShell_NotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&gNid)))
			procPostQuitMessage.Call(0)
			os.Exit(0)
		}
		return 0

	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
	return r
}

func showContextMenu(hwnd syscall.Handle) {
	hMenu, _, _ := procCreatePopupMenu.Call()

	txtSettings, _ := syscall.UTF16PtrFromString("⚙️  Buka Pengaturan Printer...")
	txtPdfDir, _ := syscall.UTF16PtrFromString("📁  Buka Folder PDF...")
	txtAutoStart, _ := syscall.UTF16PtrFromString("🔄  Jalankan Otomatis saat Windows Boot")
	txtExit, _ := syscall.UTF16PtrFromString("❌  Keluar dari Print Agent")

	procAppendMenuW.Call(hMenu, MF_STRING, ID_MENU_SETTINGS, uintptr(unsafe.Pointer(txtSettings)))
	procAppendMenuW.Call(hMenu, MF_STRING, ID_MENU_PDF_DIR, uintptr(unsafe.Pointer(txtPdfDir)))
	procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

	autoStartFlags := uintptr(MF_STRING)
	if autostart.IsEnabled() {
		autoStartFlags |= MF_CHECKED
	} else {
		autoStartFlags |= MF_UNCHECKED
	}
	procAppendMenuW.Call(hMenu, autoStartFlags, ID_MENU_AUTOSTART, uintptr(unsafe.Pointer(txtAutoStart)))
	procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)
	procAppendMenuW.Call(hMenu, MF_STRING, ID_MENU_EXIT, uintptr(unsafe.Pointer(txtExit)))

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(hMenu, TPM_RIGHTBUTTON|TPM_BOTTOMALIGN, uintptr(pt.X), uintptr(pt.Y), 0, uintptr(hwnd), 0)
	procPostMessageW.Call(uintptr(hwnd), 0, 0, 0)
}

func openDashboard() {
	url := fmt.Sprintf("http://127.0.0.1:%d/settings", gPort)
	_ = exec.Command("cmd", "/c", "start", url).Start()
}

func openFolder(path string) {
	if path == "" {
		path = "C:\\SIMRS_PDF"
	}
	_ = exec.Command("explorer.exe", path).Start()
}
