package printer

import (
	"os"
	"strings"
	"testing"

	"smart-print-agent/internal/model"
)

func TestTemplateResepRacikan(t *testing.T) {
	data := map[string]interface{}{
		"instansi": "RSU ASHAVIN - FARMASI",
		"no_resep": "R-20261112-0043",
		"tanggal":  "12/11/2026 11:30",
		"no_rm":    "RM-091203",
		"pasien":   "An. Kevin Pratama (5 Th)",
		"dokter":   "dr. Nurul Aini, Sp.A",
		"racikan": []interface{}{
			map[string]interface{}{
				"kode":   "[R/ 1]",
				"nama":   "Racikan Batuk Pilek",
				"bentuk": "Pulveres (Puyer)",
				"jumlah": "10 Bungkus",
				"signa":  "3 x Sehari 1 Bungkus (Sesudah Makan)",
				"komposisi": []interface{}{
					map[string]interface{}{"obat": "Paracetamol 500mg", "dosis": "150 mg", "jumlah": "3 Tab"},
					map[string]interface{}{"obat": "CTM 4mg", "dosis": "1 mg", "jumlah": "2.5 Tab"},
					map[string]interface{}{"obat": "Dexamethasone 0.5mg", "dosis": "0.25 mg", "jumlah": "5 Tab"},
				},
			},
		},
		"catatan_apotek": "Masa simpan racikan (BUD) 30 hari.",
	}

	lines := ApplyTemplate("resep_racikan", data)
	if len(lines) == 0 {
		t.Fatalf("ApplyTemplate resep_racikan menghasilkan 0 baris")
	}

	// 1. Uji ESC/POS generator
	escBytes, err := BuildESCPOS(lines, true, false)
	if err != nil {
		t.Fatalf("BuildESCPOS untuk racikan gagal: %v", err)
	}
	if len(escBytes) == 0 {
		t.Fatalf("BuildESCPOS menghasilkan byte kosong")
	}

	// 2. Uji PDF generator
	tempDir, err := os.MkdirTemp("", "racikan_pdf_*")
	if err != nil {
		t.Fatalf("Gagal membuat temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	req := model.PrintRequest{
		RequestID:     "REQ-RACIKAN-01",
		PrinterTarget: "CetakFarmasi",
		DocumentType:  "resep_racikan",
		Konten:        lines,
	}

	pdfPath, err := GenerateReceiptPDF(req, tempDir)
	if err != nil {
		t.Fatalf("GenerateReceiptPDF gagal untuk racikan: %v", err)
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("Gagal membaca hasil PDF: %v", err)
	}

	if !strings.HasPrefix(string(pdfBytes), "%PDF-1.4") {
		t.Errorf("PDF header salah")
	}
}

func TestTemplateResepNonRacikan(t *testing.T) {
	data := map[string]interface{}{
		"instansi": "RSU ASHAVIN - FARMASI",
		"no_resep": "R-20261112-0042",
		"tanggal":  "12/11/2026 11:30",
		"no_rm":    "RM-082194",
		"pasien":   "Ny. Siti Rahayu (48 Th)",
		"dokter":   "dr. Bambang Sutrisno, Sp.PD",
		"obat": []interface{}{
			map[string]interface{}{"nama": "Amoxicillin 500mg", "signa": "3 x 1 tablet sesudah makan", "qty": "10 Tab", "catatan": "Habiskan!"},
			map[string]interface{}{"nama": "Paracetamol 500mg", "signa": "3 x 1 tablet bila demam", "qty": "10 Tab"},
		},
	}

	lines := ApplyTemplate("resep_non_racikan", data)
	if len(lines) == 0 {
		t.Fatalf("ApplyTemplate resep_non_racikan menghasilkan 0 baris")
	}

	escBytes, err := BuildESCPOS(lines, true, false)
	if err != nil {
		t.Fatalf("BuildESCPOS untuk obat jadi gagal: %v", err)
	}
	if len(escBytes) == 0 {
		t.Fatalf("BuildESCPOS menghasilkan byte kosong")
	}
}

func TestTemplateAntrian(t *testing.T) {
	data := map[string]interface{}{
		"instansi": "RSU ASHAVIN",
		"nomor":    "U-002",
		"kategori": "UMUM / PIHAK 3",
		"waktu":    "2026-11-12 11:26:53",
		"sisa":     2,
	}

	lines := ApplyTemplate("antrian", data)
	if len(lines) == 0 {
		t.Fatalf("ApplyTemplate antrian menghasilkan 0 baris")
	}

	// Cek apakah nomor antrian U-002 menggunakan font serif dan extra_large
	foundU002 := false
	for _, l := range lines {
		if l.Text == "U-002" {
			foundU002 = true
			if l.Size != "extra_large" {
				t.Errorf("Ukuran U-002 harus extra_large, didapat: %s", l.Size)
			}
			if l.Font != "serif" {
				t.Errorf("Font U-002 harus serif, didapat: %s", l.Font)
			}
		}
	}
	if !foundU002 {
		t.Errorf("U-002 tidak ditemukan dalam template antrian")
	}
}

