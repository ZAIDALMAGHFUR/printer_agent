package printer

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"smart-print-agent/internal/config"
	"smart-print-agent/internal/model"
)

// Manager mengoordinasikan seluruh perutean cetak ke hardware lokal, socket, relay, atau PDF
type Manager struct {
	cfgMgr *config.ConfigManager
}

// NewManager membuat instance Manager printer baru
func NewManager(cfgMgr *config.ConfigManager) *Manager {
	return &Manager{cfgMgr: cfgMgr}
}

// Execute mengeksekusi satu pekerjaan cetak berdasarkan pemetaan targetnya
func (m *Manager) Execute(req model.PrintRequest) (*model.PrintResponse, error) {
	start := time.Now()
	cfg := m.cfgMgr.GetConfig()

	// 1. Dapatkan mapping printer terlebih dahulu
	mapping, exists := m.cfgMgr.GetPrinterMapping(req.PrinterTarget)
	if !exists {
		// Jika target tidak terdaftar di config, cek apakah target/printer_name adalah nama printer fisik langsung
		targetPrinter := req.PrinterTarget
		if req.PrinterName != "" {
			targetPrinter = req.PrinterName
		}
		driver := DetectDriver(req.PrinterTarget, targetPrinter, "")
		mapping = model.PrinterMapping{
			Name:       req.PrinterTarget,
			Type:       model.PrinterTypeUSB,
			Target:     targetPrinter,
			DriverType: driver,
			PaperSize:  "80mm",
			AutoCut:    driver == "escpos",
		}
		if mapping.Target == "" {
			mapping.Target = GetDefaultPrinter()
		}
		if mapping.Target == "" {
			mapping.Type = model.PrinterTypePDF
		}
	} else if req.PrinterName != "" {
		// Override target printer fisik jika request secara eksplisit menyertakan printer_name
		mapping.Target = req.PrinterName
		mapping.DriverType = DetectDriver(req.PrinterTarget, req.PrinterName, mapping.DriverType)
	}

	// FLEKSIBILITAS PEMETAAN TEMPLATE DARI DASHBOARD PRINTER AGENT:
	// Dashboard printer agent adalah otoritas utama (authoritative) penentu desain template.
	// 1. Jika printer target terdaftar di dashboard pemetaan printer agen (exists == true),
	//    maka template dari dashboard SELALU DIGUNAKAN (meng-override nilai template bawaan dari SIMRS).
	//    Hal ini memungkinkan pengguna/admin bebas mengganti template cetak (misal: gelang laki-laki vs gelang 280mm,
	//    atau label 1 kolom vs 2 kolom, atau template baru) langsung dari dropdown dashboard tanpa menyentuh kode SIMRS.
	// 2. Jika di dashboard disetel "-- Tanpa Template (RAW) --" (mapping.Template == ""),
	//    maka req.Template dikosongkan agar agen langsung mencetak dokumen asli (DomPDF via pdf_url atau RAW).
	// 3. Jika printer target belum terpetakan di config (exists == false) dan SIMRS menyertakan template,
	//    agen tetap menggunakan req.Template dari SIMRS sebagai fallback.
	if exists {
		req.Template = mapping.Template
	}

	// 0. Proses template: Cek apakah ada di TemplateManager dinamis (JSON) terlebih dahulu
	var generatedPDFPath string
	tm := GetTemplateManager()
	if req.Template != "" && (len(req.Data) > 0 || req.PDFURL == "") {
		if tmpl, ok := tm.GetTemplate(req.Template); ok {
			pdfPath, err := tm.RenderToPDF(tmpl, req.Data, cfg.PDFSaveDir, req.RequestID)
			if err == nil {
				generatedPDFPath = pdfPath
			}
		}
		if len(req.Konten) == 0 {
			req.Konten = ApplyTemplate(req.Template, req.Data)
		}
	}

	// 0b. Jika ada PDFURL dan belum ada generatedPDFPath, unduh PDF dari URL backend!
	if req.PDFURL != "" && generatedPDFPath == "" {
		docName := req.DocumentType
		if docName == "" {
			docName = req.PrinterTarget
		}
		downloadedPath, err := m.downloadPDF(req.PDFURL, cfg.PDFSaveDir, req.RequestID, docName)
		if err == nil {
			generatedPDFPath = downloadedPath
		} else {
			log.Printf("[DOWNLOAD-PDF] Gagal mengunduh PDF dari %s: %v", req.PDFURL, err)
		}
	}

	// 2. Jika opsi View / Preview diminta atau printer bertipe Virtual PDF
	isViewMode := req.View || req.Options.View || strings.EqualFold(req.Mode, "view") || strings.EqualFold(req.Mode, "preview") || strings.EqualFold(req.Options.Mode, "view") || strings.EqualFold(req.Options.Mode, "preview") || mapping.Type == model.PrinterTypePDF

	if isViewMode {
		pdfPath := generatedPDFPath
		if pdfPath == "" {
			var err error
			pdfPath, err = GenerateReceiptPDF(req, cfg.PDFSaveDir)
			if err != nil {
				return nil, fmt.Errorf("gagal membuat PDF preview: %w", err)
			}
		}
		viewURL := toLocalPDFURL(pdfPath, cfg.ServerPort, cfg.PDFSaveDir)
		return &model.PrintResponse{
			Status:        "success",
			Action:        "view",
			Mode:          "view",
			OpenTab:       true,
			JobID:         req.RequestID,
			Message:       "Dokumen berhasil disiapkan untuk preview di browser",
			TargetPrinter: "Virtual PDF",
			TargetType:    string(model.PrinterTypePDF),
			FilePath:      pdfPath,
			PDFURL:        viewURL,
			ViewURL:       viewURL,
			DurationMs:    time.Since(start).Milliseconds(),
			Timestamp:     time.Now().Format(time.RFC3339),
		}, nil
	}

	// 3. Rute Berdasarkan Tipe Printer
	switch mapping.Type {
	case model.PrinterTypeRelay:
		// Forward ke PC lain (Printer Sharing Antar-Agent)
		relayHost := mapping.IP
		if relayHost == "" {
			relayHost = mapping.Target
		}
		// Jika mapping.Target adalah nama printer fisik (bukan IP/URL), teruskan sebagai req.PrinterName
		if mapping.Target != "" && !strings.HasPrefix(mapping.Target, "http://") && !strings.Contains(mapping.Target, ":") {
			req.PrinterName = mapping.Target
		}
		log.Printf("[RELAY] Meneruskan cetak '%s' ke Agent di %s (target printer: %s)", req.PrinterTarget, relayHost, req.PrinterName)
		resp, err := RelayPrintJob(relayHost, req, cfg.TimeoutSec)
		if err != nil {
			// Cek apakah mapping.Target dapat dicetak langsung via spooler lokal (cth: printer sharing Windows)
			log.Printf("[RELAY] Relay HTTP gagal (%v). Mencoba cetak via spooler lokal ke '%s'...", err, mapping.Target)
			docName := req.DocumentType
			if docName == "" {
				docName = req.PrinterTarget
			}
			rawBytes, rawErr := m.buildRawBytes(req, mapping)
			if rawErr == nil {
				if spoolErr := PrintRawBytes(mapping.Target, docName, rawBytes); spoolErr == nil {
					log.Printf("[RELAY] Berhasil dicetak via Spooler lokal ke '%s'", mapping.Target)
					return &model.PrintResponse{
						Status:        "success",
						Action:        "print",
						Mode:          "print",
						OpenTab:       false,
						JobID:         req.RequestID,
						Message:       fmt.Sprintf("Berhasil dicetak ke printer '%s'", mapping.Target),
						TargetPrinter: mapping.Target,
						TargetType:    string(model.PrinterTypeUSB),
						DurationMs:    time.Since(start).Milliseconds(),
						Timestamp:     time.Now().Format(time.RFC3339),
					}, nil
				}
			}

			// Jika PC sebelah mati/putus dan auto-save PDF aktif: fallback ke PDF lokal!
			if cfg.AutoSavePDF {
				log.Printf("[FALLBACK] Relay gagal (%v), fallback otomatis simpan PDF lokal", err)
				pdfPath, pdfErr := GenerateReceiptPDF(req, cfg.PDFSaveDir)
				if pdfErr == nil {
					pdfURL := toLocalPDFURL(pdfPath, cfg.ServerPort, cfg.PDFSaveDir)
					return &model.PrintResponse{
						Status:        "saved_pdf",
						Action:        "saved_pdf",
						Mode:          "print",
						OpenTab:       false,
						JobID:         req.RequestID,
						Message:       fmt.Sprintf("Relay ke %s gagal, tersimpan aman sebagai PDF lokal", relayHost),
						TargetPrinter: "Local PDF Fallback",
						TargetType:    string(model.PrinterTypePDF),
						FilePath:      pdfPath,
						PDFURL:        pdfURL,
						ViewURL:       pdfURL,
						DurationMs:    time.Since(start).Milliseconds(),
						Timestamp:     time.Now().Format(time.RFC3339),
					}, nil
				}
			}
			return nil, err
		}
		return resp, nil

	case model.PrinterTypeSocket:
		// Kirim langsung ke Network Printer (IP:Port via LAN/WiFi)
		socketAddr := mapping.IP
		if socketAddr == "" {
			socketAddr = mapping.Target
		}
		log.Printf("[SOCKET] Mengirim cetak '%s' ke Network Printer di %s", req.PrinterTarget, socketAddr)
		rawBytes, err := m.buildRawBytes(req, mapping)
		if err != nil {
			return nil, err
		}

		err = PrintRawSocket(socketAddr, rawBytes, cfg.TimeoutSec)
		if err != nil {
			if cfg.AutoSavePDF {
				log.Printf("[FALLBACK] Network Printer offline (%v), simpan PDF lokal", err)
				pdfPath, _ := GenerateReceiptPDF(req, cfg.PDFSaveDir)
				pdfURL := toLocalPDFURL(pdfPath, cfg.ServerPort, cfg.PDFSaveDir)
				return &model.PrintResponse{
					Status:        "saved_pdf",
					Action:        "saved_pdf",
					Mode:          "print",
					OpenTab:       false,
					JobID:         req.RequestID,
					Message:       fmt.Sprintf("Network printer offline, otomatis tersimpan ke PDF: %s", pdfPath),
					TargetPrinter: mapping.Target,
					TargetType:    string(model.PrinterTypePDF),
					FilePath:      pdfPath,
					PDFURL:        pdfURL,
					ViewURL:       pdfURL,
					DurationMs:    time.Since(start).Milliseconds(),
					Timestamp:     time.Now().Format(time.RFC3339),
				}, nil
			}
			return nil, err
		}

		return &model.PrintResponse{
			Status:        "success",
			Action:        "print",
			Mode:          "print",
			OpenTab:       false,
			JobID:         req.RequestID,
			Message:       "Berhasil dicetak ke Network Printer",
			TargetPrinter: mapping.Target,
			TargetType:    string(model.PrinterTypeSocket),
			DurationMs:    time.Since(start).Milliseconds(),
			Timestamp:     time.Now().Format(time.RFC3339),
		}, nil

	default: // model.PrinterTypeUSB (Local Windows Spooler)
		log.Printf("[USB] Mengirim cetak '%s' ke Printer Windows '%s'", req.PrinterTarget, mapping.Target)

		docName := req.DocumentType
		if docName == "" {
			docName = req.PrinterTarget
		}

		// A. Jika driver memang di-setting sebagai Universal/Driver Windows (GDI)
		isUniversal := (mapping.DriverType == "universal" || mapping.DriverType == "driver" || mapping.DriverType == "gdi") && generatedPDFPath != ""
		if isUniversal {
			err := PrintDocumentUniversal(mapping.Target, docName, generatedPDFPath)
			if err == nil {
				pdfURL := toLocalPDFURL(generatedPDFPath, cfg.ServerPort, cfg.PDFSaveDir)
				return &model.PrintResponse{
					Status:        "success",
					Action:        "print",
					Mode:          "print",
					OpenTab:       false,
					JobID:         req.RequestID,
					Message:       "Berhasil dicetak via Universal Spooler",
					TargetPrinter: mapping.Target,
					TargetType:    string(model.PrinterTypeUSB),
					FilePath:      generatedPDFPath,
					PDFURL:        pdfURL,
					ViewURL:       pdfURL,
					DurationMs:    time.Since(start).Milliseconds(),
					Timestamp:     time.Now().Format(time.RFC3339),
				}, nil
			}
			log.Printf("[SPOOLER] Universal print gagal (%v), mencoba fallback RAW/PDF...", err)
		}

		rawBytes, err := m.buildRawBytes(req, mapping)
		if err != nil {
			// Jika rawBytes gagal tapi ada generatedPDFPath, fallback simpan PDF
			if generatedPDFPath != "" {
				pdfURL := toLocalPDFURL(generatedPDFPath, cfg.ServerPort, cfg.PDFSaveDir)
				return &model.PrintResponse{
					Status:        "saved_pdf",
					Action:        "saved_pdf",
					Mode:          "print",
					OpenTab:       false,
					JobID:         req.RequestID,
					Message:       "Dokumen tersimpan aman sebagai PDF",
					TargetPrinter: mapping.Target,
					TargetType:    string(model.PrinterTypePDF),
					FilePath:      generatedPDFPath,
					PDFURL:        pdfURL,
					ViewURL:       pdfURL,
					DurationMs:    time.Since(start).Milliseconds(),
					Timestamp:     time.Now().Format(time.RFC3339),
				}, nil
			}
			return nil, err
		}

		err = PrintRawBytes(mapping.Target, docName, rawBytes)
		if err != nil {
			// FALLBACK OTOMATIS: Jika printer tidak terhubung / offline / belum dicolok
			if cfg.AutoSavePDF {
				log.Printf("[FALLBACK] Printer '%s' belum siap (%v), otomatis simpan ke PDF", mapping.Target, err)
				pdfPath := generatedPDFPath
				if pdfPath == "" {
					var pdfErr error
					pdfPath, pdfErr = GenerateReceiptPDF(req, cfg.PDFSaveDir)
					if pdfErr != nil {
						return nil, err
					}
				}
				pdfURL := toLocalPDFURL(pdfPath, cfg.ServerPort, cfg.PDFSaveDir)
				return &model.PrintResponse{
					Status:        "saved_pdf",
					Action:        "saved_pdf",
					Mode:          "print",
					OpenTab:       false,
					JobID:         req.RequestID,
					Message:       fmt.Sprintf("Printer fisik belum terhubung, otomatis tersimpan sebagai PDF: %s", pdfPath),
					TargetPrinter: mapping.Target,
					TargetType:    string(model.PrinterTypePDF),
					FilePath:      pdfPath,
					PDFURL:        pdfURL,
					ViewURL:       pdfURL,
					DurationMs:    time.Since(start).Milliseconds(),
					Timestamp:     time.Now().Format(time.RFC3339),
				}, nil
			}
			return nil, err
		}

		return &model.PrintResponse{
			Status:        "success",
			Action:        "print",
			Mode:          "print",
			OpenTab:       false,
			JobID:         req.RequestID,
			Message:       "Berhasil dicetak ke Printer Fisik",
			TargetPrinter: mapping.Target,
			TargetType:    string(model.PrinterTypeUSB),
			DurationMs:    time.Since(start).Milliseconds(),
			Timestamp:     time.Now().Format(time.RFC3339),
		}, nil
	}
}

