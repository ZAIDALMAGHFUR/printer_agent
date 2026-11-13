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

