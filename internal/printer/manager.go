package printer

import (
	"fmt"
	"log"
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

	// 0. Proses template: Cek apakah ada di TemplateManager dinamis (JSON) terlebih dahulu
	var generatedPDFPath string
	tm := GetTemplateManager()
	if req.Template != "" {
		if tmpl, ok := tm.GetTemplate(req.Template); ok {
			pdfPath, err := tm.RenderToPDF(tmpl, req.Data, cfg.PDFSaveDir, req.RequestID)
			if err == nil {
				generatedPDFPath = pdfPath
			}
		} else if len(req.Konten) == 0 {
			req.Konten = ApplyTemplate(req.Template, req.Data)
		}
	}

	// 1. Dapatkan mapping printer
	mapping, exists := m.cfgMgr.GetPrinterMapping(req.PrinterTarget)
	if !exists {
		// Jika target tidak terdaftar, gunakan default USB atau PDF
		mapping = model.PrinterMapping{
			Name:       req.PrinterTarget,
			Type:       model.PrinterTypeUSB,
			Target:     GetDefaultPrinter(),
			DriverType: "escpos",
			PaperSize:  "80mm",
			AutoCut:    true,
		}
		if mapping.Target == "" {
			mapping.Type = model.PrinterTypePDF
		}
	}

	// 2. Jika opsi View = true, langsung jadikan PDF untuk preview di layar browser
	if req.Options.View || mapping.Type == model.PrinterTypePDF {
		pdfPath := generatedPDFPath
		if pdfPath == "" {
			var err error
			pdfPath, err = GenerateReceiptPDF(req, cfg.PDFSaveDir)
			if err != nil {
				return nil, fmt.Errorf("gagal membuat PDF preview: %w", err)
			}
		}
		return &model.PrintResponse{
			Status:        "saved_pdf",
			JobID:         req.RequestID,
			Message:       "Dokumen berhasil disimpan sebagai PDF (Preview Mode)",
			TargetPrinter: "Virtual PDF",
			TargetType:    string(model.PrinterTypePDF),
			FilePath:      pdfPath,
			DurationMs:    time.Since(start).Milliseconds(),
			Timestamp:     time.Now().Format(time.RFC3339),
		}, nil
	}

	// 3. Rute Berdasarkan Tipe Printer
	switch mapping.Type {
	case model.PrinterTypeRelay:
		// Forward ke PC lain (Printer Sharing Antar-Agent)
		log.Printf("[RELAY] Meneruskan cetak '%s' ke Agent di %s", req.PrinterTarget, mapping.Target)
		resp, err := RelayPrintJob(mapping.Target, req, cfg.TimeoutSec)
		if err != nil {
			// Jika PC sebelah mati/putus dan auto-save PDF aktif: fallback ke PDF lokal!
			if cfg.AutoSavePDF {
				log.Printf("[FALLBACK] Relay gagal (%v), fallback otomatis simpan PDF lokal", err)
				pdfPath, pdfErr := GenerateReceiptPDF(req, cfg.PDFSaveDir)
				if pdfErr == nil {
					return &model.PrintResponse{
						Status:        "saved_pdf",
						JobID:         req.RequestID,
						Message:       fmt.Sprintf("Relay ke %s gagal, tersimpan aman sebagai PDF lokal", mapping.Target),
						TargetPrinter: "Local PDF Fallback",
						TargetType:    string(model.PrinterTypePDF),
						FilePath:      pdfPath,
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
		log.Printf("[SOCKET] Mengirim cetak '%s' ke Network Printer di %s", req.PrinterTarget, mapping.Target)
		rawBytes, err := m.buildRawBytes(req, mapping)
		if err != nil {
			return nil, err
		}

		err = PrintRawSocket(mapping.Target, rawBytes, cfg.TimeoutSec)
		if err != nil {
			if cfg.AutoSavePDF {
				log.Printf("[FALLBACK] Network Printer offline (%v), simpan PDF lokal", err)
				pdfPath, _ := GenerateReceiptPDF(req, cfg.PDFSaveDir)
				return &model.PrintResponse{
					Status:        "saved_pdf",
					JobID:         req.RequestID,
					Message:       fmt.Sprintf("Network printer offline, otomatis tersimpan ke PDF: %s", pdfPath),
					TargetPrinter: mapping.Target,
					TargetType:    string(model.PrinterTypePDF),
					FilePath:      pdfPath,
					DurationMs:    time.Since(start).Milliseconds(),
					Timestamp:     time.Now().Format(time.RFC3339),
				}, nil
			}
			return nil, err
		}

		return &model.PrintResponse{
			Status:        "success",
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

		// A. Jika dokumen dirender dari visual template atau driver bertipe universal/driver/gdi
		isUniversal := generatedPDFPath != "" || mapping.DriverType == "universal" || mapping.DriverType == "driver" || mapping.DriverType == "gdi"
		if isUniversal && generatedPDFPath != "" {
			err := PrintDocumentUniversal(mapping.Target, docName, generatedPDFPath)
			if err == nil {
				return &model.PrintResponse{
					Status:        "success",
					JobID:         req.RequestID,
					Message:       "Berhasil dicetak via Universal Spooler",
					TargetPrinter: mapping.Target,
					TargetType:    string(model.PrinterTypeUSB),
					FilePath:      generatedPDFPath,
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
				return &model.PrintResponse{
					Status:        "saved_pdf",
					JobID:         req.RequestID,
					Message:       "Dokumen tersimpan aman sebagai PDF",
					TargetPrinter: mapping.Target,
					TargetType:    string(model.PrinterTypePDF),
					FilePath:      generatedPDFPath,
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
				return &model.PrintResponse{
					Status:        "saved_pdf",
					JobID:         req.RequestID,
					Message:       fmt.Sprintf("Printer fisik belum terhubung, otomatis tersimpan sebagai PDF: %s", pdfPath),
					TargetPrinter: mapping.Target,
					TargetType:    string(model.PrinterTypePDF),
					FilePath:      pdfPath,
					DurationMs:    time.Since(start).Milliseconds(),
					Timestamp:     time.Now().Format(time.RFC3339),
				}, nil
			}
			return nil, err
		}

		return &model.PrintResponse{
			Status:        "success",
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
	if c == "tspl" || c == "escpos" || c == "generic" {
		return c
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

	if strings.Contains(p, "tm-") || strings.Contains(p, "t82") ||
		strings.Contains(p, "t88") || strings.Contains(p, "epson tm") ||
		strings.Contains(p, "thermal") || strings.Contains(p, "pos") ||
		strings.Contains(p, "kassen") || strings.Contains(p, "receipt") ||
		strings.Contains(p, "xprinter") {
		return "escpos"
	}

	// 2. Cek dari nama fungsi SIMRS
	if strings.Contains(t, "label") || strings.Contains(t, "barcode") ||
		strings.Contains(t, "gelang") || strings.Contains(t, "farmasi") {
		return "tspl"
	}

	if strings.Contains(t, "antrian") || strings.Contains(t, "kwitansi") ||
		strings.Contains(t, "struk") || strings.Contains(t, "tracer") {
		return "escpos"
	}

	return "escpos"
}

