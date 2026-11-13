package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"smart-print-agent/internal/config"
	"smart-print-agent/internal/model"
	"smart-print-agent/internal/printer"
	"smart-print-agent/internal/queue"
	"smart-print-agent/internal/updater"
)

type APIHandler struct {
	cfgMgr    *config.ConfigManager
	manager   *printer.Manager
	queue     *queue.PrintQueue
	startTime time.Time
}

func NewAPIHandler(cfgMgr *config.ConfigManager, manager *printer.Manager, q *queue.PrintQueue) *APIHandler {
	return &APIHandler{
		cfgMgr:    cfgMgr,
		manager:   manager,
		queue:     q,
		startTime: time.Now(),
	}
}

// EnableCORS menambahkan header CORS agar web SIMRS bisa memanggil dari domain mana pun
func (h *APIHandler) EnableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// HandlePrint menangani endpoint universal POST /printgo/cetak
func (h *APIHandler) HandlePrint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method tidak diizinkan, gunakan POST"}`, http.StatusMethodNotAllowed)
		return
	}

	var req model.PrintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "Payload JSON tidak valid: " + err.Error(),
		})
		return
	}

	if req.PrinterTarget == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "Field 'printer_target' wajib diisi (cth: CetakAntrian, Kwitansi, LabelFarmasi)",
		})
		return
	}

	if req.RequestID == "" {
		req.RequestID = fmt.Sprintf("REQ-%d", time.Now().UnixNano()%1000000)
	}

	// Eksekusi langsung secara synchronous agar SIMRS langsung tahu statusnya (sukses / saved_pdf / error)
	resp, err := h.manager.Execute(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(model.PrintResponse{
			Status:        "error",
			JobID:         req.RequestID,
			Message:       err.Error(),
			TargetPrinter: req.PrinterTarget,
			Timestamp:     time.Now().Format(time.RFC3339),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// HandlePrintAntrian menangani endpoint khusus POST /printgo/cetak/antrian
func (h *APIHandler) HandlePrintAntrian(w http.ResponseWriter, r *http.Request) {
	// Teruskan ke HandlePrint dengan preset printer_target = CetakAntrian
	r.URL.Path = "/printgo/cetak"
	h.HandlePrint(w, r)
}

// HandleStatus menangani GET /printgo/status (Health check)
func (h *APIHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	hostname, _ := os.Hostname()
	cfg := h.cfgMgr.GetConfig()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "online",
		"agent_version": "2.0.0-Golang",
		"hostname":      hostname,
		"hospital":      cfg.HospitalName,
		"port":          cfg.ServerPort,
		"uptime_sec":    int(time.Since(h.startTime).Seconds()),
		"auto_save_pdf": cfg.AutoSavePDF,
		"pdf_dir":       cfg.PDFSaveDir,
		"timestamp":     time.Now().Format(time.RFC3339),
	})
}

// HandleGetPrinters menangani GET /api/printers
func (h *APIHandler) HandleGetPrinters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	printers, err := printer.GetInstalledPrinters()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(printers)
}

// HandleConfig menangani GET & POST /api/config
func (h *APIHandler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		cfg := h.cfgMgr.GetConfig()
		json.NewEncoder(w).Encode(cfg)

	case http.MethodPost:
		var newCfg config.AppConfig
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON tidak valid: " + err.Error()})
			return
		}

		if err := h.cfgMgr.UpdateConfig(newCfg); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Gagal menyimpan konfigurasi: " + err.Error()})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Konfigurasi berhasil disimpan"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// HandleTestPrint menangani POST /api/test-print
