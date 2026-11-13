//go:build windows

package printer

import (
	"fmt"
	"syscall"
	"unsafe"

	"smart-print-agent/internal/model"
)

var (
	winspool                = syscall.NewLazyDLL("winspool.drv")
	procOpenPrinterW        = winspool.NewProc("OpenPrinterW")
	procClosePrinter        = winspool.NewProc("ClosePrinter")
	procStartDocPrinterW    = winspool.NewProc("StartDocPrinterW")
	procEndDocPrinter       = winspool.NewProc("EndDocPrinter")
	procStartPagePrinter    = winspool.NewProc("StartPagePrinter")
	procEndPagePrinter      = winspool.NewProc("EndPagePrinter")
	procWritePrinter        = winspool.NewProc("WritePrinter")
	procEnumPrintersW       = winspool.NewProc("EnumPrintersW")
	procGetDefaultPrinterW  = winspool.NewProc("GetDefaultPrinterW")
)

type DOC_INFO_1 struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
}

type PRINTER_INFO_4 struct {
	pPrinterName *uint16
	pServerName  *uint16
	Attributes   uint32
}

const (
	PRINTER_ENUM_LOCAL       = 0x00000002
	PRINTER_ENUM_CONNECTIONS = 0x00000004
)

// PrintRawBytes mengirimkan raw bytes (ESC/POS atau TSPL) langsung ke Windows Print Spooler
func PrintRawBytes(printerName string, docName string, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("data cetak kosong")
	}

	pPrinterName, err := syscall.UTF16PtrFromString(printerName)
	if err != nil {
		return fmt.Errorf("nama printer tidak valid: %w", err)
	}

	var hPrinter syscall.Handle
	r1, _, err := procOpenPrinterW.Call(
		uintptr(unsafe.Pointer(pPrinterName)),
		uintptr(unsafe.Pointer(&hPrinter)),
		0,
	)
	if r1 == 0 {
		return fmt.Errorf("gagal membuka printer '%s': %v (pastikan printer terinstall)", printerName, err)
	}
	defer procClosePrinter.Call(uintptr(hPrinter))

	pDocName, _ := syscall.UTF16PtrFromString(docName)
	pDataType, _ := syscall.UTF16PtrFromString("RAW")

	docInfo := DOC_INFO_1{
		pDocName:    pDocName,
		pOutputFile: nil,
		pDatatype:   pDataType,
	}

	r1, _, err = procStartDocPrinterW.Call(
		uintptr(hPrinter),
		1,
		uintptr(unsafe.Pointer(&docInfo)),
	)
	if r1 == 0 {
		return fmt.Errorf("gagal memulai doc printer: %v", err)
	}
	defer procEndDocPrinter.Call(uintptr(hPrinter))

	r1, _, err = procStartPagePrinter.Call(uintptr(hPrinter))
	if r1 == 0 {
		return fmt.Errorf("gagal memulai page printer: %v", err)
	}
	defer procEndPagePrinter.Call(uintptr(hPrinter))

	var written uint32
	r1, _, err = procWritePrinter.Call(
		uintptr(hPrinter),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)),
	)
	if r1 == 0 || written != uint32(len(data)) {
		return fmt.Errorf("gagal menulis data ke printer: %v", err)
	}

	return nil
}

// GetInstalledPrinters mendeteksi seluruh printer Windows yang terpasang di komputer ini
func GetInstalledPrinters() ([]model.WindowsPrinterInfo, error) {
	flags := uint32(PRINTER_ENUM_LOCAL | PRINTER_ENUM_CONNECTIONS)
	var bytesNeeded, count uint32

	// Panggil pertama untuk mendapatkan ukuran buffer
	procEnumPrintersW.Call(
		uintptr(flags),
		0,
		4, // Level 4 (PRINTER_INFO_4)
		0,
		0,
		uintptr(unsafe.Pointer(&bytesNeeded)),
		uintptr(unsafe.Pointer(&count)),
	)

	if bytesNeeded == 0 {
		return []model.WindowsPrinterInfo{}, nil
	}

	buf := make([]byte, bytesNeeded)
	r1, _, err := procEnumPrintersW.Call(
		uintptr(flags),
		0,
		4,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(bytesNeeded),
		uintptr(unsafe.Pointer(&bytesNeeded)),
		uintptr(unsafe.Pointer(&count)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("gagal membaca daftar printer: %v", err)
	}

	defaultPrinter := GetDefaultPrinter()

	var result []model.WindowsPrinterInfo
	infoSize := unsafe.Sizeof(PRINTER_INFO_4{})

	for i := uint32(0); i < count; i++ {
		info := (*PRINTER_INFO_4)(unsafe.Pointer(&buf[uintptr(i)*infoSize]))
		if info.pPrinterName != nil {
			name := syscall.UTF16ToString((*[512]uint16)(unsafe.Pointer(info.pPrinterName))[:])
			result = append(result, model.WindowsPrinterInfo{
				Name:      name,
				IsDefault: (name == defaultPrinter),
				Status:    "Ready",
			})
		}
	}

	return result, nil
}

// GetDefaultPrinter mengambil nama default printer Windows
func GetDefaultPrinter() string {
	var size uint32
	procGetDefaultPrinterW.Call(0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return ""
	}

	buf := make([]uint16, size)
	r1, _, _ := procGetDefaultPrinterW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

