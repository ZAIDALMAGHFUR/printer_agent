//go:build windows

package printer

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

// PrintDocumentUniversal mencetak dokumen PDF ke printer Windows apa pun (Zebra, TSC, Epson, HP, Canon)
// menggunakan Windows Driver bawaan via headless Edge / PowerShell Spooler (pengganti modern Crystal Reports)
func PrintDocumentUniversal(printerName string, docName string, pdfFilePath string) error {
	if _, err := os.Stat(pdfFilePath); os.IsNotExist(err) {
		return fmt.Errorf("file dokumen tidak ditemukan: %s", pdfFilePath)
	}

	absPath, err := filepath.Abs(pdfFilePath)
	if err != nil {
		absPath = pdfFilePath
	}

	// 1. Coba menggunakan Microsoft Edge Headless (tersedia di 100% Windows 10 & 11)
	edgeCandidates := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}

	for _, edgePath := range edgeCandidates {
		if _, err := os.Stat(edgePath); err == nil {
			log.Printf("[SPOOLER] Mencetak via Headless Edge Spooler ke '%s': %s", printerName, absPath)
			cmd := exec.Command(edgePath, "--headless", fmt.Sprintf("--print-to-printer=%s", printerName), absPath)
			if err := cmd.Run(); err == nil {
				return nil
			}
			log.Printf("[SPOOLER] Edge print warning (%v), mencoba metode cadangan PowerShell...", err)
			break
		}
	}

	// 2. Metode Cadangan: PowerShell PrintTo verb
	log.Printf("[SPOOLER] Mencetak via PowerShell PrintTo ke '%s'", printerName)
	psCmd := fmt.Sprintf(`Start-Process -FilePath "%s" -Verb PrintTo -ArgumentList '"%s"' -PassThru | Out-Null`, absPath, printerName)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gagal mencetak ke spooler Windows '%s': %w", printerName, err)
	}

	return nil
}

