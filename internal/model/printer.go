package model

// PrinterType mendefinisikan tipe koneksi ke printer
type PrinterType string

const (
	PrinterTypeUSB    PrinterType = "usb"    // Printer lokal via Windows Spooler (USB/LPT/Virtual)
	PrinterTypeSocket PrinterType = "socket" // Direct Raw TCP Socket (cth: 192.168.1.200:9100)
	PrinterTypeRelay  PrinterType = "relay"  // Forward ke Desktop Agent di PC lain
	PrinterTypePDF    PrinterType = "pdf"    // Simpan langsung sebagai file PDF
)

// PrinterMapping mendefinisikan pemetaan dari nama target SIMRS ke printer fisik
type PrinterMapping struct {
	Name        string      `json:"name"`                  // Cth: CetakAntrian, Kwitansi, LabelFarmasi
	Type        PrinterType `json:"type"`                  // usb, socket, relay, pdf
	Target      string      `json:"target"`                // Nama printer Windows / IP:Port / URL Relay
	DriverType  string      `json:"driver_type"`           // escpos (thermal), tspl (tsc barcode), generic (a4)
	PaperSize   string      `json:"paper_size,omitempty"`  // 80mm, 58mm, 50x30mm, A4
	AutoCut     bool        `json:"auto_cut"`              // Apakah otomatis potong kertas
	Description string      `json:"description,omitempty"` // Deskripsi penggunaan
}

// WindowsPrinterInfo merepresentasikan informasi printer yang terdeteksi di Windows
type WindowsPrinterInfo struct {
	Name      string `json:"name"`
	Port      string `json:"port"`
	Driver    string `json:"driver"`
	IsDefault bool   `json:"is_default"`
	Status    string `json:"status"`
}

