package printer

import (
	"bytes"
	"testing"

	"smart-print-agent/internal/model"
)

func TestBuildESCPOS_AntrianAshavin(t *testing.T) {
	lines := []model.PrintLine{
		{Text: "RSU ASHAVIN", Align: "center", Bold: true, Size: "large"},
		{Text: "Jln. Raya Jepara - Bangsri Km. 09", Align: "center", Size: "small"},
		{Type: "divider"},
		{Text: "Tanggal : 2026-11-12 11:26:53", Align: "left", Bold: true},
		{Text: "\nNomor Antrian :\n", Align: "center"},
		{Text: "U-002", Align: "center", Bold: true, Size: "extra_large"},
		{Text: "UMUM / PIHAK 3\n", Align: "center", Bold: true, Size: "large"},
		{Text: "Silahkan menunggu nomor Anda dipanggil", Align: "center", Size: "small"},
		{Type: "cut"},
	}

	raw, err := BuildESCPOS(lines, true, false)
	if err != nil {
		t.Fatalf("BuildESCPOS failed: %v", err)
	}

	if len(raw) == 0 {
		t.Fatal("Hasil raw ESC/POS kosong")
	}

	// Verifikasi byte inisialisasi printer ESC @ (0x1B, 0x40)
	if !bytes.HasPrefix(raw, []byte{0x1B, 0x40}) {
		t.Errorf("Harus diawali dengan ESC @ (0x1B, 0x40)")
	}

	// Verifikasi teks RSU ASHAVIN ada di dalam raw bytes
	if !bytes.Contains(raw, []byte("RSU ASHAVIN")) {
		t.Errorf("Raw data harus mengandung 'RSU ASHAVIN'")
	}

	// Verifikasi teks nomor antrian U-002
	if !bytes.Contains(raw, []byte("U-002")) {
		t.Errorf("Raw data harus mengandung 'U-002'")
	}

	// Verifikasi perintah Auto-Cut GS V 0 (0x1D, 0x56, 0x00)
	if !bytes.Contains(raw, []byte{0x1D, 0x56, 0x00}) {
		t.Errorf("Raw data harus mengandung perintah Auto Cut GS V 0")
	}
}