func (m *Manager) buildRawBytes(req model.PrintRequest, mapping model.PrinterMapping) ([]byte, error) {
	// Jika raw data sudah disediakan langsung oleh SIMRS
	if req.RawData != "" {
		return []byte(req.RawData), nil
	}

	copies := req.Options.Copies
	if copies <= 0 {
		copies = 1
	}

	// Deteksi driver otomatis jika diset "auto" atau kosong
	driver := DetectDriver(mapping.Name, mapping.Target, mapping.DriverType)

	switch driver {
	case "tspl":
		// Jika ada template dinamis JSON, render langsung ke TSPL dengan posisi presisi & multi-kolom
		if req.Template != "" {
			if tmpl, ok := GetTemplateManager().GetTemplate(req.Template); ok {
				return BuildTSPLFromTemplate(tmpl, req.Data, copies)
			}
		}
		// Driver TSC Barcode
		return BuildTSPL(req.Konten, mapping.PaperSize, copies)
	default:
		// Driver ESC/POS (Epson TM-T82X / Kassen / Xprinter)
		autoCut := mapping.AutoCut
		if req.Options.CutPaper {
			autoCut = true
		}
		return BuildESCPOS(req.Konten, autoCut, req.Options.OpenCashDrawer)
	}
}

// DetectDriver mendeteksi driver printer secara cerdas tanpa membebani user
func DetectDriver(targetName string, printerName string, configured string) string {
	c := strings.ToLower(configured)
	if c == "tspl" || strings.HasPrefix(c, "tspl") {
		return "tspl"
	}
	if c == "escpos" || strings.HasPrefix(c, "escpos") {
		return "escpos"
	}
	if c == "generic" {
		return "generic"
	}

	p := strings.ToLower(printerName)
	t := strings.ToLower(targetName)

	// 1. Cek dari nama fisik printer Windows
	if strings.Contains(p, "tsc") || strings.Contains(p, "zebra") ||
		strings.Contains(p, "ttp") || strings.Contains(p, "te200") ||
		strings.Contains(p, "da200") || strings.Contains(p, "barcode") ||
		strings.Contains(p, "label") {
		return "tspl"
	}

	// Cek apakah printer Dot-Matrix (Epson LX-300, LX-310, LQ-2190) atau Printer A4/Laser/Inkjet
	if strings.Contains(p, "lx") || strings.Contains(p, "lq") ||
		strings.Contains(p, "matrix") || strings.Contains(p, "dot") ||
		strings.Contains(p, "laser") || strings.Contains(p, "deskjet") ||
		strings.Contains(p, "canon") || strings.Contains(p, "hp ") ||
		strings.Contains(p, "brother") {
		return "generic"
	}

	if strings.Contains(p, "tm-") || strings.Contains(p, "t82") ||
		strings.Contains(p, "t88") || strings.Contains(p, "epson tm") ||
		strings.Contains(p, "thermal") || strings.Contains(p, "pos") ||
		strings.Contains(p, "kassen") || strings.Contains(p, "receipt") ||
		strings.Contains(p, "xprinter") {
		return "escpos"
	}

	// 2. Cek dari nama fungsi SIMRS
	if strings.Contains(t, "registrasi") || strings.Contains(t, "billing") ||
		strings.Contains(t, "sep") || strings.Contains(t, "laporan") ||
		strings.Contains(t, "a4") {
		return "generic"
	}

	if strings.Contains(t, "label") || strings.Contains(t, "barcode") ||
		strings.Contains(t, "gelang") || strings.Contains(t, "farmasi") {
		return "tspl"
	}

	if strings.Contains(t, "antrian") || strings.Contains(t, "kwitansi") ||
		strings.Contains(t, "struk") || strings.Contains(t, "tracer") {
		return "escpos"
	}

	return "generic"
}

