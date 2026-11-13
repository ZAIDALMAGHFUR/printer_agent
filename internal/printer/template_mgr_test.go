package printer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDynamicTemplateManager(t *testing.T) {
	tempDir := filepath.Join(os.TempDir(), "test_templates")
	defer os.RemoveAll(tempDir)

	tm := GetTemplateManager(tempDir)
	list := tm.ListTemplates()
	t.Logf("Found %d templates in temp dir", len(list))

	// Test with actual templates in ../../templates
	actualTM := NewTemplateManager("../../templates")

	tmpl, found := actualTM.GetTemplate("label_pasien")
	if !found {
		t.Fatalf("Template 'label_pasien' harusnya ditemukan di folder templates")
	}

	if tmpl.Paper.WidthMM != 50 || tmpl.Paper.HeightMM != 30 {
		t.Errorf("Dimensi kertas salah: expected 50x30, got %fx%f", tmpl.Paper.WidthMM, tmpl.Paper.HeightMM)
	}

	// Render ke PDF
	saveDir := filepath.Join(os.TempDir(), "test_pdf_out")
	defer os.RemoveAll(saveDir)

	pdfPath, err := actualTM.RenderToPDF(tmpl, tmpl.SampleData, saveDir, "TEST-JOB-01")
	if err != nil {
		t.Fatalf("Gagal render template ke PDF: %v", err)
	}

	fi, err := os.Stat(pdfPath)
	if err != nil {
		t.Fatalf("File PDF tidak terbentuk: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatalf("File PDF berukuran 0 byte")
	}
	t.Logf("Berhasil render PDF universal: %s (%d bytes)", pdfPath, fi.Size())
}