func (h *APIHandler) HandleTestPrint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		TargetName string `json:"target_name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	target := body.TargetName
	if target == "" {
		target = "CetakAntrian"
	}

	cfg := h.cfgMgr.GetConfig()
	mapping, _ := h.cfgMgr.GetPrinterMapping(target)

	// Buat test print sesuai driver
	var req model.PrintRequest
	req.RequestID = fmt.Sprintf("TEST-%d", time.Now().Unix()%10000)
	req.PrinterTarget = target

	if strings.ToLower(mapping.DriverType) == "tspl" {
		// Test Label Barcode
		req.DocumentType = "test_barcode"
		req.Konten = []model.PrintLine{
			{Text: cfg.HospitalName, Bold: true, Size: "medium"},
			{Text: "TEST CETAK BARCODE", Size: "normal"},
			{Type: "divider"},
			{Type: "barcode", Text: "TEST-123456", Height: 45},
			{Text: time.Now().Format("2006-01-02 15:04:05"), Size: "small"},
		}
	} else {
		// Test Struk Thermal
		req.DocumentType = "test_thermal"
		req.Konten = []model.PrintLine{
			{Text: cfg.HospitalName, Align: "center", Bold: true, Size: "large"},
			{Text: "TES UJI CETAK AGENT GOLANG", Align: "center", Bold: true, Size: "normal"},
			{Type: "divider"},
			{Text: "Printer Target : " + target, Align: "left"},
			{Text: "Device Windows : " + mapping.Target, Align: "left"},
			{Text: "Waktu Pengujian: " + time.Now().Format("2006-01-02 15:04:05"), Align: "left"},
			{Type: "divider"},
			{Text: "TES NOMOR ANTRIAN:", Align: "center"},
			{Text: "T-001", Align: "center", Bold: true, Size: "extra_large"},
			{Text: "Status: BERHASIL TERHUBUNG", Align: "center", Bold: true},
			{Type: "cut"},
		}
	}

	resp, err := h.manager.Execute(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// HandleHistory menangani GET /api/history
func (h *APIHandler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.queue.GetHistory())
}

// HandleTemplates menangani CRUD template visual (GET, POST, DELETE)
func (h *APIHandler) HandleTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tm := printer.GetTemplateManager()

	switch r.Method {
	case http.MethodGet:
		id := r.URL.Query().Get("id")
		if id != "" {
			tmpl, found := tm.GetTemplate(id)
			if !found {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "Template tidak ditemukan"})
				return
			}
			json.NewEncoder(w).Encode(tmpl)
			return
		}
		json.NewEncoder(w).Encode(tm.ListTemplates())

	case http.MethodPost:
		var tmpl model.TemplateDefinition
		if err := json.NewDecoder(r.Body).Decode(&tmpl); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Payload JSON template tidak valid: " + err.Error()})
			return
		}
		if err := tm.SaveTemplate(tmpl); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Gagal menyimpan template: " + err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "Template berhasil disimpan ke folder templates/",
			"id":      tmpl.ID,
		})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Parameter id diperlukan"})
			return
		}
		if err := tm.DeleteTemplate(id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Gagal menghapus template: " + err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "Template berhasil dihapus",
			"id":      id,
		})

	default:
		http.Error(w, `{"error":"Method tidak diizinkan"}`, http.StatusMethodNotAllowed)
	}
}

// HandleTemplateTestPrint mencetak uji template langsung ke printer fisik atau PDF preview
func (h *APIHandler) HandleTemplateTestPrint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method tidak diizinkan, gunakan POST"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TemplateID    string                 `json:"template_id"`
		PrinterTarget string                 `json:"printer_target"`
		Data          map[string]interface{} `json:"data"`
		View          bool                   `json:"view"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Data request tidak valid"})
		return
	}

	tm := printer.GetTemplateManager()
	tmpl, found := tm.GetTemplate(req.TemplateID)
	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Template '" + req.TemplateID + "' tidak ditemukan"})
		return
	}

	renderData := req.Data
	if len(renderData) == 0 {
		renderData = tmpl.SampleData
	}

	printReq := model.PrintRequest{
		RequestID:     fmt.Sprintf("TEST-TMPL-%d", time.Now().Unix()%10000),
		PrinterTarget: req.PrinterTarget,
		DocumentType:  tmpl.ID,
		Template:      tmpl.ID,
		Data:          renderData,
		Options: model.PrintOptions{
			View: req.View,
		},
	}

	resp, err := h.manager.Execute(printReq)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// HandleUpdaterStatus mengembalikan status sinkronisasi GitHub updater
func (h *APIHandler) HandleUpdaterStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg := h.cfgMgr.GetConfig()
	upd := updater.GetUpdater(cfg.GitHubRepo, "templates")
	json.NewEncoder(w).Encode(upd.GetStatus())
}

// HandleUpdaterSync memicu sinkronisasi template langsung dari GitHub
func (h *APIHandler) HandleUpdaterSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method tidak diizinkan, gunakan POST"}`, http.StatusMethodNotAllowed)
		return
	}

	cfg := h.cfgMgr.GetConfig()
	upd := updater.GetUpdater(cfg.GitHubRepo, "templates")
	err := upd.SyncTemplates()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(upd.GetStatus())
}