// downloadPDF mengunduh file PDF dari remote URL (cth: endpoint backend SIMRS) dan menyimpannya ke folder lokal
func (m *Manager) downloadPDF(pdfURL, saveDir, reqID, docName string) (string, error) {
	if docName == "" {
		docName = "Struk"
	}
	if reqID == "" {
		reqID = fmt.Sprintf("REQ-%d", time.Now().UnixNano()%1000000)
	}

	// 1. Bersihkan parameter format=json jika tidak sengaja terbawa di URL agar mengunduh binary PDF murni
	if strings.Contains(pdfURL, "format=") {
		pdfURL = strings.ReplaceAll(pdfURL, "format=json&", "")
		pdfURL = strings.ReplaceAll(pdfURL, "&format=json", "")
		pdfURL = strings.ReplaceAll(pdfURL, "?format=json", "")
	}

	now := time.Now()
	targetDir := filepath.Join(saveDir, now.Format("2006-01-02"))
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori PDF: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(pdfURL)
	if err != nil {
		return "", fmt.Errorf("gagal mengambil file PDF dari %s: %w", pdfURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server mengembalikan status HTTP %d saat mengunduh PDF", resp.StatusCode)
	}

	// 2. Baca seluruh data respon untuk validasi magic bytes PDF
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gagal membaca data respon PDF: %w", err)
	}

	// 3. Validasi apakah file benar-benar berformat PDF (dimulai dengan %PDF)
	if len(bodyBytes) < 4 || !bytes.HasPrefix(bodyBytes, []byte("%PDF")) {
		preview := string(bodyBytes)
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		return "", fmt.Errorf("respon dari %s bukan file PDF valid (panjang: %d bytes, konten: %s)", pdfURL, len(bodyBytes), preview)
	}

	fileName := fmt.Sprintf("%s_%s_%s.pdf", docName, reqID, now.Format("150405"))
	fullPath := filepath.Join(targetDir, fileName)

	if err := os.WriteFile(fullPath, bodyBytes, 0644); err != nil {
		return "", fmt.Errorf("gagal menyimpan file PDF: %w", err)
	}

	log.Printf("[DOWNLOAD-PDF] Sukses mengunduh dan menyimpan PDF valid (%d bytes) ke: %s", len(bodyBytes), fullPath)
	return fullPath, nil
}

// toLocalPDFURL mengubah path absolut file PDF di disk lokal menjadi URL HTTP lokal yang dapat diakses browser
func toLocalPDFURL(pdfPath string, port int, saveDir string) string {
	if pdfPath == "" {
		return ""
	}
	relPath, err := filepath.Rel(saveDir, pdfPath)
	if err == nil && !strings.HasPrefix(relPath, "..") {
		return fmt.Sprintf("http://127.0.0.1:%d/printgo/pdf/%s", port, filepath.ToSlash(relPath))
	}
	return fmt.Sprintf("http://127.0.0.1:%d/printgo/pdf/%s", port, filepath.Base(pdfPath))
}



