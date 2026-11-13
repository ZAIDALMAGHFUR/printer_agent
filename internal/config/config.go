package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"smart-print-agent/internal/model"
)

// AppConfig menyimpan seluruh konfigurasi aplikasi print agent
type AppConfig struct {
	ServerPort       int                             `json:"server_port"`        // Default: 1237
	HospitalName     string                          `json:"hospital_name"`      // Nama RS
	Publisher        string                          `json:"publisher"`          // Publisher / Pengembang
	AutoSavePDF      bool                            `json:"auto_save_pdf"`      // Jika printer mati/belum ada, auto save PDF
	PDFSaveDir       string                          `json:"pdf_save_dir"`       // Lokasi penyimpanan PDF (cth: C:\SIMRS_PDF)
	QueueWorkerCount int                             `json:"queue_worker_count"` // Jumlah worker goroutine paralel
	TimeoutSec       int                             `json:"timeout_sec"`        // Timeout koneksi socket/relay
	GitHubRepo       string                          `json:"github_repo"`        // URL Repository GitHub untuk Auto-Update & Sync
	AutoSyncInterval int                             `json:"auto_sync_interval_min"` // Interval sinkronisasi otomatis dalam menit
	Printers         map[string]model.PrinterMapping `json:"printers"`           // Pemetaan printer
}

// ConfigManager mengelola konfigurasi secara thread-safe
type ConfigManager struct {
	mu       sync.RWMutex
	cfg      *AppConfig
	filePath string
}

var (
	instance *ConfigManager
	once     sync.Once
)

// GetManager mengembalikan singleton instance ConfigManager
func GetManager(customPath ...string) *ConfigManager {
	once.Do(func() {
		p := "configs/config.json"
		if len(customPath) > 0 && customPath[0] != "" {
			p = customPath[0]
		}
		instance = &ConfigManager{filePath: p}
		instance.loadOrInit()
	})
	return instance
}

func (cm *ConfigManager) loadOrInit() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Cari lokasi config yang valid (CWD atau relatif terhadap Executable)
	candidates := []string{cm.filePath}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, cm.filePath),
			filepath.Join(exeDir, "..", cm.filePath),
		)
	}

	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			var cfg AppConfig
			if err := json.Unmarshal(data, &cfg); err == nil {
				cm.cfg = &cfg
				cm.filePath = p
				return
			}
		}
	}

	// Buat konfigurasi default jika belum ada
	cm.cfg = defaultConfiguration()
	_ = cm.saveLocked()
}

func defaultConfiguration() *AppConfig {
	return &AppConfig{
		ServerPort:       1237,
		HospitalName:     "RSU ASHAVIN",
		Publisher:        "SIMRS Developer Team",
		AutoSavePDF:      true,
		PDFSaveDir:       "C:\\SIMRS_PDF",
		QueueWorkerCount: 4,
		TimeoutSec:       5,
		GitHubRepo:       "https://github.com/ZAIDALMAGHFUR/printer_agent",
		AutoSyncInterval: 60,
		Printers: map[string]model.PrinterMapping{
			"CetakAntrian": {
				Name:        "CetakAntrian",
				Type:        model.PrinterTypeUSB,
				Target:      "EPSON TM-T82X Receipt",
				DriverType:  "escpos",
				PaperSize:   "80mm",
				AutoCut:     true,
				Description: "Printer antrian loket / kiosk (Epson TM-T82X)",
			},
			"Kwitansi": {
				Name:        "Kwitansi",
				Type:        model.PrinterTypeUSB,
				Target:      "EPSON TM-T82X Receipt",
				DriverType:  "escpos",
				PaperSize:   "80mm",
				AutoCut:     true,
				Description: "Printer kwitansi kasir",
			},
			"LabelFarmasi": {
				Name:        "LabelFarmasi",
				Type:        model.PrinterTypeUSB,
				Target:      "TSC TTP-244 Pro",
				DriverType:  "tspl",
				PaperSize:   "50x30mm",
				AutoCut:     false,
				Description: "Printer barcode etiket obat farmasi (TSC)",
			},
			"LabelPasien": {
				Name:        "LabelPasien",
				Type:        model.PrinterTypeUSB,
				Target:      "TSC TTP-244 Pro",
				DriverType:  "tspl",
				PaperSize:   "50x30mm",
				AutoCut:     false,
				Description: "Printer barcode identitas pasien",
			},
			"Billing": {
				Name:        "Billing",
				Type:        model.PrinterTypePDF,
				Target:      "AutoSavePDF",
				DriverType:  "generic",
				PaperSize:   "A4",
				AutoCut:     false,
				Description: "Dokumen rincian biaya / billing A4",
			},
			"CetakSep": {
				Name:        "CetakSep",
				Type:        model.PrinterTypePDF,
				Target:      "AutoSavePDF",
				DriverType:  "generic",
				PaperSize:   "A4",
				AutoCut:     false,
				Description: "Surat Eligibilitas Peserta (SEP BPJS)",
			},
		},
	}
}

// GetConfig mengembalikan salinan konfigurasi saat ini
func (cm *ConfigManager) GetConfig() AppConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return *cm.cfg
}

// GetPrinterMapping mengambil mapping printer berdasarkan nama target
func (cm *ConfigManager) GetPrinterMapping(targetName string) (model.PrinterMapping, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	mapping, exists := cm.cfg.Printers[targetName]
	return mapping, exists
}

// UpdateConfig memperbarui konfigurasi dan menyimpannya ke disk
func (cm *ConfigManager) UpdateConfig(newCfg AppConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cfg = &newCfg
	return cm.saveLocked()
}

// SetPrinterMapping mengubah atau menambah 1 pemetaan printer
func (cm *ConfigManager) SetPrinterMapping(mapping model.PrinterMapping) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.cfg.Printers == nil {
		cm.cfg.Printers = make(map[string]model.PrinterMapping)
	}
	cm.cfg.Printers[mapping.Name] = mapping
	return cm.saveLocked()
}

func (cm *ConfigManager) saveLocked() error {
	dir := filepath.Dir(cm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cm.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cm.filePath, data, 0644)
}
