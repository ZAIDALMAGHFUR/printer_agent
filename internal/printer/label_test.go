package printer

import (
	"strings"
	"testing"
)

func TestTemplateLabelPasien(t *testing.T) {
	data := map[string]interface{}{
		"nama":      "NANI",
		"no_mr":     "00150515",
		"tgl_lahir": "29/01/1934",
		"umur":      "92 tahun",
		"alamat":    "552 Don Roads East Jazminland, MA 46857",
		"barcode":   "2611000008",
		"waktu":     "2026-11-02 12:15:40",
	}

	lines := ApplyTemplate("label_pasien", data)
	if len(lines) == 0 {
		t.Fatalf("ApplyTemplate label_pasien menghasilkan 0 baris")
	}

	tsplBytes, err := BuildTSPL(lines, "50x30mm", 1)
	if err != nil {
		t.Fatalf("BuildTSPL failed: %v", err)
	}

	tsplStr := string(tsplBytes)
	if !strings.Contains(tsplStr, "NANI") {
		t.Errorf("TSPL harus memuat nama NANI")
	}
	if !strings.Contains(tsplStr, "00150515") {
		t.Errorf("TSPL harus memuat no MR 00150515")
	}
	if !strings.Contains(tsplStr, "BARCODE") {
		t.Errorf("TSPL harus memiliki perintah BARCODE")
	}
	if !strings.Contains(tsplStr, "2611000008") {
		t.Errorf("TSPL harus memuat nomor barcode")
	}
}

