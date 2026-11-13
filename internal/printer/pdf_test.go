package printer

import (
	"os"
	"strings"
	"testing"

	"smart-print-agent/internal/model"
)

func TestGenerateReceiptPDF(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pdf_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	req := model.PrintRequest{
		RequestID:     "REQ-ASHAVIN-001",
		PrinterTarget: "CetakAntrian",
		DocumentType:  "antrian",
		Konten: []model.PrintLine{
			{Text: "RSU ASHAVIN", Align: "center", Bold: true, Size: "large"},
			{Text: "Jln. Raya Jepara - Bangsri Km. 09, No. 9 Ds. Jambu, Kec.\nMlonggo, Kab. Jepara, Jepara\nTelp: 082221455308\nEmail: rsuashavin5pandawagrup@gmail\nWebsite:", Align: "center", Size: "small"},
			{Type: "divider"},
			{Text: "Tanggal : 2026-11-12 11:26:53", Align: "left", Bold: true},
			{Text: "\nNomor Antrian :\n", Align: "center"},
			{Text: "U-002", Align: "center", Bold: true, Size: "extra_large"},
			{Text: "UMUM / PIHAK 3\n", Align: "center", Bold: true, Size: "large"},
			{Text: "Silahkan menunggu nomor Anda dipanggil\nAntrian yang belum dipanggil 2 Orang", Align: "center", Size: "small"},
			{Type: "cut"},
		},
	}

	pdfPath, err := GenerateReceiptPDF(req, tempDir)
	if err != nil {
		t.Fatalf("GenerateReceiptPDF failed: %v", err)
	}

	if !strings.HasSuffix(pdfPath, ".pdf") {
		t.Errorf("File path harus berakhiran .pdf: %s", pdfPath)
	}

	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("Failed to read generated PDF: %v", err)
	}

	if !strings.HasPrefix(string(data), "%PDF-1.4") {
		t.Errorf("File header harus '%%PDF-1.4'")
	}
	if !strings.Contains(string(data), "%%EOF") {
		t.Errorf("File harus diakhiri '%%%%EOF'")
	}
}
