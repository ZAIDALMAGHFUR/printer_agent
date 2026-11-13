package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smart-print-agent/internal/autostart"
	"smart-print-agent/internal/config"
	"smart-print-agent/internal/handler"
	"smart-print-agent/internal/printer"
	"smart-print-agent/internal/queue"
	"smart-print-agent/internal/tray"
	"smart-print-agent/internal/updater"
	"smart-print-agent/internal/web"
)

func main() {
	// 1. Inisialisasi Config Manager
	cfgMgr := config.GetManager("configs/config.json")
	cfg := cfgMgr.GetConfig()

	// 2. Auto Registry Windows Startup (Jalankan otomatis saat Windows booting)
	if !autostart.IsEnabled() {
		_ = autostart.Enable()
	}

	// 3. Jalankan Windows System Tray Icon di background
	tray.StartTray(cfg.ServerPort, cfg.PDFSaveDir, cfg.HospitalName)

	// 4. Inisialisasi Printer Manager & Queue
	printMgr := printer.NewManager(cfgMgr)
	printQueue := queue.NewPrintQueue(100, printMgr, cfg.QueueWorkerCount)

	// 5. Inisialisasi Updater Service & Background Sync ke GitHub
	updaterSvc := updater.GetUpdater(cfg.GitHubRepo, "templates")
	updaterSvc.StartBackgroundSync(cfg.AutoSyncInterval)

	// 6. Inisialisasi API Handler
	api := handler.NewAPIHandler(cfgMgr, printMgr, printQueue)

	// 7. Daftarkan Routes
	mux := http.NewServeMux()

	// API Cetak untuk SIMRS
	mux.HandleFunc("/printgo/cetak", api.EnableCORS(api.HandlePrint))
	mux.HandleFunc("/printgo/cetak/antrian", api.EnableCORS(api.HandlePrintAntrian))
	mux.HandleFunc("/printgo/status", api.EnableCORS(api.HandleStatus))

	// API Template Management & Visual Designer
	mux.HandleFunc("/printgo/templates", api.EnableCORS(api.HandleTemplates))
	mux.HandleFunc("/printgo/templates/test-print", api.EnableCORS(api.HandleTemplateTestPrint))

	// API Updater & GitHub Sync
	mux.HandleFunc("/printgo/updater/status", api.EnableCORS(api.HandleUpdaterStatus))
	mux.HandleFunc("/printgo/updater/sync", api.EnableCORS(api.HandleUpdaterSync))

	// API Manajemen untuk Dashboard
	mux.HandleFunc("/api/printers", api.EnableCORS(api.HandleGetPrinters))
	mux.HandleFunc("/api/config", api.EnableCORS(api.HandleConfig))
	mux.HandleFunc("/api/test-print", api.EnableCORS(api.HandleTestPrint))
	mux.HandleFunc("/api/history", api.EnableCORS(api.HandleHistory))

	// Web Dashboard Pengaturan Printer
	dashboardHandler := web.GetStaticFileSystem()
	mux.Handle("/settings/", http.StripPrefix("/settings", dashboardHandler))
	mux.Handle("/settings", http.RedirectHandler("/settings/", http.StatusMovedPermanently))
	mux.Handle("/", dashboardHandler)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.ServerPort)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Tangani shutdown sinyal (CTRL+C / SIGTERM)
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		_ = server.Close()
		os.Exit(0)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[FATAL] Gagal menjalankan server HTTP: %v", err)
	}
}
