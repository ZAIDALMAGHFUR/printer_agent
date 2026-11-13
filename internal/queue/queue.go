package queue

import (
	"log"
	"sync"
	"time"

	"smart-print-agent/internal/model"
	"smart-print-agent/internal/printer"
)

// PrintHistory menyimpan rekaman hasil cetak terakhir untuk dashboard
type PrintHistory struct {
	JobID         string    `json:"job_id"`
	PrinterTarget string    `json:"printer_target"`
	DocumentType  string    `json:"document_type"`
	Status        string    `json:"status"`
	Message       string    `json:"message"`
	TargetPrinter string    `json:"target_printer"`
	DurationMs    int64     `json:"duration_ms"`
	Timestamp     time.Time `json:"timestamp"`
}

// PrintQueue mengelola antrian non-blocking dan riwayat pekerjaan cetak
type PrintQueue struct {
	jobs       chan model.PrintJob
	manager    *printer.Manager
	workerDone chan struct{}
	mu         sync.RWMutex
	history    []PrintHistory
	maxHistory int
}

// NewPrintQueue membuat instance antrian baru
func NewPrintQueue(bufferSize int, manager *printer.Manager, workerCount int) *PrintQueue {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	if workerCount <= 0 {
		workerCount = 2
	}

	pq := &PrintQueue{
		jobs:       make(chan model.PrintJob, bufferSize),
		manager:    manager,
		workerDone: make(chan struct{}),
		history:    make([]PrintHistory, 0, 50),
		maxHistory: 50,
	}

	// Mulai worker pool
	for i := 1; i <= workerCount; i++ {
		go pq.worker(i)
	}

	return pq
}

// Enqueue memasukkan pekerjaan cetak ke antrian (non-blocking)
func (pq *PrintQueue) Enqueue(job model.PrintJob) bool {
	select {
	case pq.jobs <- job:
		return true
	default:
		// Antrian penuh
		log.Printf("[QUEUE] Antrian cetak penuh! Gagal memasukkan job %s", job.ID)
		return false
	}
}

// GetHistory mengembalikan daftar riwayat cetak terakhir
func (pq *PrintQueue) GetHistory() []PrintHistory {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	res := make([]PrintHistory, len(pq.history))
	copy(res, pq.history)
	return res
}

func (pq *PrintQueue) addHistory(h PrintHistory) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Tambahkan di awal (paling baru)
	pq.history = append([]PrintHistory{h}, pq.history...)
	if len(pq.history) > pq.maxHistory {
		pq.history = pq.history[:pq.maxHistory]
	}
}

func (pq *PrintQueue) worker(id int) {
	for job := range pq.jobs {
		start := time.Now()
		resp, err := pq.manager.Execute(job.Request)

		h := PrintHistory{
			JobID:         job.ID,
			PrinterTarget: job.Request.PrinterTarget,
			DocumentType:  job.Request.DocumentType,
			DurationMs:    time.Since(start).Milliseconds(),
			Timestamp:     time.Now(),
		}

		if err != nil {
			h.Status = "error"
			h.Message = err.Error()
			log.Printf("[WORKER-%d] Gagal memproses job %s: %v", id, job.ID, err)
		} else {
			h.Status = resp.Status
			h.Message = resp.Message
			h.TargetPrinter = resp.TargetPrinter
			log.Printf("[WORKER-%d] Berhasil memproses job %s (%s) dalam %dms",
				id, job.ID, resp.Status, h.DurationMs)
		}

		pq.addHistory(h)
	}
}

