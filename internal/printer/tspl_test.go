package printer

import (
	"strings"
	"testing"

	"smart-print-agent/internal/model"
)

func TestBuildTSPL_LabelFarmasi(t *testing.T) {
	lines := []model.PrintLine{
		{Text: "RSU ASHAVIN - FARMASI", Bold: true, Size: "medium"},
		{Text: "Tn. Ahmad Subarjo (123456)", Size: "normal"},
		{Text: "Amoxicillin 500mg", Size: "normal"},
		{Text: "3 x Sehari 1 Tablet", Bold: true},
		{Type: "barcode", Text: "OBT-202611-0091", Height: 45},
	}

	raw, err := BuildTSPL(lines, "50x30mm", 1)
	if err != nil {
		t.Fatalf("BuildTSPL failed: %v", err)
	}

	str := string(raw)

	// Verifikasi perintah dasar TSPL
	if !strings.Contains(str, "SIZE 50 mm, 30 mm") {
		t.Errorf("TSPL harus mengandung 'SIZE 50 mm, 30 mm', got: %s", str)
	}
	if !strings.Contains(str, "CLS") {
		t.Errorf("TSPL harus mengandung 'CLS'")
	}
	if !strings.Contains(str, "RSU ASHAVIN - FARMASI") {
		t.Errorf("TSPL harus mengandung nama RS")
	}
	if !strings.Contains(str, "BARCODE") || !strings.Contains(str, "OBT-202611-0091") {
		t.Errorf("TSPL harus mengandung perintah BARCODE")
	}
	if !strings.Contains(str, "PRINT 1, 1") {
		t.Errorf("TSPL harus diakhiri dengan 'PRINT 1, 1'")
	}
}

func TestBuildTSPL_TwoColumns(t *testing.T) {
	lines := []model.PrintLine{
		{Text: "RSU ASHAVIN", Bold: true, Size: "medium"},
		{Text: "TEST CETAK BARCODE", Size: "normal"},
		{Type: "divider"},
		{Type: "barcode", Text: "TEST-123456", Height: 35},
	}

	raw, err := BuildTSPL(lines, "75x20mm_2col", 1)
	if err != nil {
		t.Fatalf("BuildTSPL 2col failed: %v", err)
	}

	str := string(raw)

	if !strings.Contains(str, "SIZE 75 mm, 20 mm") {
		t.Errorf("TSPL harus mengandung 'SIZE 75 mm, 20 mm', got: %s", str)
	}
	// Periksa apakah perintah diduplikasi untuk kolom 1 dan kolom 2 (+300 dots offset)
	if !strings.Contains(str, "TEXT 16,") {
		t.Errorf("TSPL harus mengandung koordinat kolom 1: %s", str)
	}
	if !strings.Contains(str, "TEXT 316,") {
		t.Errorf("TSPL harus mengandung koordinat kolom 2 (+300 dots): %s", str)
	}
	if strings.Count(str, "TEST-123456") < 2 {
		t.Errorf("Barcode harus diduplikasi ke kedua kolom, got: %s", str)
	}
}

func TestBuildTSPLFromTemplate_TwoColumns(t *testing.T) {
	tm := GetTemplateManager()
	tmpl, ok := tm.GetTemplate("label_pasien_2kolom")
	if !ok {
		t.Fatalf("Template label_pasien_2kolom tidak ditemukan")
	}

	data := map[string]interface{}{
		"nama":      "BUDI SANTOSO",
		"no_mr":     "12345678",
		"tgl_lahir": "01/01/1980",
		"umur":      "46 Th",
		"jk":        "L",
		"barcode":   "2611009999",
		"waktu":     "2026-11-16 19:40",
	}

	raw, err := BuildTSPLFromTemplate(tmpl, data, 1)
	if err != nil {
		t.Fatalf("BuildTSPLFromTemplate failed: %v", err)
	}

	str := string(raw)

	if !strings.Contains(str, "SIZE 75.0 mm, 20.0 mm") {
		t.Errorf("TSPL template harus SIZE 75.0 mm, 20.0 mm, got: %s", str)
	}
	if !strings.Contains(str, "BUDI SANTOSO") {
		t.Errorf("TSPL harus mengandung nama pasien terinterpolasi, got: %s", str)
	}
	if strings.Count(str, "BUDI SANTOSO") < 2 {
		t.Errorf("Nama pasien harus tercetak di kolom kiri dan kanan (2-up), got: %s", str)
	}
	if strings.Count(str, "2611009999") < 2 {
		t.Errorf("Barcode harus tercetak di kolom kiri dan kanan, got: %s", str)
	}
}

