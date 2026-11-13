package printer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDynamicTemplateManager(t *testing.T) {
	tempDir := filepath.Join(os.TempDir(), "test_templates")
	defer os.RemoveAll(tempDir)

	tm := NewTemplateManager(tempDir)
	list := tm.ListTemplates()
	t.Logf("Found %d templates in temp dir", len(list))

	// Test with actual templates in ../../templates
	actualTM := NewTemplateManager("../../templates")

	// 1. Test Label Pasien (Gambar 3)
	tmpl, found := actualTM.GetTemplate("label_pasien")
	if !found {
		t.Fatalf("Template 'label_pasien' harusnya ditemukan di folder templates")
	}

	if tmpl.Paper.WidthMM != 50 || tmpl.Paper.HeightMM != 30 {
		t.Errorf("Dimensi kertas salah: expected 50x30, got %fx%f", tmpl.Paper.WidthMM, tmpl.Paper.HeightMM)
	}

	saveDir := filepath.Join(os.TempDir(), "test_pdf_out")
	defer os.RemoveAll(saveDir)

	pdfPath, err := actualTM.RenderToPDF(tmpl, tmpl.SampleData, saveDir, "TEST-LABEL")
	if err != nil {
		t.Fatalf("Gagal render label ke PDF: %v", err)
	}

	fi, err := os.Stat(pdfPath)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("File PDF label tidak valid: %v", err)
	}
	t.Logf("Berhasil render PDF label (Gambar 3): %s (%d bytes)", pdfPath, fi.Size())

	// 2. Test Antrian Kiosk (Gambar 2)
	antrianTmpl, found := actualTM.GetTemplate("antrian")
	if !found {
		t.Fatalf("Template 'antrian' harusnya ditemukan di folder templates")
	}

	antrianPDF, err := actualTM.RenderToPDF(antrianTmpl, antrianTmpl.SampleData, saveDir, "TEST-ANTRIAN")
	if err != nil {
		t.Fatalf("Gagal render antrian ke PDF: %v", err)
	}

	fi2, err := os.Stat(antrianPDF)
	if err != nil || fi2.Size() == 0 {
		t.Fatalf("File PDF antrian tidak valid: %v", err)
	}
	t.Logf("Berhasil render PDF antrian (Gambar 2): %s (%d bytes)", antrianPDF, fi2.Size())

	// 3. Test Bukti Pendaftaran Pasien / Tracer 80mm
	buktiTmpl, found := actualTM.GetTemplate("bukti_pendaftaran")
	if !found {
		t.Fatalf("Template 'bukti_pendaftaran' harusnya ditemukan di folder templates")
	}

	buktiPDF, err := actualTM.RenderToPDF(buktiTmpl, buktiTmpl.SampleData, saveDir, "TEST-BUKTI")
	if err != nil {
		t.Fatalf("Gagal render bukti_pendaftaran ke PDF: %v", err)
	}

	fi3, err := os.Stat(buktiPDF)
	if err != nil || fi3.Size() == 0 {
		t.Fatalf("File PDF bukti_pendaftaran tidak valid: %v", err)
	}
	t.Logf("Berhasil render PDF bukti_pendaftaran: %s (%d bytes)", buktiPDF, fi3.Size())
}
