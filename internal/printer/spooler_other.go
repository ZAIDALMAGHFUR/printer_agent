//go:build !windows

package printer

import (
	"fmt"
	"log"
	"os/exec"
)

// PrintDocumentUniversal untuk Linux / macOS menggunakan CUPS lpr / lp
func PrintDocumentUniversal(printerName string, docName string, pdfFilePath string) error {
	log.Printf("[SPOOLER-CUPS] Mencetak via lp ke '%s': %s", printerName, pdfFilePath)
	cmd := exec.Command("lp", "-d", printerName, pdfFilePath)
	if err := cmd.Run(); err != nil {
		// Coba lpr
		cmd2 := exec.Command("lpr", "-P", printerName, pdfFilePath)
		if err2 := cmd2.Run(); err2 != nil {
			return fmt.Errorf("gagal mencetak via CUPS: %v / %v", err, err2)
		}
	}
	return nil
}

