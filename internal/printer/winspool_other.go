//go:build !windows

package printer

import (
	"fmt"
	"os"
	"path/filepath"

	"smart-print-agent/internal/model"
)

// GetDefaultPrinter mengembalikan printer default di Linux
func GetDefaultPrinter() string {
	printers, _ := GetInstalledPrinters()
	if len(printers) > 0 {
		return printers[0].Name
	}
	return "Virtual PDF (Linux)"
}

// GetInstalledPrinters mendeteksi printer di Linux (/dev/usb/lp* atau fallback)
func GetInstalledPrinters() ([]model.WindowsPrinterInfo, error) {
	var list []model.WindowsPrinterInfo
	matches, _ := filepath.Glob("/dev/usb/lp*")
	for _, m := range matches {
		list = append(list, model.WindowsPrinterInfo{
			Name:   m,
			Driver: "Generic / Raw USB",
			Port:   m,
		})
	}
	if len(list) == 0 {
		list = append(list, model.WindowsPrinterInfo{
			Name:   "Virtual PDF (Linux)",
			Driver: "PDF",
			Port:   "FILE:",
		})
	}
	return list, nil
}

// PrintRawBytes mencetak data RAW ke perangkat printer di Linux
func PrintRawBytes(printerName string, docName string, data []byte) error {
	// Jika printer adalah device path (/dev/usb/lp0)
	if filepath.IsAbs(printerName) {
		f, err := os.OpenFile(printerName, os.O_WRONLY, 0666)
		if err != nil {
			return fmt.Errorf("gagal membuka printer device %s: %w", printerName, err)
		}
		defer f.Close()
		_, err = f.Write(data)
		return err
	}
	return fmt.Errorf("printer lokal %s tidak dapat diakses langsung", printerName)
}

