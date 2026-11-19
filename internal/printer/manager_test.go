package printer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"smart-print-agent/internal/config"
	"smart-print-agent/internal/model"
)

func TestPrinterMappingWithTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	cfgData := []byte(`{
		"server_port": 2349,
		"hospital_name": "RSU ASHAVIN",
		"auto_save_pdf": true,
		"pdf_save_dir": "` + filepath.ToSlash(tmpDir) + `",
		"printers": {
			"AntrianLobby": {
				"name": "AntrianLobby",
				"type": "pdf",
				"target": "AutoSavePDF",
				"driver_type": "escpos",
				"paper_size": "80mm",
				"template": "antrian"
			},
			"Label2Col": {
				"name": "Label2Col",
				"type": "pdf",
				"target": "AutoSavePDF",
				"driver_type": "tspl",
				"paper_size": "75x20mm_2col",
				"template": "label_pasien_2kolom"
			},
			"GelangPasien": {
				"name": "GelangPasien",
				"type": "pdf",
				"target": "AutoSavePDF",
				"driver_type": "tspl",
				"paper_size": "100x28mm",
				"template": "gelang_pasien_laki"
			}
		}
	}`)

	if err := os.WriteFile(cfgPath, cfgData, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfgMgr := config.GetManager(cfgPath)
	mapping, exists := cfgMgr.GetPrinterMapping("AntrianLobby")
	if !exists {
		t.Fatalf("Expected AntrianLobby to exist")
	}
	if mapping.Template != "antrian" {
		t.Errorf("Expected mapping.Template to be 'antrian', got '%s'", mapping.Template)
	}

	mapping2, exists2 := cfgMgr.GetPrinterMapping("Label2Col")
	if !exists2 {
		t.Fatalf("Expected Label2Col to exist")
	}
	if mapping2.Template != "label_pasien_2kolom" {
		t.Errorf("Expected mapping.Template to be 'label_pasien_2kolom', got '%s'", mapping2.Template)
	}

	// Test Execute uses mapping.Template when req.Template is empty
	mgr := NewManager(cfgMgr)
	req := model.PrintRequest{
		PrinterTarget: "AntrianLobby",
		RequestID:     "TEST-MAPPING-EXEC",
		Data: map[string]interface{}{
			"nomor":    "B-099",
			"kategori": "BPJS KESEHATAN",
		},
		Options: model.PrintOptions{
			View: true,
		},
	}

	resp, err := mgr.Execute(req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resp.Status != "success" {
		t.Errorf("Expected status success, got %s", resp.Status)
	}
	if resp.FilePath == "" {
		t.Errorf("Expected non-empty FilePath")
	}

	// Test GelangPasien specifically routes to gelang_pasien_laki
	mappingGelang, existsGelang := cfgMgr.GetPrinterMapping("GelangPasien")
	if !existsGelang {
		t.Fatalf("Expected GelangPasien to exist")
	}
	if mappingGelang.Template != "gelang_pasien_laki" {
		t.Errorf("Expected mappingGelang.Template to be 'gelang_pasien_laki', got '%s'", mappingGelang.Template)
	}

	reqGelang := model.PrintRequest{
		PrinterTarget: "GelangPasien",
		RequestID:     "TEST-GELANG-EXEC",
		Data: map[string]interface{}{
			"nama_pasien":    "NANI",
			"no_mr":          "00150515",
			"tgl_lahir":      "29/01/1934",
			"umur":           "92",
			"tgl_registrasi": "2026-11-02 12:15:40",
			"no_registrasi":  "2611000008",
		},
		Options: model.PrintOptions{
			View: true,
		},
	}

	respGelang, err := mgr.Execute(reqGelang)
	if err != nil {
		t.Fatalf("Execute GelangPasien failed: %v", err)
	}
	if respGelang.Status != "success" {
		t.Errorf("Expected status success, got %s", respGelang.Status)
	}
	if !strings.Contains(respGelang.FilePath, "gelang_pasien_laki_") {
		t.Errorf("Expected PDF path to contain 'gelang_pasien_laki_', got '%s'", respGelang.FilePath)
	}
}

func TestDownloadPDFValidation(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"server_port": 2349, "pdf_save_dir": "`+filepath.ToSlash(tmpDir)+`"}`), 0644)
	cfgMgr := config.GetManager(cfgPath)
	mgr := NewManager(cfgMgr)

	// 1. Test server returning non-PDF (JSON string) -> harus gagal
	tsJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"error","message":"Bukan PDF"}`))
	}))
	defer tsJSON.Close()

	_, errJSON := mgr.downloadPDF(tsJSON.URL, tmpDir, "TEST-NONPDF", "Billing")
	if errJSON == nil {
		t.Errorf("Harusnya downloadPDF gagal saat server mengembalikan respon JSON bukan %%PDF")
	}

	// 2. Test server returning valid PDF bytes -> harus sukses
	tsPDF := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "format=json") {
			t.Errorf("URL query format=json harusnya sudah dibersihkan sebelum request, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF"))
	}))
	defer tsPDF.Close()

	pdfPath, errPDF := mgr.downloadPDF(tsPDF.URL+"?format=json", tmpDir, "TEST-REALPDF", "Billing")
	if errPDF != nil {
		t.Fatalf("Download PDF valid gagal: %v", errPDF)
	}
	if fi, err := os.Stat(pdfPath); err != nil || fi.Size() == 0 {
		t.Errorf("File PDF hasil download tidak valid: %v", err)
	}
}
