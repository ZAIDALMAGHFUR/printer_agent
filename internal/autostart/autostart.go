//go:build windows

package autostart

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	advapi32           = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW   = advapi32.NewProc("RegOpenKeyExW")
	procRegSetValueExW  = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW = advapi32.NewProc("RegDeleteValueW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey     = advapi32.NewProc("RegCloseKey")
)

const (
	HKEY_CURRENT_USER = 0x80000001
	KEY_ALL_ACCESS    = 0xF003F
	KEY_READ          = 0x20019
	KEY_SET_VALUE     = 0x0002
	REG_SZ            = 1

	runSubKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	appName   = "SmartPrintAgent"
)

// Enable mendaftarkan aplikasi ke Registry Windows agar otomatis berjalan saat startup
func Enable() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, _ = filepath.Abs(exePath)
	quotedPath := `"` + exePath + `"`

	var hKey syscall.Handle
	subKeyPtr, _ := syscall.UTF16PtrFromString(runSubKey)

	r1, _, err := procRegOpenKeyExW.Call(
		uintptr(HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(subKeyPtr)),
		0,
		uintptr(KEY_SET_VALUE),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if r1 != 0 {
		return err
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	namePtr, _ := syscall.UTF16PtrFromString(appName)
	valPtr, _ := syscall.UTF16FromString(quotedPath)

	r1, _, err = procRegSetValueExW.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		uintptr(REG_SZ),
		uintptr(unsafe.Pointer(&valPtr[0])),
		uintptr(len(valPtr)*2),
	)
	if r1 != 0 {
		return err
	}

	return nil
}

// Disable menghapus pendaftaran autostart dari Registry Windows
func Disable() error {
	var hKey syscall.Handle
	subKeyPtr, _ := syscall.UTF16PtrFromString(runSubKey)

	r1, _, err := procRegOpenKeyExW.Call(
		uintptr(HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(subKeyPtr)),
		0,
		uintptr(KEY_SET_VALUE),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if r1 != 0 {
		return err
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	namePtr, _ := syscall.UTF16PtrFromString(appName)
	r1, _, err = procRegDeleteValueW.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(namePtr)),
	)
	if r1 != 0 {
		return err
	}

	return nil
}

// IsEnabled mengecek apakah aplikasi sudah terdaftar di autostart Registry
func IsEnabled() bool {
	var hKey syscall.Handle
	subKeyPtr, _ := syscall.UTF16PtrFromString(runSubKey)

	r1, _, _ := procRegOpenKeyExW.Call(
		uintptr(HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(subKeyPtr)),
		0,
		uintptr(KEY_READ),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if r1 != 0 {
		return false
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	namePtr, _ := syscall.UTF16PtrFromString(appName)
	r1, _, _ = procRegQueryValueExW.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		0,
		0,
		0,
	)
	return r1 == 0
}
